//go:build integration

package workspaces

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testClock struct{}

func (testClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: true}, nil
}

type fixture struct {
	t          *testing.T
	admin, iam *pgxpool.Pool
	store      *Store
	now        time.Time
	principal  string
	other      string
	session    string
	reference  string
	caller     runtimeaccess.Registration
	local      runtimeaccess.Registration
	csrf       [32]byte
}

func testID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("test randomness unavailable")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func isolatedFixture(t *testing.T) fixture {
	t.Helper()
	adminDSN, iamDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || iamDSN == "" {
		t.Skip("run the isolated PostgreSQL 18.7 IAM verification")
	}
	admin, adminErr := pgxpool.ParseConfig(adminDSN)
	iam, iamErr := pgxpool.ParseConfig(iamDSN)
	if adminErr != nil || iamErr != nil {
		t.Fatal("isolated fixture configuration invalid")
	}
	if admin.ConnConfig.Host != "127.0.0.1" || admin.ConnConfig.User != "postgres" ||
		!strings.HasPrefix(admin.ConnConfig.Database, "hit68_test_") || iam.ConnConfig.User != "hp_hit81_app" ||
		iam.ConnConfig.Host != admin.ConnConfig.Host || iam.ConnConfig.Port != admin.ConnConfig.Port ||
		iam.ConnConfig.Database != admin.ConnConfig.Database {
		t.Fatal("fixture requires dedicated loopback app and administrator logins")
	}
	pool := func(config *pgxpool.Config) *pgxpool.Pool {
		config.MaxConns, config.MinConns = 5, 0
		pool, err := pgxpool.NewWithConfig(context.Background(), config)
		if err != nil {
			t.Fatal("fixture pool unavailable")
		}
		t.Cleanup(pool.Close)
		return pool
	}
	f := fixture{t: t, admin: pool(admin), iam: pool(iam), now: time.Now().UTC().Truncate(time.Microsecond),
		principal: testID(t), other: testID(t), session: testID(t),
		caller: runtimeaccess.Registration{RuntimeID: testID(t), Service: "BFF", Generation: 1,
			BootID: testID(t), CertificateFingerprint: "sha256:" + strings.Repeat("a", 64)},
		local: runtimeaccess.Registration{RuntimeID: testID(t), Service: "IAM", Generation: 1,
			BootID: testID(t), CertificateFingerprint: "sha256:" + strings.Repeat("b", 64)}}
	var server string
	if f.admin.QueryRow(context.Background(), `SHOW server_version_num`).Scan(&server) != nil || server != "180007" {
		t.Fatal("fixture database must be PostgreSQL 18.7")
	}
	var err error
	f.store, err = New(f.iam, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.exec(`DELETE FROM hp_iam.runtime_registry WHERE runtime_id IN ($1,$2)`, f.caller.RuntimeID, f.local.RuntimeID)
		f.exec(`DELETE FROM hp_iam.sessions WHERE session_id=$1`, f.session)
		f.exec(`DELETE FROM hp_iam.principals WHERE principal_id IN($1,$2)`, f.principal, f.other)
	})
	f.exec(`INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
 VALUES($1,'urn:hit124-test-issuer',$1::uuid::text,$3),($2,'urn:hit124-test-issuer',$2::uuid::text,$3)`,
		f.principal, f.other, f.now)
	secret := testID(t) + testID(t)
	hash := sha256.Sum256([]byte(secret))
	f.reference = hex.EncodeToString(hash[:])
	if ref, err := sessionlookup.Reference(secret); err != nil || ref != f.reference {
		t.Fatal("fixture reference inconsistent with session consumer")
	}
	f.csrf = sha256.Sum256([]byte(testID(t)))
	f.exec(`INSERT INTO hp_iam.sessions
 (session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
 VALUES($1,$2,$3,$4,$5,$6,$7,1)`, f.session, hash[:], f.principal, f.csrf[:],
		f.now.Add(-time.Minute), f.now.Add(time.Hour), f.now.Add(30*time.Minute))
	for _, runtime := range []runtimeaccess.Registration{f.caller, f.local} {
		f.exec(`INSERT INTO hp_iam.runtime_registry
 (runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at)
 VALUES($1,$2,$3,$4,$5,'OPEN',$6,$6)`, runtime.RuntimeID, runtime.Service, runtime.Generation,
			runtime.BootID, runtime.CertificateFingerprint, f.now)
	}
	return f
}

func (f fixture) exec(query string, args ...any) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := f.admin.Exec(ctx, query, args...); err != nil {
		var failure *pgconn.PgError
		if errors.As(err, &failure) {
			f.t.Fatalf("isolated fixture statement failed (SQLSTATE %s)", failure.Code)
		}
		f.t.Fatal("isolated fixture statement failed")
	}
}

func (f fixture) identity() app.Identity {
	f.t.Helper()
	who, err := f.store.Identity(context.Background(), f.reference, f.caller, f.local)
	if err != nil || who.PrincipalID != f.principal || who.SessionID != f.session ||
		who.SessionEpoch != 1 || who.CSRFHash != f.csrf || !who.CreatedAt.Equal(f.now.Add(-time.Minute)) ||
		!who.ExpiresAt.Equal(f.now.Add(time.Hour)) || !who.ValidUntil.Equal(f.now.Add(30*time.Minute)) {
		f.t.Fatalf("current session identity was not recovered: %v", err)
	}
	return who
}

func (f fixture) workspace() string {
	f.t.Helper()
	id := testID(f.t)
	f.t.Cleanup(func() {
		f.exec(`DELETE FROM hp_iam.outbox WHERE workspace_id=$1`, id)
		f.exec(`DELETE FROM hp_iam.audit_minimal WHERE workspace_id=$1`, id)
		f.exec(`DELETE FROM hp_iam.command_receipts WHERE workspace_id=$1`, id)
		f.exec(`DELETE FROM hp_iam.memberships WHERE workspace_id=$1`, id)
		f.exec(`DELETE FROM hp_iam.workspaces WHERE workspace_id=$1`, id)
		f.exec(`DELETE FROM hp_iam.revocations WHERE workspace_id=$1`, id)
	})
	return id
}

type fixtureChange struct {
	app.Change
	material app.Material
}

func (change fixtureChange) prepare(context.Context) (app.Material, error) {
	return change.material, nil
}

func (f fixture) change() fixtureChange {
	f.t.Helper()
	change := app.Change{WorkspaceID: f.workspace(), RequestID: testID(f.t), BodyDigest: "sha256:" + strings.Repeat("b", 64),
		AuditAction: "TEST_WORKSPACE_CREATE", AuditReason: "TEST_EXPLICIT_APPROVAL", MessageKind: "TEST_WORKSPACE_CREATED",
		Lifecycle: "ACTIVE", OwnerConfirmedAt: f.now.Add(-time.Second), PayloadExpiresAt: f.now.Add(time.Hour)}
	material := app.Material{NameRef: testID(f.t), AuditID: testID(f.t), MessageID: testID(f.t),
		MessageRef: testID(f.t), MessageDigest: "sha256:" + strings.Repeat("c", 64)}
	return fixtureChange{Change: change, material: material}
}

func (f fixture) counts(id string) [5]int {
	f.t.Helper()
	var counts [5]int
	err := f.admin.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.memberships WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.command_receipts WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.audit_minimal WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.outbox WHERE workspace_id=$1)`, id).
		Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4])
	if err != nil {
		f.t.Fatal("isolated durable effects could not be inspected")
	}
	return counts
}

func TestPostgresWorkspacesAtomicCreateAndReplay(t *testing.T) {
	f := isolatedFixture(t)
	ctx, who, change := context.Background(), f.identity(), f.change()
	type result struct {
		row app.Row
		err error
	}
	start := make(chan struct{})
	outcomes := make(chan result, 2)
	var prepared atomic.Int32
	prepare := func(ctx context.Context) (app.Material, error) {
		prepared.Add(1)
		return change.prepare(ctx)
	}
	for range 2 {
		go func() {
			<-start
			row, err := f.store.Create(ctx, who, change.Change, prepare)
			outcomes <- result{row, err}
		}()
	}
	close(start)
	first, second := <-outcomes, <-outcomes
	if first.err != nil || second.err != nil || first.row != second.row || first.row.WorkspaceID != change.WorkspaceID ||
		first.row.NameRef != change.material.NameRef || first.row.Role != "OWNER" || first.row.Lifecycle != "ACTIVE" ||
		first.row.SecurityEpoch != 1 || first.row.Revision != 1 {
		t.Fatalf("concurrent commands did not recover the same first owner workspace: %v, %v", first.err, second.err)
	}
	if f.counts(change.WorkspaceID) != [5]int{1, 1, 1, 1, 1} {
		t.Fatal("workspace, owner, receipt, audit and outbox must exist exactly once")
	}
	if prepared.Load() != 1 {
		t.Fatal("content staging was not serialized with the exact command")
	}
	var exact bool
	err := f.admin.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM hp_iam.workspaces w
 JOIN hp_iam.memberships m USING(workspace_id)
 JOIN hp_iam.command_receipts c USING(workspace_id)
 JOIN hp_iam.audit_minimal a USING(workspace_id)
 JOIN hp_iam.outbox o USING(workspace_id)
 WHERE w.workspace_id=$1 AND w.name_content_ref=$2 AND w.owner_confirmed_at=$3 AND w.initial_activation_at IS NULL
   AND m.principal_id=$4 AND m.role='OWNER' AND m.status='ACTIVE' AND m.revision=1
   AND c.actor_id=$4 AND c.request_id=$5 AND c.body_digest=$6 AND c.result_id=$1
   AND a.actor_id=$4 AND a.action_code=$7 AND o.payload_content_ref=$8 AND o.digest=$9 AND o.state='PENDING')`,
		change.WorkspaceID, change.material.NameRef, change.OwnerConfirmedAt, f.principal,
		change.RequestID, change.BodyDigest, change.AuditAction, change.material.MessageRef, change.material.MessageDigest).Scan(&exact)
	if err != nil || !exact {
		t.Fatal("durable state does not contain the exact authorized effect")
	}
	restarted, err := New(f.iam, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	// Treat the original response as lost: only the stable scope and receipt
	// may recover the result in this new adapter instance.
	mustNotPrepare := func(context.Context) (app.Material, error) {
		t.Fatal("existing receipt must not stage new owner content")
		return app.Material{}, app.ErrUnavailable
	}
	replayed, err := restarted.Create(ctx, who, change.Change, mustNotPrepare)
	if err != nil || replayed != first.row {
		t.Fatalf("durable replay failed: %v", err)
	}
	change.BodyDigest = "sha256:" + strings.Repeat("d", 64)
	if row, err := restarted.Create(ctx, who, change.Change, mustNotPrepare); !errors.Is(err, app.ErrConflict) || row != (app.Row{}) {
		t.Fatalf("different body was not a bounded conflict: %v", err)
	}
}

func TestPostgresWorkspacesRollbackAndApplicationRole(t *testing.T) {
	f := isolatedFixture(t)
	ctx, who, change := context.Background(), f.identity(), f.change()
	failedPreparation := func(context.Context) (app.Material, error) { return app.Material{}, app.ErrUnavailable }
	if _, err := f.store.Create(ctx, who, change.Change, failedPreparation); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("content failure did not abort creation: %v", err)
	}
	if f.counts(change.WorkspaceID) != [5]int{} {
		t.Fatal("failed content preparation left durable SQL effects")
	}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelPreparation := func(context.Context) (app.Material, error) {
		cancel()
		return change.material, nil
	}
	if _, err := f.store.Create(cancelled, who, change.Change, cancelPreparation); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("cancelled content preparation did not stop before SQL: %v", err)
	}
	if f.counts(change.WorkspaceID) != [5]int{} {
		t.Fatal("cancelled preparation left durable SQL effects")
	}
	// Force the final insert to violate its actual PostgreSQL key. Earlier
	// workspace, membership, receipt and audit inserts must all roll back.
	f.exec(`INSERT INTO hp_iam.outbox
 (workspace_id,message_id,digest,kind,payload_content_ref,payload_expires_at,created_at,state,next_attempt_at)
 VALUES($1,$2,$3,'TEST_COLLISION',$4,$5,$6,'PENDING',$6)`,
		change.WorkspaceID, change.material.MessageID, change.material.MessageDigest, change.material.MessageRef, change.PayloadExpiresAt, f.now)
	if row, err := f.store.Create(ctx, who, change.Change, change.prepare); !errors.Is(err, app.ErrUnavailable) || row != (app.Row{}) {
		t.Fatalf("SQL error should be a bounded failure before commit: %v", err)
	}
	if f.counts(change.WorkspaceID) != [5]int{0, 0, 0, 0, 1} {
		t.Fatal("SQL failure left partial durable creation")
	}
	f.exec(`DELETE FROM hp_iam.outbox WHERE workspace_id=$1 AND message_id=$2`, change.WorkspaceID, change.material.MessageID)
	if _, err := f.store.Create(ctx, who, change.Change, change.prepare); err != nil {
		t.Fatalf("retry after verified rollback did not succeed: %v", err)
	}
	var ordinary int
	if f.iam.QueryRow(ctx, `SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id=$1`, change.WorkspaceID).Scan(&ordinary) != nil || ordinary != 0 {
		t.Fatal("runtime pool must have no cross-scope visibility after transaction finishes")
	}
	unapproved, err := New(f.admin, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unapproved.Identity(ctx, f.reference, f.caller, f.local); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("administrator must not serve owner facts")
	}
}

func (f fixture) candidate(actor, lifecycle, membership string, epoch int64) string {
	f.t.Helper()
	id := f.workspace()
	f.exec(`INSERT INTO hp_iam.workspaces
 (workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at) VALUES($1,$2,1,$3,$4,$5)`,
		id, testID(f.t), epoch, lifecycle, f.now)
	f.exec(`INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision)
 VALUES($1,$2,'READER',$3,1)`, id, actor, membership)
	return id
}

func (f fixture) revoke(scope, subject string, workspace *string) {
	f.t.Helper()
	id := testID(f.t)
	f.t.Cleanup(func() { f.exec(`DELETE FROM hp_iam.revocations WHERE revocation_id=$1`, id) })
	f.exec(`INSERT INTO hp_iam.revocations
 (workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,$2,$3,$4,1,$5::timestamptz,$5::timestamptz+interval '5 seconds','PENDING','TEST_SCOPE')`,
		workspace, id, scope, subject, f.now)
}

func TestPostgresWorkspacesCurrentOwnFactsAndLocks(t *testing.T) {
	f := isolatedFixture(t)
	ctx, who := context.Background(), f.identity()
	ours := f.candidate(f.principal, "ACTIVE", "ACTIVE", 9)
	foreign := f.candidate(f.other, "ACTIVE", "ACTIVE", 9)
	revoked := f.candidate(f.principal, "ACTIVE", "REVOKED", 9)
	invited := f.candidate(f.principal, "ACTIVE", "INVITED", 9)
	onboarding := f.candidate(f.principal, "ONBOARDING", "ACTIVE", 9)
	deleting := f.candidate(f.principal, "DELETING", "ACTIVE", 9)
	deleted := f.candidate(f.principal, "DELETED", "ACTIVE", 9)
	unsafeEpoch := f.candidate(f.principal, "ACTIVE", "ACTIVE", 9007199254740992)
	blocked := f.candidate(f.principal, "ACTIVE", "ACTIVE", 9)
	f.revoke("MEMBERSHIP", testID(t), &blocked)
	ids := []string{foreign, ours, revoked, onboarding, deleting, deleted, invited, unsafeEpoch, blocked, testID(t)}
	err := f.store.WithRows(ctx, who, ids, func(guard context.Context, rows []app.Row) error {
		got := make([]string, 0, len(rows))
		for _, row := range rows {
			got = append(got, row.WorkspaceID)
		}
		if !slices.Equal(got, []string{ours}) || rows[0].SecurityEpoch != 9 || rows[0].MembershipRevision != 1 || rows[0].Role != "READER" {
			t.Fatal("candidate scope became authority or stale facts were not closed")
		}
		// A concurrent lifecycle or authorization update cannot change the
		// checked membership while the actual response start is in progress.
		tx, err := f.admin.Begin(guard)
		if err != nil {
			t.Fatal("fixture transaction unavailable")
		}
		defer func() { _ = tx.Rollback(guard) }()
		if _, err := tx.Exec(guard, `SET LOCAL lock_timeout='100ms'`); err != nil {
			t.Fatal("fixture lock timeout unavailable")
		}
		_, err = tx.Exec(guard, `UPDATE hp_iam.memberships SET revision=revision+1 WHERE workspace_id=$1 AND principal_id=$2`, ours, f.principal)
		var locked *pgconn.PgError
		if !errors.As(err, &locked) || locked.Code != "55P03" {
			t.Fatal("membership changed before the guarded response began")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("current facts not available: %v", err)
	}
	f.revoke("WORKSPACE", ours, nil)
	if err := f.store.WithRows(ctx, who, []string{ours}, func(_ context.Context, rows []app.Row) error {
		if len(rows) != 0 {
			t.Fatal("exact global workspace denial did not block its scope")
		}
		return nil
	}); err != nil {
		t.Fatalf("global workspace denial read failed: %v", err)
	}
	f.exec(`UPDATE hp_iam.runtime_registry SET status='FENCED' WHERE runtime_id=$1`, f.local.RuntimeID)
	if _, err := f.store.Identity(ctx, f.reference, f.caller, f.local); !errors.Is(err, app.ErrStale) {
		t.Fatalf("fenced local IAM boot remained authorized: %v", err)
	}
	f.exec(`UPDATE hp_iam.runtime_registry SET status='OPEN' WHERE runtime_id=$1`, f.local.RuntimeID)
	f.exec(`UPDATE hp_iam.sessions SET epoch=2 WHERE session_id=$1`, f.session)
	check := func(context.Context, []app.Row) error {
		t.Fatal("old actor facts must not start a protected response")
		return nil
	}
	if err := f.store.WithRows(ctx, who, []string{ours}, check); !errors.Is(err, app.ErrStale) {
		t.Fatalf("session epoch change was not detected: %v", err)
	}
	f.revoke("SESSION", f.session, nil)
	if _, err := f.store.Identity(ctx, f.reference, f.caller, f.local); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatalf("committed session denial did not close identity: %v", err)
	}
	f.exec(`UPDATE hp_iam.runtime_registry SET status='FENCED' WHERE runtime_id=$1`, f.caller.RuntimeID)
	if _, err := f.store.Identity(ctx, f.reference, f.caller, f.local); !errors.Is(err, app.ErrDenied) {
		t.Fatalf("fenced runtime remained authorized: %v", err)
	}
}
