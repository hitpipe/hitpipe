//go:build integration

package sessionlookup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testDatabase struct {
	admin  *pgxpool.Pool
	reader *pgxpool.Pool
	lookup *Lookup
}

func database(t *testing.T) testDatabase {
	t.Helper()
	adminDSN, readerDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_DSN")
	if adminDSN == "" || readerDSN == "" {
		t.Skip("run tools/check_postgres_session_lookup.py for isolated PostgreSQL tests")
	}
	adminConfig, adminErr := pgxpool.ParseConfig(adminDSN)
	readerConfig, readerErr := pgxpool.ParseConfig(readerDSN)
	if adminErr != nil || readerErr != nil {
		t.Fatal("invalid test database configuration; connection values are withheld")
	}
	for _, cfg := range []*pgxpool.Config{adminConfig, readerConfig} {
		if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") {
			t.Fatal("integration tests require an isolated loopback hit68_test_ database")
		}
		cfg.MaxConns, cfg.MinConns = 5, 0
	}
	if readerConfig.ConnConfig.User != "hp_hit68_lookup" ||
		readerConfig.ConnConfig.Database != adminConfig.ConnConfig.Database ||
		readerConfig.ConnConfig.Port != adminConfig.ConnConfig.Port {
		t.Fatal("reader and fixture administrator must use the same isolated test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("could not configure fixture administrator")
	}
	t.Cleanup(admin.Close)
	reader, err := pgxpool.NewWithConfig(ctx, readerConfig)
	if err != nil {
		t.Fatal("could not configure IAM reader")
	}
	t.Cleanup(reader.Close)
	var databaseName, currentUser, readOnly string
	var elevated, iamRole bool
	err = reader.QueryRow(ctx, `SELECT current_database(),current_user,current_setting('transaction_read_only'),
EXISTS(SELECT 1 FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole)),
pg_has_role(current_user,'hp_iam_app','MEMBER')`).Scan(&databaseName, &currentUser, &readOnly, &elevated, &iamRole)
	if err != nil || databaseName != readerConfig.ConnConfig.Database || currentUser != "hp_hit68_lookup" ||
		readOnly != "on" || elevated || !iamRole {
		t.Fatal("test runtime is not a nonprivileged read-only IAM role")
	}
	lookup, err := New(reader)
	if err != nil {
		t.Fatal("could not construct the bounded IAM lookup")
	}
	return testDatabase{admin: admin, reader: reader, lookup: lookup}
}

type fixture struct {
	reference string
	secret    string
	id        string
	want      sessionpolicy.Snapshot
}

func uuid(t *testing.T) string {
	t.Helper()
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		t.Fatal("test random source unavailable")
	}
	bytes[6], bytes[8] = bytes[6]&0x0f|0x40, bytes[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

func (db testDatabase) seed(t *testing.T) fixture {
	t.Helper()
	id, principal := uuid(t), uuid(t)
	secret := "HIT68-SYNTHETIC-SECRET-" + strings.ReplaceAll(id, "-", "")
	ref, err := Reference(secret)
	if err != nil {
		t.Fatal("synthetic secret does not meet the wire length bound")
	}
	digest := sha256.Sum256([]byte(secret))
	csrf := sha256.Sum256([]byte("HIT68-SYNTHETIC-CSRF-" + id))
	created := time.Date(2026, 11, 12, 0, 0, 0, 123456000, time.UTC)
	want := sessionpolicy.Snapshot{Found: true, CreatedAt: created,
		ExpiresAt: created.Add(24 * time.Hour), IdleExpiresAt: created.Add(8 * time.Hour)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.admin.Exec(ctx, `INSERT INTO hp_iam.principals
(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://issuer.invalid',$2,$3)`, principal, id, created)
	if err != nil {
		t.Fatal("could not create isolated principal fixture")
	}
	_, err = db.admin.Exec(ctx, `INSERT INTO hp_iam.sessions
(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
VALUES($1,$2,$3,$4,$5,$6,$7,1)`, id, digest[:], principal, csrf[:], created, want.ExpiresAt, want.IdleExpiresAt)
	if err != nil {
		t.Fatal("could not create canonical isolated session fixture")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.admin.Exec(ctx, "DELETE FROM hp_iam.sessions WHERE session_id=$1", id); err != nil {
			t.Error("fixture session cleanup failed")
		}
		if _, err := db.admin.Exec(ctx, "DELETE FROM hp_iam.principals WHERE principal_id=$1", principal); err != nil {
			t.Error("fixture principal cleanup failed")
		}
	})
	return fixture{reference: ref, secret: secret, id: id, want: want}
}

func snapshotEqual(left, right sessionpolicy.Snapshot) bool {
	return left.Found == right.Found && left.Revoked == right.Revoked &&
		left.CreatedAt.Equal(right.CreatedAt) && left.ExpiresAt.Equal(right.ExpiresAt) &&
		left.IdleExpiresAt.Equal(right.IdleExpiresAt)
}

// This clock is a fixed test double, never a claim about host clock health.
type fixtureClock struct{ at time.Time }

func (clock fixtureClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: clock.at, Trusted: true}, nil
}

func TestPostgresExactSnapshotAndExistingConsumer(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	got, err := db.lookup.Lookup(context.Background(), f.reference)
	if err != nil || !snapshotEqual(got, f.want) {
		t.Fatal("lookup did not return the exact minimal stored snapshot")
	}
	consumer, err := authenticatesession.New(db.lookup, fixtureClock{f.want.CreatedAt.Add(time.Hour)})
	if err != nil || !consumer.Execute(context.Background(), f.reference).IsValid() {
		t.Fatal("the unchanged consumer could not use the real PostgreSQL adapter")
	}
	// The reference is internal. A public UUID or the opaque cookie is not it.
	for _, invalid := range []string{f.id, f.secret, strings.ToUpper(f.reference)} {
		got, err := db.lookup.Lookup(context.Background(), invalid)
		if err == nil || got != (sessionpolicy.Snapshot{}) {
			t.Fatal("an alternate identity was accepted as an internal reference")
		}
	}
}

func TestPostgresMissingSessionIsNotFound(t *testing.T) {
	db := database(t)
	missing, _ := Reference(strings.Repeat("missing-synthetic-", 4))
	got, err := db.lookup.Lookup(context.Background(), missing)
	if err != nil || got != (sessionpolicy.Snapshot{}) {
		t.Fatal("missing session must be a zero Found=false snapshot without a database error")
	}
	consumer, _ := authenticatesession.New(db.lookup, fixtureClock{time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)})
	result := consumer.Execute(context.Background(), missing)
	if result.State != "denied" || result.Reason != "unknown_session" {
		t.Fatal("missing session became a valid or unavailable session")
	}
}

func TestPostgresNextReadObservesRevocation(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	first, err := db.lookup.Lookup(context.Background(), f.reference)
	if err != nil || !first.Found || first.Revoked {
		t.Fatal("initial fixture was not active")
	}
	if _, err := db.admin.Exec(context.Background(), "UPDATE hp_iam.sessions SET revoked_at=$1 WHERE session_id=$2",
		f.want.CreatedAt.Add(time.Minute), f.id); err != nil {
		t.Fatal("fixture administrator could not revoke the session")
	}
	next, err := db.lookup.Lookup(context.Background(), f.reference)
	f.want.Revoked = true
	if err != nil || !snapshotEqual(next, f.want) {
		t.Fatal("the next read hid a committed revocation or changed stored deadlines")
	}
	consumer, _ := authenticatesession.New(db.lookup, fixtureClock{f.want.CreatedAt.Add(time.Hour)})
	if result := consumer.Execute(context.Background(), f.reference); result.State != "denied" || result.Reason != "revoked_session" {
		t.Fatal("existing policy accepted a revoked session")
	}
}

func sessionRows(t *testing.T, admin *pgxpool.Pool) []string {
	t.Helper()
	rows, err := admin.Query(context.Background(), `SELECT row_to_json(s)::text||xmin::text||ctid::text
FROM hp_iam.sessions s ORDER BY session_id`)
	if err != nil {
		t.Fatal("could not capture the fixture's data and row versions")
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal("could not read fixture row version")
		}
		result = append(result, value)
	}
	if rows.Err() != nil {
		t.Fatal("could not finish capturing fixture row versions")
	}
	return result
}

func TestPostgresReadsNeverModifyOrRenewSession(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	before := sessionRows(t, db.admin)
	for range 10 {
		if _, err := db.lookup.Lookup(context.Background(), f.reference); err != nil {
			t.Fatal("read failed under the runtime's read-only transaction policy")
		}
	}
	if !reflect.DeepEqual(before, sessionRows(t, db.admin)) {
		t.Fatal("lookup changed session contents or MVCC row identity")
	}
}

func TestPostgresErrorsAreSanitizedAndNeverValid(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	if _, err := db.admin.Exec(context.Background(), "REVOKE SELECT ON hp_iam.sessions FROM hp_iam_app"); err != nil {
		t.Fatal("could not install the controlled permission failure")
	}
	t.Cleanup(func() {
		if _, err := db.admin.Exec(context.Background(), "GRANT SELECT ON hp_iam.sessions TO hp_iam_app"); err != nil {
			t.Error("could not restore fixture permissions")
		}
	})
	got, err := db.lookup.Lookup(context.Background(), f.reference)
	if !errors.Is(err, ErrUnavailable) || got != (sessionpolicy.Snapshot{}) {
		t.Fatal("a database failure must return only the bounded unavailable error and a zero snapshot")
	}
	if strings.Contains(err.Error(), f.secret) || strings.Contains(err.Error(), f.reference) ||
		strings.Contains(err.Error(), f.id) || strings.Contains(err.Error(), "hp_iam") {
		t.Fatal("diagnostic disclosed sensitive query or identity details")
	}
	consumer, _ := authenticatesession.New(db.lookup, fixtureClock{f.want.CreatedAt.Add(time.Hour)})
	if result := consumer.Execute(context.Background(), f.reference); result.State != "unavailable" || result.Reason != "lookup_unavailable" {
		t.Fatal("database failure became an authenticated session")
	}
}

func lockSessions(t *testing.T, admin *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := admin.Begin(context.Background())
	if err != nil {
		t.Fatal("could not begin controlled fixture lock")
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(context.Background(), "LOCK TABLE hp_iam.sessions IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal("could not install controlled fixture lock")
	}
	return tx
}

func TestPostgresInFlightCancellationFailsClosed(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	tx := lockSessions(t, db.admin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type answer struct {
		snapshot sessionpolicy.Snapshot
		err      error
	}
	done := make(chan answer, 1)
	go func() {
		snapshot, err := db.lookup.Lookup(ctx, f.reference)
		done <- answer{snapshot, err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		var waiting bool
		if err := db.admin.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
WHERE datname=current_database() AND usename='hp_hit68_lookup' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal("could not observe the controlled blocked read")
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the controlled read never reached the database lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) || result.snapshot != (sessionpolicy.Snapshot{}) {
			t.Fatal("in-flight cancellation returned session data or lost cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled database read did not finish promptly")
	}
	_ = tx.Rollback(context.Background())
	if got, err := db.lookup.Lookup(context.Background(), f.reference); err != nil || !snapshotEqual(got, f.want) {
		t.Fatal("lookup did not recover safely after cancelling a read")
	}
}

func TestPostgresInternalDeadlineBoundsBlockedRead(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	tx := lockSessions(t, db.admin)
	started := time.Now()
	got, err := db.lookup.Lookup(context.Background(), f.reference)
	if !errors.Is(err, context.DeadlineExceeded) || got != (sessionpolicy.Snapshot{}) || time.Since(started) > 5*time.Second {
		t.Fatal("a blocked database read escaped the bounded fail-closed deadline")
	}
	_ = tx.Rollback(context.Background())
}

func TestPostgresConcurrentReadsReturnIndependentValues(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	var workers sync.WaitGroup
	failed := make(chan bool, 16)
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 4 {
				got, err := db.lookup.Lookup(context.Background(), f.reference)
				if err != nil || !snapshotEqual(got, f.want) {
					failed <- true
					return
				}
				got.Revoked = true // A caller cannot mutate the next caller's snapshot.
			}
		}()
	}
	workers.Wait()
	close(failed)
	if len(failed) != 0 {
		t.Fatal("concurrent reads shared mutable state or failed")
	}
	if got, err := db.lookup.Lookup(context.Background(), f.reference); err != nil || !snapshotEqual(got, f.want) {
		t.Fatal("caller mutation escaped into stored or shared state")
	}
}

func TestPostgresCancelledContextAndClosedPool(t *testing.T) {
	db := database(t)
	f := db.seed(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := db.lookup.Lookup(ctx, f.reference)
	if !errors.Is(err, context.Canceled) || got != (sessionpolicy.Snapshot{}) {
		t.Fatal("a cancelled request produced session data")
	}
	db.reader.Close()
	got, err = db.lookup.Lookup(context.Background(), f.reference)
	if !errors.Is(err, ErrUnavailable) || got != (sessionpolicy.Snapshot{}) {
		t.Fatal("closed pool did not fail safely")
	}
	if got, err := db.lookup.Lookup(nil, hex.EncodeToString(make([]byte, 32))); err == nil || got != (sessionpolicy.Snapshot{}) {
		t.Fatal("nil context must not cause a panic or valid session")
	}
}
