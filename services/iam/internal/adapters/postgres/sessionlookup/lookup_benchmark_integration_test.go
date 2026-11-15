//go:build integration

package sessionlookup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

const hit82PoolLimit = 5

// This helper intentionally accepts testing.TB: a benchmark must not manufacture
// a testing.T or use a production database. All connection values stay private.
func benchmarkDatabase(tb testing.TB) testDatabase {
	tb.Helper()
	adminDSN, readerDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_DSN")
	if adminDSN == "" || readerDSN == "" {
		tb.Skip("use the isolated PostgreSQL verification runner for benchmarks")
	}
	adminConfig, adminErr := pgxpool.ParseConfig(adminDSN)
	readerConfig, readerErr := pgxpool.ParseConfig(readerDSN)
	if adminErr != nil || readerErr != nil {
		tb.Fatal("invalid isolated database configuration; connection values withheld")
	}
	for _, cfg := range []*pgxpool.Config{adminConfig, readerConfig} {
		if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") {
			tb.Fatal("benchmarks require an isolated loopback hit68_test_ database")
		}
		cfg.MinConns, cfg.MaxConns = 0, hit82PoolLimit
	}
	if readerConfig.ConnConfig.User != "hp_hit68_lookup" ||
		readerConfig.ConnConfig.Database != adminConfig.ConnConfig.Database ||
		readerConfig.ConnConfig.Port != adminConfig.ConnConfig.Port {
		tb.Fatal("benchmark reader and fixture administrator must share the isolated test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		tb.Fatal("could not create the isolated fixture administrator")
	}
	tb.Cleanup(admin.Close)
	reader, err := pgxpool.NewWithConfig(ctx, readerConfig)
	if err != nil {
		tb.Fatal("could not create the bounded runtime pool")
	}
	tb.Cleanup(reader.Close)
	var databaseName, currentUser, readOnly string
	var elevated, iamRole bool
	var serverVersion int
	err = reader.QueryRow(ctx, `SELECT current_database(),current_user,current_setting('transaction_read_only'),
EXISTS(SELECT 1 FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole)),
pg_has_role(current_user,'hp_iam_app','MEMBER'),current_setting('server_version_num')::int`).Scan(
		&databaseName, &currentUser, &readOnly, &elevated, &iamRole, &serverVersion)
	if err != nil || databaseName != readerConfig.ConnConfig.Database || currentUser != "hp_hit68_lookup" ||
		readOnly != "on" || elevated || !iamRole || serverVersion != 180007 {
		tb.Fatal("benchmark requires nonprivileged read-only IAM access to PostgreSQL 18.7")
	}
	lookup, err := New(reader)
	if err != nil {
		tb.Fatal("could not construct the bounded IAM lookup")
	}
	warmBenchmarkPool(tb, ctx, reader)
	return testDatabase{admin: admin, reader: reader, lookup: lookup}
}

func warmBenchmarkPool(tb testing.TB, ctx context.Context, pool *pgxpool.Pool) {
	tb.Helper()
	var connections []*pgxpool.Conn
	defer func() {
		for _, connection := range connections {
			connection.Release()
		}
	}()
	for range hit82PoolLimit {
		connection, err := pool.Acquire(ctx)
		if err != nil {
			tb.Fatal("could not warm the bounded runtime pool before measurement")
		}
		connections = append(connections, connection)
	}
}

func benchmarkUUID(tb testing.TB) string {
	tb.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		tb.Fatal("synthetic fixture random source unavailable")
	}
	value[6], value[8] = value[6]&0x0f|0x40, value[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func benchmarkSeed(tb testing.TB, db testDatabase) fixture {
	tb.Helper()
	id, principal := benchmarkUUID(tb), benchmarkUUID(tb)
	secret := "HIT82-SYNTHETIC-SECRET-" + strings.ReplaceAll(id, "-", "")
	reference, err := Reference(secret)
	if err != nil {
		tb.Fatal("invalid bounded synthetic secret")
	}
	digest, csrf := sha256.Sum256([]byte(secret)), sha256.Sum256([]byte("HIT82-SYNTHETIC-CSRF-"+id))
	created := time.Date(2026, 11, 12, 0, 0, 0, 123456000, time.UTC)
	want := sessionpolicy.Snapshot{Found: true, CreatedAt: created,
		ExpiresAt: created.Add(24 * time.Hour), IdleExpiresAt: created.Add(8 * time.Hour)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.admin.Exec(ctx, `INSERT INTO hp_iam.principals
(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://issuer.invalid',$2,$3)`, principal, id, created)
	if err != nil {
		tb.Fatal("could not create synthetic benchmark principal")
	}
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.admin.Exec(ctx, "DELETE FROM hp_iam.sessions WHERE session_id=$1", id); err != nil {
			tb.Error("benchmark session cleanup failed")
		}
		if _, err := db.admin.Exec(ctx, "DELETE FROM hp_iam.principals WHERE principal_id=$1", principal); err != nil {
			tb.Error("benchmark principal cleanup failed")
		}
	})
	_, err = db.admin.Exec(ctx, `INSERT INTO hp_iam.sessions
(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
VALUES($1,$2,$3,$4,$5,$6,$7,1)`, id, digest[:], principal, csrf[:], created, want.ExpiresAt, want.IdleExpiresAt)
	if err != nil {
		tb.Fatal("could not create synthetic benchmark session")
	}
	return fixture{reference: reference, secret: secret, id: id, want: want}
}

type benchmarkWorkload struct {
	name      string
	ctx       context.Context
	reference string
	want      sessionpolicy.Snapshot
	canceled  bool
	parallel  bool
}

func benchmarkWorkloads(tb testing.TB, db testDatabase) []benchmarkWorkload {
	tb.Helper()
	active, revoked := benchmarkSeed(tb, db), benchmarkSeed(tb, db)
	// Verify an actual committed change is visible to the following independent
	// read. No cache, mocked result, or synthetic timing substitutes for this.
	if got, err := db.lookup.Lookup(context.Background(), revoked.reference); err != nil || !snapshotEqual(got, revoked.want) {
		tb.Fatal("pre-revocation read did not return the stored active snapshot")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := db.admin.Exec(ctx, "UPDATE hp_iam.sessions SET revoked_at=$1 WHERE session_id=$2",
		revoked.want.CreatedAt.Add(time.Minute), revoked.id)
	if err != nil || tag.RowsAffected() != 1 {
		tb.Fatal("could not commit the controlled fixture revocation")
	}
	revoked.want.Revoked = true
	if got, err := db.lookup.Lookup(context.Background(), revoked.reference); err != nil || !snapshotEqual(got, revoked.want) {
		tb.Fatal("the next read did not observe committed revocation")
	}
	unknown, err := Reference(strings.Repeat("HIT82-UNKNOWN-SYNTHETIC-", 3))
	if err != nil {
		tb.Fatal("invalid bounded synthetic unknown reference")
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	workloads := []benchmarkWorkload{
		{name: "valid", ctx: context.Background(), reference: active.reference, want: active.want},
		{name: "unknown", ctx: context.Background(), reference: unknown},
		{name: "revoked", ctx: context.Background(), reference: revoked.reference, want: revoked.want},
		{name: "canceled", ctx: canceled, reference: active.reference, canceled: true},
		{name: "parallel_valid_pool5", ctx: context.Background(), reference: active.reference, want: active.want, parallel: true},
	}
	for _, workload := range workloads {
		got, err := db.lookup.Lookup(workload.ctx, workload.reference)
		if !workload.accepts(got, err) {
			tb.Fatal("benchmark fixture does not satisfy lookup semantics: " + workload.name)
		}
	}
	consumer, err := authenticatesession.New(db.lookup, fixtureClock{active.want.CreatedAt.Add(time.Hour)})
	if err != nil || !consumer.Execute(context.Background(), active.reference).IsValid() {
		tb.Fatal("synthetic valid snapshot was rejected by the existing session policy")
	}
	if result := consumer.Execute(context.Background(), revoked.reference); result.State != "denied" || result.Reason != "revoked_session" {
		tb.Fatal("existing session policy did not reject committed revocation")
	}
	if result := consumer.Execute(context.Background(), unknown); result.State != "denied" || result.Reason != "unknown_session" {
		tb.Fatal("existing session policy did not reject the unknown session")
	}
	return workloads
}

func (workload benchmarkWorkload) accepts(got sessionpolicy.Snapshot, err error) bool {
	if workload.canceled {
		return errors.Is(err, context.Canceled) && got == (sessionpolicy.Snapshot{})
	}
	return err == nil && snapshotEqual(got, workload.want)
}

func BenchmarkPostgresLookup(b *testing.B) {
	db := benchmarkDatabase(b)
	workloads := benchmarkWorkloads(b, db)
	for _, workload := range workloads {
		b.Run(workload.name, func(b *testing.B) {
			b.ReportAllocs()
			if workload.parallel {
				b.SetParallelism(5)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						got, err := db.lookup.Lookup(workload.ctx, workload.reference)
						if !workload.accepts(got, err) {
							b.Error("concurrent benchmark lookup changed session semantics")
							return
						}
					}
				})
				return
			}
			b.ResetTimer()
			for b.Loop() {
				got, err := db.lookup.Lookup(workload.ctx, workload.reference)
				if !workload.accepts(got, err) {
					b.Fatal("benchmark lookup changed session semantics")
				}
			}
		})
	}
}

type benchmarkPoolStats struct {
	MaxConnections        int32 `json:"max_connections"`
	TotalConnections      int32 `json:"total_connections"`
	AcquiredConnections   int32 `json:"acquired_connections"`
	IdleConnections       int32 `json:"idle_connections"`
	AcquireCount          int64 `json:"acquire_count"`
	AcquireDurationNS     int64 `json:"acquire_duration_ns"`
	EmptyAcquireCount     int64 `json:"empty_acquire_count"`
	CanceledAcquireCount  int64 `json:"canceled_acquire_count"`
	NewConnectionsCount   int64 `json:"new_connections_count"`
	DatabaseReaderClients int   `json:"database_reader_connections"`
}

func benchmarkPoolSnapshot(tb testing.TB, db testDatabase) benchmarkPoolStats {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var clients int
	if err := db.admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
WHERE datname=current_database() AND usename='hp_hit68_lookup'`).Scan(&clients); err != nil {
		tb.Fatal("could not count isolated runtime database connections")
	}
	stat := db.reader.Stat()
	if stat.TotalConns() > hit82PoolLimit || stat.MaxConns() != hit82PoolLimit || clients > hit82PoolLimit {
		tb.Fatal("runtime benchmark exceeded its five-connection bound")
	}
	return benchmarkPoolStats{MaxConnections: stat.MaxConns(), TotalConnections: stat.TotalConns(),
		AcquiredConnections: stat.AcquiredConns(), IdleConnections: stat.IdleConns(),
		AcquireCount: stat.AcquireCount(), AcquireDurationNS: int64(stat.AcquireDuration()),
		EmptyAcquireCount: stat.EmptyAcquireCount(), CanceledAcquireCount: stat.CanceledAcquireCount(),
		NewConnectionsCount: stat.NewConnsCount(), DatabaseReaderClients: clients}
}

type benchmarkRows struct {
	Count  int    `json:"count"`
	SHA256 string `json:"contents_xmin_ctid_sha256"`
}

func benchmarkRowSnapshot(tb testing.TB, db testDatabase) benchmarkRows {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := db.admin.Query(ctx, `SELECT row_to_json(s)::text||xmin::text||ctid::text
FROM hp_iam.sessions s ORDER BY session_id`)
	if err != nil {
		tb.Fatal("could not capture isolated session contents and row identity")
	}
	defer rows.Close()
	digest, count := sha256.New(), 0
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			tb.Fatal("could not read isolated session row identity")
		}
		_, _ = fmt.Fprintf(digest, "%d:%s\n", len(value), value)
		count++
	}
	if rows.Err() != nil {
		tb.Fatal("could not finish isolated session row identity capture")
	}
	return benchmarkRows{Count: count, SHA256: hex.EncodeToString(digest.Sum(nil))}
}

type benchmarkSamples struct {
	Scenario             string             `json:"scenario"`
	Count                int                `json:"count"`
	Workers              int                `json:"workers"`
	WarmupCount          int                `json:"warmup_count"`
	ElapsedNS            int64              `json:"elapsed_ns"`
	ThroughputOperations float64            `json:"throughput_operations_per_second"`
	P50NS                int64              `json:"p50_ns"`
	P95NS                int64              `json:"p95_ns"`
	P99NS                int64              `json:"p99_ns"`
	LatencyNS            []int64            `json:"latency_ns"`
	PoolBefore           benchmarkPoolStats `json:"pool_before"`
	PoolAfter            benchmarkPoolStats `json:"pool_after"`
	SemanticsVerified    bool               `json:"semantics_verified"`
}

func sampleBenchmarkWorkload(t *testing.T, db testDatabase, workload benchmarkWorkload) benchmarkSamples {
	t.Helper()
	const count, warmups = 1000, 500
	for range warmups {
		got, err := db.lookup.Lookup(workload.ctx, workload.reference)
		if !workload.accepts(got, err) {
			t.Fatal("warmup lookup changed session semantics: " + workload.name)
		}
	}
	result := benchmarkSamples{Scenario: workload.name, Count: count, Workers: 1,
		WarmupCount: warmups, LatencyNS: make([]int64, count), PoolBefore: benchmarkPoolSnapshot(t, db)}
	started := time.Now()
	if workload.parallel {
		result.Workers = hit82PoolLimit
		var workers sync.WaitGroup
		var failed atomic.Bool
		ready := make(chan struct{})
		for worker := range result.Workers {
			workers.Add(1)
			go func(worker int) {
				defer workers.Done()
				<-ready
				for i := worker; i < count; i += result.Workers {
					start := time.Now()
					got, err := db.lookup.Lookup(workload.ctx, workload.reference)
					result.LatencyNS[i] = time.Since(start).Nanoseconds()
					if !workload.accepts(got, err) {
						failed.Store(true)
					}
				}
			}(worker)
		}
		started = time.Now()
		close(ready)
		workers.Wait()
		result.ElapsedNS = time.Since(started).Nanoseconds()
		if failed.Load() {
			t.Fatal("parallel sampled lookup changed session semantics")
		}
	} else {
		for i := range count {
			start := time.Now()
			got, err := db.lookup.Lookup(workload.ctx, workload.reference)
			result.LatencyNS[i] = time.Since(start).Nanoseconds()
			if !workload.accepts(got, err) {
				t.Fatal("sampled lookup changed session semantics: " + workload.name)
			}
		}
		result.ElapsedNS = time.Since(started).Nanoseconds()
	}
	result.PoolAfter = benchmarkPoolSnapshot(t, db)
	result.ThroughputOperations = float64(count) * float64(time.Second) / float64(result.ElapsedNS)
	sorted := append([]int64(nil), result.LatencyNS...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	result.P50NS, result.P95NS, result.P99NS = sorted[499], sorted[949], sorted[989]
	result.SemanticsVerified = true
	return result
}

// TestHIT82LatencySamples writes measured, unsorted per-call observations only
// when the runner explicitly supplies an output directory. Fixed warmup, sample
// counts and workload order apply equally to the baseline and candidate build.
func TestHIT82LatencySamples(t *testing.T) {
	output := os.Getenv("HITPIPE_IAM_BENCH_OUTPUT_DIR")
	if output == "" {
		t.Skip("set HITPIPE_IAM_BENCH_OUTPUT_DIR to retain real latency observations")
	}
	db := benchmarkDatabase(t)
	workloads := benchmarkWorkloads(t, db)
	before := benchmarkRowSnapshot(t, db)
	var samples []benchmarkSamples
	for _, workload := range workloads {
		samples = append(samples, sampleBenchmarkWorkload(t, db, workload))
	}
	after := benchmarkRowSnapshot(t, db)
	if before != after {
		t.Fatal("benchmarked reads modified session contents or MVCC row identity")
	}
	report := struct {
		SchemaVersion               int                `json:"schema_version"`
		Task                        string             `json:"task"`
		PostgresVersionNumber       int                `json:"postgres_version_number"`
		GoVersion                   string             `json:"go_version"`
		GOOS                        string             `json:"goos"`
		GOARCH                      string             `json:"goarch"`
		GOMAXPROCS                  int                `json:"gomaxprocs"`
		PoolLimit                   int                `json:"pool_limit"`
		RuntimeReadOnly             bool               `json:"runtime_read_only_verified"`
		RuntimeNonprivileged        bool               `json:"runtime_nonprivileged_verified"`
		CommittedRevocationNextRead bool               `json:"committed_revocation_next_read_verified"`
		RowsBefore                  benchmarkRows      `json:"rows_before"`
		RowsAfter                   benchmarkRows      `json:"rows_after"`
		RowsUnchanged               bool               `json:"row_contents_and_identity_unchanged"`
		Samples                     []benchmarkSamples `json:"samples"`
	}{1, "HIT-82", 180007, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0),
		hit82PoolLimit, true, true, true, before, after, true, samples}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal("could not encode measured latency observations")
	}
	if err := os.MkdirAll(output, 0o700); err != nil {
		t.Fatal("could not create latency observation directory")
	}
	if err := os.WriteFile(filepath.Join(output, "latency-samples.json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal("could not retain latency observations")
	}
}
