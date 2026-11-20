//go:build integration

package runtimeaccess

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type database struct{ admin, app *pgxpool.Pool }

func fixtureDB(t *testing.T) database {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_authorization.py for isolated PostgreSQL 18.7 tests")
	}
	admin, e1 := pgxpool.ParseConfig(adminDSN)
	app, e2 := pgxpool.ParseConfig(appDSN)
	if e1 != nil || e2 != nil {
		t.Fatal("isolated fixture configuration invalid")
	}
	if admin.ConnConfig.Host != "127.0.0.1" || admin.ConnConfig.User != "postgres" ||
		!strings.HasPrefix(admin.ConnConfig.Database, "hit68_test_") || app.ConnConfig.User != "hp_hit81_app" ||
		app.ConnConfig.Host != admin.ConnConfig.Host || app.ConnConfig.Port != admin.ConnConfig.Port ||
		app.ConnConfig.Database != admin.ConnConfig.Database {
		t.Fatal("tests require dedicated loopback app and administrator logins")
	}
	newPool := func(cfg *pgxpool.Config) *pgxpool.Pool {
		cfg.MaxConns, cfg.MinConns = 5, 0
		p, err := pgxpool.NewWithConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal("fixture connection unavailable")
		}
		t.Cleanup(p.Close)
		return p
	}
	return database{admin: newPool(admin), app: newPool(app)}
}

func testUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("test randomness unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (d database) exec(t *testing.T, stmt string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := d.admin.Exec(ctx, stmt, args...); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			t.Fatalf("fixture command failed (SQLSTATE %s)", pg.SQLState())
		}
		t.Fatal("fixture command failed")
	}
}

func (d database) runtime(t *testing.T, service string) app.Registration {
	t.Helper()
	id := testUUID(t)
	t.Cleanup(func() { d.exec(t, `DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1`, id) })
	return app.Registration{RuntimeID: id, Service: service, BootID: testUUID(t),
		CertificateFingerprint: "sha256:" + strings.Repeat("a", 64), Generation: 1}
}

func (d database) denial(t *testing.T, scope, state string, workspace *string, epoch int64) app.Denial {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	id, subject := testUUID(t), testUUID(t)
	if scope == "WORKSPACE" && workspace != nil {
		subject = *workspace
	}
	var effective *time.Time
	if state == "EFFECTIVE" {
		at := now.Add(time.Microsecond)
		effective = &at
	}
	t.Cleanup(func() { d.exec(t, `DELETE FROM hp_iam.revocations WHERE revocation_id=$1`, id) })
	d.exec(t, `INSERT INTO hp_iam.revocations
	 (workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,effective_at,reason_code)
	 VALUES($1,$2,$3,$4,$5,$6::timestamptz,$6::timestamptz+interval '5 seconds',$7,$8,'TEST_RUNTIME_ACCESS')`,
		workspace, id, scope, subject, epoch, now, state, effective)
	return app.Denial{WorkspaceID: workspace, Scope: scope, SubjectID: subject, RevocationID: id,
		Epoch: epoch, AcceptedAt: now, EffectiveAt: effective}
}

func mustStore(t *testing.T, p *pgxpool.Pool, allowed ...app.Registration) *Store {
	t.Helper()
	s, err := New(p, allowed)
	if err != nil {
		t.Fatalf("valid runtime fixture not accepted: %v", err)
	}
	return s
}

func mustRegister(t *testing.T, s *Store, reg app.Registration) app.Snapshot {
	t.Helper()
	actual, err := s.Register(context.Background(), reg)
	if err != nil || actual.RuntimeID != reg.RuntimeID || actual.Generation != reg.Generation ||
		actual.BootID != reg.BootID || actual.Gate != syncing || actual.Revision <= 0 || actual.Denials == nil {
		t.Fatalf("expected the persisted tuple and a current syncing snapshot: %v", err)
	}
	return actual
}

func expectError(t *testing.T, snapshot app.Snapshot, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(snapshot, app.Snapshot{}) {
		t.Fatalf("expected an empty failure (%v), got %v", want, err)
	}
}

func (d database) persisted(t *testing.T, id string) (count int, revision int64, gate string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := d.admin.QueryRow(ctx, `SELECT count(*),
		coalesce((extract(epoch from max(last_observed_at))*1000000)::bigint,0),coalesce(min(status),'')
		FROM hp_iam.runtime_registry WHERE runtime_id=$1`, id).Scan(&count, &revision, &gate)
	if err != nil {
		t.Fatal("fixture registry inspection failed")
	}
	return count, revision, gate
}

func TestPostgresRuntimeAccessRosterAndOpen(t *testing.T) {
	d := fixtureDB(t)
	self, peer := d.runtime(t, "IAM"), d.runtime(t, "BFF")
	ctx := context.Background()
	s := mustStore(t, d.app, self, peer)
	duplicateBoot := peer
	duplicateBoot.BootID = strings.ToUpper(self.BootID)
	if _, err := New(d.app, []app.Registration{self, duplicateBoot}); !errors.Is(err, app.ErrInvalid) {
		t.Fatal("canonicalized duplicate boot must fail inventory validation")
	}
	wrong := self
	wrong.CertificateFingerprint = "sha256:" + strings.Repeat("b", 64)
	rejected, err := s.Register(ctx, wrong)
	expectError(t, rejected, err, app.ErrDenied)

	first := mustRegister(t, s, self)
	if first.Complete || len(first.Denials) != 0 {
		t.Fatal("partial roster and empty denial set expected")
	}
	rejected, err = s.OpenLocal(ctx, self, first)
	expectError(t, rejected, err, app.ErrStale)

	mustRegister(t, s, peer)
	full, err := s.Read(ctx, self)
	if err != nil || !full.Complete || full.Revision <= first.Revision {
		t.Fatal("exact peer registration should make the roster complete")
	}
	pending := d.denial(t, "SESSION", "PENDING", nil, 23)
	workspace := testUUID(t)
	effective := d.denial(t, "CONNECTION", "EFFECTIVE", &workspace, 71)
	incident := d.denial(t, "WORKSPACE", "INCIDENT", &workspace, 2)
	global := d.denial(t, "MEMBERSHIP", "PENDING", nil, 29)
	rejected, err = s.OpenLocal(ctx, self, full)
	expectError(t, rejected, err, app.ErrStale)
	fresh, err := s.Read(ctx, self)
	if err != nil || !fresh.Complete || fresh.Revision <= full.Revision || len(fresh.Denials) != 4 {
		t.Fatal("fresh snapshot must preserve pending, effective and incident denies")
	}
	want := map[string]app.Denial{pending.RevocationID: pending, effective.RevocationID: effective,
		incident.RevocationID: incident, global.RevocationID: global}
	for _, actual := range fresh.Denials {
		if !reflect.DeepEqual(actual, want[actual.RevocationID]) {
			t.Fatal("snapshot discarded a canonical denial fact")
		}
	}
	opened, err := s.OpenLocal(ctx, self, fresh)
	if err != nil || opened.Gate != open || opened.Revision <= fresh.Revision || !opened.Complete {
		t.Fatalf("proper local acknowledgement did not open its durable registry row: %v", err)
	}
	rejected, err = s.OpenLocal(ctx, self, fresh)
	expectError(t, rejected, err, app.ErrConflict)

	restarted := mustStore(t, d.app, self, peer)
	afterRestart := mustRegister(t, restarted, self)
	if afterRestart.Revision <= opened.Revision {
		t.Fatal("runtime snapshot revision must survive adapter restart")
	}
	rejected, err = restarted.OpenLocal(ctx, self, afterRestart)
	expectError(t, rejected, err, app.ErrConflict)

	unknown := d.runtime(t, "AI")
	mustRegister(t, mustStore(t, d.app, unknown), unknown)
	degraded, err := s.Read(ctx, self)
	if err != nil || degraded.Complete || degraded.Gate != syncing {
		t.Fatal("unexpected roster member must close the effective gate")
	}
	count, _, stored := d.persisted(t, self.RuntimeID)
	if count != 1 || stored != open {
		t.Fatal("remote registration/observations must not change durable OPEN ownership")
	}
}

func TestPostgresRuntimeAccessConcurrencyAndFencing(t *testing.T) {
	d := fixtureDB(t)
	base := d.runtime(t, "IAM")
	s := mustStore(t, d.app, base)
	const requests = 8
	var wg sync.WaitGroup
	type outcome struct {
		snap app.Snapshot
		err  error
	}
	results := make(chan outcome, requests)
	for range requests {
		wg.Go(func() {
			actual, err := s.Register(context.Background(), base)
			results <- outcome{actual, err}
		})
	}
	wg.Wait()
	close(results)
	seen := make(map[int64]bool, requests)
	for result := range results {
		if result.err != nil || result.snap.Gate != syncing || !result.snap.Complete ||
			seen[result.snap.Revision] || result.snap.Revision < 1 {
			t.Fatalf("concurrent repeat did not produce a single complete registration: %v", result.err)
		}
		seen[result.snap.Revision] = true
	}
	count, before, _ := d.persisted(t, base.RuntimeID)
	if count != 1 || !seen[before] {
		t.Fatal("each snapshot must have a distinct durable revision for the one logical runtime")
	}

	replacement := base
	replacement.BootID, replacement.Generation = testUUID(t), base.Generation+1
	newStore := mustStore(t, d.app, replacement)
	ctx := context.Background()
	rejected, err := newStore.Register(ctx, replacement)
	expectError(t, rejected, err, app.ErrConflict)
	d.exec(t, `UPDATE hp_iam.runtime_registry SET status='FENCED' WHERE runtime_id=$1`, base.RuntimeID)
	rejected, err = newStore.Register(ctx, replacement)
	expectError(t, rejected, err, app.ErrConflict)
	d.exec(t, `UPDATE hp_iam.runtime_registry SET fence_evidence_ref=$2 WHERE runtime_id=$1`, base.RuntimeID, testUUID(t))
	next := mustRegister(t, newStore, replacement)
	if !next.Complete || next.Revision <= before {
		t.Fatal("approved new boot must produce a fresh, still syncing snapshot")
	}
	rejected, err = s.Register(ctx, base)
	expectError(t, rejected, err, app.ErrStale)
}

func TestPostgresRuntimeAccessSQLFailures(t *testing.T) {
	d := fixtureDB(t)
	self := d.runtime(t, "IAM")
	ctx := context.Background()
	s := mustStore(t, d.app, self)
	rejected, err := mustStore(t, d.admin, self).Register(ctx, self)
	expectError(t, rejected, err, app.ErrUnavailable)
	if count, _, _ := d.persisted(t, self.RuntimeID); count != 0 {
		t.Fatal("administrative login must not acquire registration authority")
	}
	tx, err := d.admin.Begin(ctx)
	if err != nil {
		t.Fatal("isolated cancellation fixture unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.runtime_registry IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		t.Fatal("isolated cancellation fixture lock unavailable")
	}
	blocked, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	rejected, err = s.Register(blocked, self)
	cancel()
	expectError(t, rejected, err, app.ErrUnavailable)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("isolated cancellation fixture rollback failed")
	}
	if count, _, _ := d.persisted(t, self.RuntimeID); count != 0 {
		t.Fatal("cancelled registration changed durable state")
	}

	// A disposable fixture defers its failure to actual PostgreSQL COMMIT.
	d.exec(t, `CREATE FUNCTION public.hit115_commit_failure() RETURNS trigger LANGUAGE plpgsql AS
		'BEGIN RAISE EXCEPTION USING MESSAGE = ''Synthetic private value not for application diagnostics''; END'`)
	t.Cleanup(func() { d.exec(t, `DROP FUNCTION public.hit115_commit_failure()`) })
	d.exec(t, `CREATE CONSTRAINT TRIGGER hit115_commit_failure AFTER UPDATE ON hp_iam.runtime_registry
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.hit115_commit_failure()`)
	t.Cleanup(func() { d.exec(t, `DROP TRIGGER hit115_commit_failure ON hp_iam.runtime_registry`) })
	rejected, err = s.Register(ctx, self)
	expectError(t, rejected, err, app.ErrUnavailable)
	if count, _, _ := d.persisted(t, self.RuntimeID); count != 0 {
		t.Fatal("a failed COMMIT must roll back the registration and expose no snapshot")
	}

	d.app.Close()
	rejected, err = s.Register(ctx, self)
	expectError(t, rejected, err, app.ErrUnavailable)
}

func TestPostgresRuntimeAccessDenialOverflow(t *testing.T) {
	d := fixtureDB(t)
	self := d.runtime(t, "IAM")
	s := mustStore(t, d.app, self)
	t.Cleanup(func() { d.exec(t, `DELETE FROM hp_iam.revocations WHERE reason_code='TEST_RUNTIME_ACCESS_OVERFLOW'`) })
	d.exec(t, `INSERT INTO hp_iam.revocations
		(revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
		SELECT md5('hit115-overflow'||i::text)::uuid,'SESSION',md5('subject'||i::text)::uuid,1,
			statement_timestamp(),statement_timestamp()+interval '5 seconds','PENDING','TEST_RUNTIME_ACCESS_OVERFLOW'
		FROM generate_series(1,$1) AS i`, maxDenials+1)
	rejected, err := s.Register(context.Background(), self)
	expectError(t, rejected, err, app.ErrUnavailable)
	if count, _, _ := d.persisted(t, self.RuntimeID); count != 0 {
		t.Fatal("a truncated denial collection must not persist or expose a registration")
	}
}
