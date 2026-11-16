//go:build integration

package authorization

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authorityDatabase struct {
	admin, app *pgxpool.Pool
	reader     *Reader
}

func authorityDB(t *testing.T) authorityDatabase {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_authorization.py for isolated PostgreSQL 18.7 tests")
	}
	adminCfg, adminErr := pgxpool.ParseConfig(adminDSN)
	appCfg, appErr := pgxpool.ParseConfig(appDSN)
	if adminErr != nil || appErr != nil {
		t.Fatal("fixture configuration invalid; connection values withheld")
	}
	for _, cfg := range []*pgxpool.Config{adminCfg, appCfg} {
		if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") {
			t.Fatal("authorization tests require isolated loopback hit68_test_ database")
		}
		cfg.MaxConns, cfg.MinConns = 5, 0
	}
	if adminCfg.ConnConfig.User != "postgres" || appCfg.ConnConfig.User != "hp_hit81_app" || appCfg.ConnConfig.Database != adminCfg.ConnConfig.Database || appCfg.ConnConfig.Port != adminCfg.ConnConfig.Port {
		t.Fatal("actual application and administrator logins must share an isolated database")
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), adminCfg)
	if err != nil {
		t.Fatal("fixture administrator unavailable")
	}
	t.Cleanup(admin.Close)
	app, err := pgxpool.NewWithConfig(context.Background(), appCfg)
	if err != nil {
		t.Fatal("fixture app unavailable")
	}
	t.Cleanup(app.Close)
	reader, err := New(app)
	if err != nil {
		t.Fatal(err)
	}
	return authorityDatabase{admin: admin, app: app, reader: reader}
}
func authorityUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("synthetic random fixture unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (db authorityDatabase) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.admin.Exec(ctx, sql, args...); err != nil {
		t.Fatal("isolated authority fixture statement failed; values withheld")
	}
}

type authorityFixture struct {
	workspace, foreignWorkspace, principal, foreignPrincipal, session, foreignSession, runtime, boot, grant, foreignGrant, resource, raw, reference string
	now                                                                                                                                             time.Time
	caller                                                                                                                                          authorizeaction.Caller
}

func (db authorityDatabase) seed(t *testing.T) authorityFixture {
	t.Helper()
	f := authorityFixture{workspace: authorityUUID(t), foreignWorkspace: authorityUUID(t), principal: authorityUUID(t), foreignPrincipal: authorityUUID(t), session: authorityUUID(t), foreignSession: authorityUUID(t), runtime: authorityUUID(t), boot: authorityUUID(t), grant: authorityUUID(t), foreignGrant: authorityUUID(t), resource: authorityUUID(t), now: time.Now().UTC().Truncate(time.Microsecond)}
	f.raw = "HIT84-SYNTHETIC-SESSION-" + authorityUUID(t)
	digest := sha256.Sum256([]byte(f.raw))
	f.reference = hex.EncodeToString(digest[:])
	csrf := sha256.Sum256([]byte("HIT84-SYNTHETIC-CSRF-" + authorityUUID(t)))
	foreign := sha256.Sum256([]byte("HIT84-SYNTHETIC-FOREIGN-" + authorityUUID(t)))
	for _, principal := range []string{f.principal, f.foreignPrincipal} {
		db.exec(t, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://issuer.invalid',$1::text,$2)`, principal, f.now.Add(-time.Minute))
	}
	for _, s := range []struct {
		id, principal string
		hash          [32]byte
	}{{f.session, f.principal, digest}, {f.foreignSession, f.foreignPrincipal, foreign}} {
		db.exec(t, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch) VALUES($1,$2,$3,$4,$5,$6,$7,3)`, s.id, s.hash[:], s.principal, csrf[:], f.now.Add(-time.Minute), f.now.Add(time.Hour), f.now.Add(30*time.Minute))
	}
	for _, workspace := range []string{f.workspace, f.foreignWorkspace} {
		db.exec(t, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at) VALUES($1,$2,2,7,'ACTIVE',$3,$3)`, workspace, authorityUUID(t), f.now)
	}
	for _, m := range []struct{ workspace, principal string }{{f.workspace, f.principal}, {f.foreignWorkspace, f.foreignPrincipal}} {
		db.exec(t, `INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at) VALUES($1,$2,'OWNER','ACTIVE',5,$3)`, m.workspace, m.principal, f.now)
	}
	for _, b := range []struct{ workspace, grant string }{{f.workspace, f.grant}, {f.foreignWorkspace, f.foreignGrant}} {
		db.exec(t, `INSERT INTO hp_iam.external_bindings(workspace_id,grant_id,provider,provider_account_id,livemode,pack,status,security_epoch,created_at,verified_at) VALUES($1,$2,'STRIPE',$2::text,false,'PAID_ORDER_TASK_V1','VERIFIED',17,$3,$3)`, b.workspace, b.grant, f.now)
	}
	f.caller = authorizeaction.Caller{Service: "BFF", RuntimeID: f.runtime, BootID: f.boot, Generation: 11, CertificateFingerprint: "sha256:" + strings.Repeat("a", 64)}
	db.exec(t, `INSERT INTO hp_iam.runtime_registry(runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at) VALUES($1,'BFF',11,$2,$3,'OPEN',$4,$4)`, f.runtime, f.boot, f.caller.CertificateFingerprint, f.now)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		statements := []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM hp_iam.revocations WHERE workspace_id IN($1,$2) OR subject_id IN($3,$4)`, []any{f.workspace, f.foreignWorkspace, f.session, f.foreignSession}},
			{`DELETE FROM hp_iam.external_bindings WHERE workspace_id IN($1,$2)`, []any{f.workspace, f.foreignWorkspace}},
			{`DELETE FROM hp_iam.memberships WHERE workspace_id IN($1,$2)`, []any{f.workspace, f.foreignWorkspace}},
			{`DELETE FROM hp_iam.workspaces WHERE workspace_id IN($1,$2)`, []any{f.workspace, f.foreignWorkspace}},
			{`DELETE FROM hp_iam.sessions WHERE principal_id IN($1,$2)`, []any{f.principal, f.foreignPrincipal}},
			{`DELETE FROM hp_iam.principals WHERE principal_id IN($1,$2)`, []any{f.principal, f.foreignPrincipal}},
			{`DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1`, []any{f.runtime}},
		}
		for _, s := range statements {
			if _, err := db.admin.Exec(ctx, s.sql, s.args...); err != nil {
				t.Error("isolated authority fixture cleanup failed")
			}
		}
	})
	return f
}
func (f authorityFixture) request() authorizeaction.ReadRequest {
	return authorizeaction.ReadRequest{Reference: f.reference, WorkspaceID: f.workspace, RuntimeID: f.runtime, BindingGrantID: f.grant}
}
func (db authorityDatabase) read(t *testing.T, r authorizeaction.ReadRequest) authorizeaction.Snapshot {
	t.Helper()
	got, err := db.reader.Read(context.Background(), r)
	if err != nil {
		t.Fatalf("fresh authority read failed with bounded error: %v", err)
	}
	return got
}

func TestPostgresAuthorizationCurrentProjectionAndIsolation(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	got := db.read(t, f.request())
	if !got.Session.Found || !got.Session.Policy.Found || got.Session.ID != f.session || got.Session.PrincipalID != f.principal || got.Session.Epoch != 3 || got.Session.Policy.Revoked || !got.Principal.Found || got.Principal.ID != f.principal || got.Principal.Disabled {
		t.Fatal("current session/principal projection mismatch")
	}
	if !got.Workspace.Found || got.Workspace.ID != f.workspace || got.Workspace.Lifecycle != "ACTIVE" || got.Workspace.Epoch != 7 || !got.Membership.Found || got.Membership.PrincipalID != f.principal || got.Membership.WorkspaceID != f.workspace || got.Membership.Role != "OWNER" || got.Membership.Status != "ACTIVE" || got.Membership.Revision != 5 || got.Membership.Revoked {
		t.Fatal("workspace/membership projection mismatch")
	}
	if !got.Runtime.Found || got.Runtime.Caller != f.caller || got.Runtime.Status != "OPEN" || !got.Binding.Found || got.Binding.GrantID != f.grant || got.Binding.WorkspaceID != f.workspace || got.Binding.Epoch != 17 || got.Binding.Status != "VERIFIED" {
		t.Fatal("caller/runtime/binding projection mismatch")
	}
	uppercase := f.request()
	uppercase.WorkspaceID = strings.ToUpper(uppercase.WorkspaceID)
	uppercase.RuntimeID = strings.ToUpper(uppercase.RuntimeID)
	uppercase.BindingGrantID = strings.ToUpper(uppercase.BindingGrantID)
	if upper := db.read(t, uppercase); !upper.Membership.Found || upper.Workspace.ID != f.workspace || upper.Runtime.Caller.RuntimeID != f.runtime || upper.Binding.GrantID != f.grant {
		t.Fatal("valid UUID letter case changed the authority identity")
	}
	// Prove the existing SHA reference and optimized consumer are reused; raw
	// cookies and public UUIDs never become a substitute lookup selector.
	reference, err := sessionlookup.Reference(f.raw)
	if err != nil || reference != f.reference {
		t.Fatal("existing reference derivation mismatch")
	}
	lookup, err := sessionlookup.New(db.app)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authenticatesession.New(lookup, authorityClock{at: f.now})
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Execute(context.Background(), reference).IsValid() {
		t.Fatal("existing session consumer did not accept fixture")
	}
	foreign := f.request()
	foreign.WorkspaceID = f.foreignWorkspace
	foreign.BindingGrantID = f.foreignGrant
	cross := db.read(t, foreign)
	if !cross.Workspace.Found || cross.Membership.Found || !cross.Session.Found || cross.Session.PrincipalID != f.principal {
		t.Fatal("foreign workspace inherited the actor's membership")
	}
	ownAgain := db.read(t, f.request())
	if !ownAgain.Membership.Found || ownAgain.Membership.WorkspaceID != f.workspace {
		t.Fatal("pooled workspace GUC escaped request transaction")
	}
	foreignGrant := f.request()
	foreignGrant.BindingGrantID = f.foreignGrant
	if db.read(t, foreignGrant).Binding.Found {
		t.Fatal("foreign IAM grant crossed workspace boundary")
	}
	unknown := f.request()
	unknown.Reference = strings.Repeat("f", 64)
	if got := db.read(t, unknown); got.Session.Found || got.Principal.Found || got.Workspace.Found || got.Membership.Found || got.Runtime.Found {
		t.Fatal("unknown session exposed an authority projection")
	}
}

type authorityClock struct{ at time.Time }

func (c authorityClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: c.at, Trusted: true}, nil
}

func TestPostgresAuthorizationFreshCommittedChanges(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	_ = db.read(t, f.request())
	db.exec(t, `UPDATE hp_iam.memberships SET role='READER',revision=6 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal)
	got := db.read(t, f.request())
	if got.Membership.Role != "READER" || got.Membership.Revision != 6 {
		t.Fatal("committed role change hidden by stale authority")
	}
	db.exec(t, `UPDATE hp_iam.memberships SET status='REVOKED',revoked_at=$3,revision=7 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal, f.now)
	got = db.read(t, f.request())
	if got.Membership.Status != "REVOKED" || !got.Membership.Revoked || got.Membership.Revision != 7 {
		t.Fatal("committed membership revoke hidden")
	}
	db.exec(t, `UPDATE hp_iam.workspaces SET security_epoch=8 WHERE workspace_id=$1`, f.workspace)
	db.exec(t, `UPDATE hp_iam.sessions SET epoch=4,revoked_at=$2 WHERE session_id=$1`, f.session, f.now)
	db.exec(t, `UPDATE hp_iam.principals SET disabled_at=$2 WHERE principal_id=$1`, f.principal, f.now)
	db.exec(t, `UPDATE hp_iam.external_bindings SET status='REVOKED',security_epoch=18 WHERE workspace_id=$1 AND grant_id=$2`, f.workspace, f.grant)
	db.exec(t, `UPDATE hp_iam.runtime_registry SET status='FENCED',generation=12 WHERE runtime_id=$1`, f.runtime)
	got = db.read(t, f.request())
	if got.Session.Epoch != 4 || !got.Session.Policy.Revoked || !got.Principal.Disabled || got.Workspace.Epoch != 8 || got.Binding.Status != "REVOKED" || got.Binding.Epoch != 18 || got.Runtime.Status != "FENCED" || got.Runtime.Caller.Generation != 12 {
		t.Fatal("committed deny/epoch/runtime state was stale or merged")
	}
}

func (db authorityDatabase) revoke(t *testing.T, f authorityFixture, scope, workspace, subject, status string, epoch int64) {
	t.Helper()
	var ws, effective any
	if workspace != "" {
		ws = workspace
	}
	if status == "EFFECTIVE" {
		effective = f.now
	}
	db.exec(t, `INSERT INTO hp_iam.revocations(workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,effective_at,reason_code) VALUES($1,$2,$3,$4,$5,$6,$6::timestamptz+interval'5seconds',$7,$8,'SYNTHETIC')`, ws, authorityUUID(t), scope, subject, epoch, f.now, status, effective)
}
func TestPostgresAuthorizationScopeEpochsRemainSeparate(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	db.revoke(t, f, "SESSION", "", f.session, "PENDING", 4)
	db.revoke(t, f, "MEMBERSHIP", f.workspace, authorityUUID(t), "INCIDENT", 31)
	db.revoke(t, f, "CONNECTION", f.workspace, f.grant, "EFFECTIVE", 18)
	db.revoke(t, f, "WORKSPACE", f.workspace, f.workspace, "PENDING", 8)
	db.revoke(t, f, "SESSION", "", f.foreignSession, "PENDING", 99)
	db.revoke(t, f, "WORKSPACE", f.foreignWorkspace, f.foreignWorkspace, "PENDING", 100)
	got := db.read(t, f.request())
	if got.Session.Epoch != 3 || got.Workspace.Epoch != 7 || got.Binding.Epoch != 17 || got.Membership.Revision != 5 || len(got.Revocations) != 4 {
		t.Fatal("scope identities were merged, inferred or leaked from unrelated authority")
	}
	want := map[string]int64{"SESSION": 4, "MEMBERSHIP": 31, "CONNECTION": 18, "WORKSPACE": 8}
	for _, r := range got.Revocations {
		if want[r.Scope] != r.Epoch || r.WorkspaceID != f.workspace && !(r.Scope == "SESSION" && r.WorkspaceID == "") {
			t.Fatal("revocation scope/epoch lost")
		}
		delete(want, r.Scope)
	}
	if len(want) != 0 {
		t.Fatal("relevant typed scope missing")
	}
}

func TestPostgresAuthorizationConcurrentFreshSnapshots(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	const calls = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan authorizeaction.Snapshot, calls)
	errs := make(chan error, calls)
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, err := db.reader.Read(context.Background(), f.request())
			results <- s
			errs <- err
		}()
	}
	close(start)
	// Commit the role and workspace epoch together. Every returned snapshot must
	// be entirely before or entirely after this transaction, never a torn grant.
	tx, err := db.admin.Begin(context.Background())
	if err != nil {
		t.Fatal("fixture change transaction unavailable")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `UPDATE hp_iam.memberships SET role='READER',revision=6 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal); err != nil {
		t.Fatal("fixture membership change unavailable")
	}
	if _, err := tx.Exec(context.Background(), `UPDATE hp_iam.workspaces SET security_epoch=8 WHERE workspace_id=$1`, f.workspace); err != nil {
		t.Fatal("fixture epoch change unavailable")
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal("fixture change commit unavailable")
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent fresh read failed: %v", err)
		}
	}
	for s := range results {
		before := s.Membership.Role == "OWNER" && s.Membership.Revision == 5 && s.Workspace.Epoch == 7
		after := s.Membership.Role == "READER" && s.Membership.Revision == 6 && s.Workspace.Epoch == 8
		if !before && !after {
			t.Fatal("authority read mixed committed revisions across snapshots")
		}
	}
	latest := db.read(t, f.request())
	if latest.Membership.Role != "READER" || latest.Workspace.Epoch != 8 {
		t.Fatal("next request did not observe committed change")
	}
}

func TestPostgresAuthorizationCancellationFailureAndPoolAuthority(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	for _, env := range []string{"HITPIPE_IAM_TEST_ADMIN_DSN", "HITPIPE_IAM_TEST_WORKER_DSN", "HITPIPE_IAM_TEST_PURGE_DSN", "HITPIPE_IAM_MIGRATOR_DSN"} {
		t.Run(env, func(t *testing.T) {
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip("forbidden authority fixture not configured")
			}
			cfg, err := pgxpool.ParseConfig(dsn)
			if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Database != db.app.Config().ConnConfig.Database || cfg.ConnConfig.Port != db.app.Config().ConnConfig.Port {
				t.Fatal("forbidden authority must use same isolated database")
			}
			cfg.MaxConns = 5
			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal("forbidden authority fixture unavailable")
			}
			defer pool.Close()
			reader, _ := New(pool)
			if got, err := reader.Read(context.Background(), f.request()); got.Session.Found || !errors.Is(err, ErrConfiguration) {
				t.Fatal("forbidden authority admitted")
			}
		})
	}
	// Exhaust only this adapter's isolated pool; deadlines include acquisition.
	held := make([]*pgxpool.Conn, 0, 5)
	for range 5 {
		conn, err := db.app.Acquire(context.Background())
		if err != nil {
			t.Fatal("bounded pool fixture unavailable")
		}
		held = append(held, conn)
	}
	for _, timeout := range []time.Duration{20 * time.Millisecond, 5 * time.Second} {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		started := time.Now()
		got, err := db.reader.Read(ctx, f.request())
		cancel()
		if got.Session.Found || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 2500*time.Millisecond {
			t.Fatal("pool acquisition was not bounded/cancellable")
		}
	}
	for _, conn := range held {
		conn.Release()
	}
	if !db.read(t, f.request()).Session.Found {
		t.Fatal("canceled read corrupted borrowed pool")
	}
	cfg := db.app.Config()
	closed, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("closed pool fixture unavailable")
	}
	reader, _ := New(closed)
	closed.Close()
	if got, err := reader.Read(context.Background(), f.request()); got.Session.Found || !errors.Is(err, ErrUnavailable) {
		t.Fatal("database failure returned authority")
	}
}

// Controlled ports model a verified caller, a resource owner's exact proof and
// one shared local actual-start section. They are test authorities, not shipped
// mTLS, owner-resource or distributed barrier adapters.
type authorityCaller struct{ caller authorizeaction.Caller }

func (c authorityCaller) Current(context.Context) (authorizeaction.Caller, error) {
	return c.caller, nil
}

type authorityOwner struct {
	operation, grant     string
	before               func()
	proofAction          string
	principal, workspace string
	resource             *string
}

func (o authorityOwner) Prove(_ context.Context, actor authorizeaction.Actor, request authorizeaction.Request) (authorizeaction.Proof, error) {
	sameResource := request.ResourceID == nil && o.resource == nil || request.ResourceID != nil && o.resource != nil && *request.ResourceID == *o.resource
	if actor.PrincipalID != o.principal || actor.WorkspaceID != o.workspace || request.WorkspaceID != o.workspace || !sameResource {
		return authorizeaction.Proof{}, errors.New("synthetic owner scope denied")
	}
	if o.before != nil {
		o.before()
	}
	proof := authorizeaction.Proof{Verified: true, PrincipalID: actor.PrincipalID, WorkspaceID: actor.WorkspaceID, ResourceID: request.ResourceID, Action: request.Action, Operation: o.operation, BindingGrantID: o.grant}
	if o.proofAction != "" {
		proof.Action = o.proofAction
	}
	if request.Action == "CASE_RECOVER" {
		proof.RecoveryAction = "RECHECK"
	}
	return proof, nil
}

type authorityGate struct {
	mu       sync.Mutex
	expected authorizeaction.ScopeEpochs
	closed   bool
	inside   chan struct{}
	release  chan struct{}
}
type authorityAdmission struct {
	gate    *authorityGate
	binding authorizeaction.GateBinding
}

func (a authorityAdmission) Binding() authorizeaction.GateBinding { return a.binding }
func (a authorityAdmission) Confirm(ctx context.Context, scope authorizeaction.ScopeEpochs) (int64, error) {
	if ctx.Err() != nil || a.gate.closed {
		return 0, errors.New("synthetic gate closed")
	}
	want := a.gate.expected
	if scope.PrincipalID != want.PrincipalID || scope.SessionID != want.SessionID || scope.WorkspaceID != want.WorkspaceID || scope.BindingGrantID != want.BindingGrantID || scope.SessionEpoch != want.SessionEpoch || scope.WorkspaceEpoch != want.WorkspaceEpoch || scope.BindingEpoch != want.BindingEpoch || scope.MembershipRevision != want.MembershipRevision {
		return 0, errors.New("synthetic current scope mismatch")
	}
	if len(scope.Revocations) != len(want.Revocations) {
		return 0, errors.New("synthetic deny overlay unknown")
	}
	for n := range scope.Revocations {
		if scope.Revocations[n] != want.Revocations[n] {
			return 0, errors.New("synthetic deny overlay differs")
		}
	}
	// Explicit workspace-scoped wire epoch; never max(3,7,17,5).
	return want.WorkspaceEpoch, nil
}
func (g *authorityGate) Within(ctx context.Context, binding authorizeaction.GateBinding, work func(context.Context, authorizeaction.Admission) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || ctx.Err() != nil {
		return errors.New("synthetic actual-start closed")
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if g.inside != nil {
		close(g.inside)
		g.inside = nil
		if g.release != nil {
			select {
			case <-g.release:
			case <-bounded.Done():
				return bounded.Err()
			}
		}
	}
	if err := work(bounded, authorityAdmission{gate: g, binding: binding}); err != nil {
		return err
	}
	if g.closed || bounded.Err() != nil {
		return errors.New("synthetic final gate denied")
	}
	return nil
}
func (f authorityFixture) gate(grant bool) *authorityGate {
	facts := authorizeaction.ScopeEpochs{PrincipalID: f.principal, SessionID: f.session, WorkspaceID: f.workspace, SessionEpoch: 3, WorkspaceEpoch: 7, MembershipRevision: 5}
	if grant {
		facts.BindingGrantID = f.grant
		facts.BindingEpoch = 17
	}
	return &authorityGate{expected: facts}
}
func (db authorityDatabase) service(t *testing.T, f authorityFixture, owner authorityOwner, gate *authorityGate, caller authorizeaction.Caller) authorizeaction.Service {
	t.Helper()
	lookup, err := sessionlookup.New(db.app)
	if err != nil {
		t.Fatal(err)
	}
	owner.principal, owner.workspace = f.principal, f.workspace
	if owner.operation == "saveDraft" || owner.operation == "getDraft" || owner.operation == "recoverCase" || owner.operation == "activateOperation" {
		resource := f.resource
		owner.resource = &resource
	}
	service, err := authorizeaction.New(lookup, authorizeaction.ReferenceFunc(sessionlookup.Reference), authorityClock{at: f.now}, authorityCaller{caller}, db.reader, owner, gate)
	if err != nil {
		t.Fatal("controlled authorization composition unavailable")
	}
	return service
}
func (f authorityFixture) action(action string) authorizeaction.Request {
	request := authorizeaction.Request{WorkspaceID: f.workspace, RuntimeID: f.runtime, Action: action}
	if action == "CONFIG_READ" || action == "CONFIG_WRITE" || action == "CASE_RECOVER" || action == "ACTIVATE" {
		resource := f.resource
		request.ResourceID = &resource
	}
	return request
}

func TestPostgresAuthorizationRoleActionWorkspaceMatrix(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	cases := []struct {
		role, action, operation string
		allowed                 bool
	}{
		{"OWNER", "CONFIG_WRITE", "saveDraft", true},
		{"OWNER", "CONNECTOR_WRITE", "connectService", true},
		{"OWNER", "CASE_READ", "listCases", true},
		{"OPERATOR", "CASE_READ", "listCases", true},
		{"OPERATOR", "CASE_RECOVER", "recoverCase", true},
		{"OPERATOR", "CONFIG_WRITE", "saveDraft", false},
		{"OPERATOR", "CONNECTOR_WRITE", "connectService", false},
		{"READER", "CASE_READ", "listCases", true},
		{"READER", "CONFIG_READ", "getDraft", true},
		{"READER", "CASE_RECOVER", "recoverCase", false},
		{"READER", "CONFIG_WRITE", "saveDraft", false},
	}
	for _, c := range cases {
		t.Run(c.role+"/"+c.action, func(t *testing.T) {
			db.exec(t, `UPDATE hp_iam.memberships SET role=$3 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal, c.role)
			owner, gate := authorityOwner{operation: c.operation}, f.gate(false)
			if strings.HasPrefix(c.action, "CONNECTOR_") {
				owner.grant, gate = f.grant, f.gate(true)
			}
			service := db.service(t, f, owner, gate, f.caller)
			got, err := service.Execute(context.Background(), f.raw, f.action(c.action))
			if c.allowed {
				if err != nil || !got.Allowed || got.Epoch != 7 || got.PrincipalID != f.principal || got.SessionID != f.session || got.WorkspaceID != f.workspace || got.Role != c.role || !got.CheckedAt.Equal(f.now) {
					t.Fatalf("accepted role/action did not produce exact current OAS projection: %v", err)
				}
			} else if !errors.Is(err, authorizeaction.ErrDenied) || got.Allowed || got.PrincipalID != "" || got.WorkspaceID != "" {
				t.Fatal("forbidden role/action exposed authorization")
			}
		})
	}
	service := db.service(t, f, authorityOwner{operation: "listCases"}, f.gate(false), f.caller)
	request := f.action("CASE_READ")
	request.WorkspaceID = f.foreignWorkspace
	if got, err := service.Execute(context.Background(), f.raw, request); got.Allowed || !errors.Is(err, authorizeaction.ErrDenied) || got.WorkspaceID != "" {
		t.Fatal("foreign workspace acquired actor's authority")
	}
}

func TestPostgresAuthorizationFreshDenialsAndCurrentEpochGate(t *testing.T) {
	tests := []struct {
		name   string
		change func(authorityDatabase, *testing.T, authorityFixture)
	}{
		{"unknown-session", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `DELETE FROM hp_iam.sessions WHERE session_id=$1`, f.session)
		}},
		{"disabled", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.principals SET disabled_at=$2 WHERE principal_id=$1`, f.principal, f.now)
		}},
		{"revoked-session", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.sessions SET revoked_at=$2 WHERE session_id=$1`, f.session, f.now)
		}},
		{"expired", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.sessions SET expires_at=$2,idle_expires_at=$2 WHERE session_id=$1`, f.session, f.now)
		}},
		{"revoked-membership", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.memberships SET status='REVOKED',revoked_at=$3,revision=6 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal, f.now)
		}},
		{"new-session-epoch", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.sessions SET epoch=4 WHERE session_id=$1`, f.session)
		}},
		{"new-workspace-epoch", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.workspaces SET security_epoch=8 WHERE workspace_id=$1`, f.workspace)
		}},
		{"runtime-closed", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.runtime_registry SET status='CLOSED' WHERE runtime_id=$1`, f.runtime)
		}},
		{"runtime-syncing", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.runtime_registry SET status='SYNCING' WHERE runtime_id=$1`, f.runtime)
		}},
		{"runtime-fenced", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.runtime_registry SET status='FENCED' WHERE runtime_id=$1`, f.runtime)
		}},
		{"runtime-new-boot", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.runtime_registry SET boot_id=$2 WHERE runtime_id=$1`, f.runtime, authorityUUID(t))
		}},
		{"runtime-new-generation", func(db authorityDatabase, t *testing.T, f authorityFixture) {
			db.exec(t, `UPDATE hp_iam.runtime_registry SET generation=12 WHERE runtime_id=$1`, f.runtime)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := authorityDB(t)
			f := db.seed(t)
			service := db.service(t, f, authorityOwner{operation: "listCases"}, f.gate(false), f.caller)
			if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err != nil || !got.Allowed {
				t.Fatal("baseline authorization unavailable")
			}
			test.change(db, t, f)
			if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed || got.PrincipalID != "" {
				t.Fatal("committed deny/unknown current gate retained a stale allow")
			}
		})
	}
}

func TestPostgresAuthorizationTypedRevocationAdmission(t *testing.T) {
	for _, scope := range []string{"SESSION", "MEMBERSHIP", "CONNECTION", "WORKSPACE"} {
		for _, status := range []string{"PENDING", "INCIDENT"} {
			t.Run(scope+"/"+status, func(t *testing.T) {
				db := authorityDB(t)
				f := db.seed(t)
				workspace, subject, epoch := f.workspace, authorityUUID(t), int64(31)
				switch scope {
				case "SESSION":
					workspace, subject, epoch = "", f.session, 4
				case "CONNECTION":
					subject, epoch = f.grant, 18
				case "WORKSPACE":
					subject, epoch = f.workspace, 8
				}
				db.revoke(t, f, scope, workspace, subject, status, epoch)
				owner, gate := authorityOwner{operation: "listCases"}, f.gate(false)
				if scope == "CONNECTION" {
					owner.grant, gate = f.grant, f.gate(true)
				}
				service := db.service(t, f, owner, gate, f.caller)
				if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
					t.Fatal("pending/incident typed revoke admitted")
				}
			})
		}
	}
	t.Run("effective-needs-exact-explicit-overlay", func(t *testing.T) {
		db := authorityDB(t)
		f := db.seed(t)
		db.revoke(t, f, "WORKSPACE", f.workspace, f.workspace, "EFFECTIVE", 7)
		gate := f.gate(false)
		service := db.service(t, f, authorityOwner{operation: "listCases"}, gate, f.caller)
		if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
			t.Fatal("epoch equality inferred an effective historical regrant")
		}
		// A deliberately configured trusted fixture confirms the exact effective
		// record and current grant overlay. Its existence/epoch alone was insufficient.
		gate.expected.Revocations = []authorizeaction.Revocation{{Scope: "WORKSPACE", SubjectID: f.workspace, WorkspaceID: f.workspace, Epoch: 7, Status: "EFFECTIVE"}}
		if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err != nil || !got.Allowed || got.Epoch != 7 {
			t.Fatalf("explicit controlled current overlay was not considered: %v", err)
		}
	})
}

func TestPostgresAuthorizationOwnerProofBindingAndCallerIdentity(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	gate := f.gate(true)
	service := db.service(t, f, authorityOwner{operation: "listCases", grant: f.grant}, gate, f.caller)
	if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err != nil || !got.Allowed || got.Epoch != 7 {
		t.Fatal("owner-resolved IAM grant did not participate with separate epoch")
	}
	foreign := db.service(t, f, authorityOwner{operation: "listCases", grant: f.foreignGrant}, f.gate(true), f.caller)
	if got, err := foreign.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
		t.Fatal("foreign owner-resolved IAM grant admitted")
	}
	mismatched := f.caller
	mismatched.CertificateFingerprint = "sha256:" + strings.Repeat("b", 64)
	wrongCaller := db.service(t, f, authorityOwner{operation: "listCases"}, f.gate(false), mismatched)
	if got, err := wrongCaller.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
		t.Fatal("unregistered caller certificate trusted")
	}
	gate.closed = true
	if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
		t.Fatal("closed actual-start gate admitted")
	}
}

func TestPostgresAuthorizationRechecksAfterOwnerWait(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	owner := authorityOwner{operation: "saveDraft", before: func() {
		db.exec(t, `UPDATE hp_iam.memberships SET role='READER',revision=6 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal)
	}}
	service := db.service(t, f, owner, f.gate(false), f.caller)
	if got, err := service.Execute(context.Background(), f.raw, f.action("CONFIG_WRITE")); err == nil || got.Allowed {
		t.Fatal("owner-port wait turned earlier role into retained grant")
	}
}

func TestPostgresAuthorizationSharedLocalGateSerializesRevoke(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	inside, release := make(chan struct{}), make(chan struct{})
	gate := f.gate(false)
	gate.inside, gate.release = inside, release
	service := db.service(t, f, authorityOwner{operation: "listCases"}, gate, f.caller)
	type response struct {
		authorization authorizeaction.Authorization
		err           error
	}
	done := make(chan response, 1)
	go func() {
		got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ"))
		done <- response{got, err}
	}()
	select {
	case <-inside:
	case <-time.After(time.Second):
		t.Fatal("controlled gate section never entered")
	}
	started, committed := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		gate.mu.Lock()
		defer gate.mu.Unlock()
		_, err := db.admin.Exec(context.Background(), `UPDATE hp_iam.memberships SET status='REVOKED',revoked_at=$3,revision=6 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal, f.now)
		committed <- err
	}()
	<-started
	select {
	case <-committed:
		t.Fatal("local revoke crossed admitted start section")
	default:
	}
	close(release)
	select {
	case result := <-done:
		if result.err != nil || !result.authorization.Allowed {
			t.Fatal("already admitted local section did not complete")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("controlled section did not finish")
	}
	select {
	case err := <-committed:
		if err != nil {
			t.Fatal("controlled revoke commit failed")
		}
	case <-time.After(time.Second):
		t.Fatal("controlled revoke did not complete")
	}
	if got, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
		t.Fatal("start after committed local revoke was allowed")
	}
}

func TestPostgresAuthorizationUnrelatedConnectionIsNotAuthority(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	otherGrant := authorityUUID(t)
	db.exec(t, `INSERT INTO hp_iam.external_bindings(workspace_id,grant_id,provider,provider_account_id,livemode,pack,status,security_epoch,created_at,verified_at) VALUES($1,$2,'STRIPE',$2::text,false,'PAID_ORDER_TASK_V1','VERIFIED',91,$3,$3)`, f.workspace, otherGrant, f.now)
	db.revoke(t, f, "CONNECTION", f.workspace, otherGrant, "PENDING", 92)
	// The owner attests grant A. A revoke for grant B in the same workspace must
	// neither block A nor be included as A's grant/epoch in Confirm.
	exact := db.service(t, f, authorityOwner{operation: "listCases", grant: f.grant}, f.gate(true), f.caller)
	if got, err := exact.Execute(context.Background(), f.raw, f.action("CASE_READ")); err != nil || !got.Allowed || got.Epoch != 7 {
		t.Fatalf("unrelated connector revoke changed exact owner-resolved authority: %v", err)
	}
	profile := db.service(t, f, authorityOwner{operation: "getProfile"}, f.gate(false), f.caller)
	if got, err := profile.Execute(context.Background(), f.raw, f.action("PROFILE_READ")); err != nil || !got.Allowed {
		t.Fatalf("unrelated connector revoke blocked own profile: %v", err)
	}
	// A current matching SESSION deny remains global to this actor even though
	// there is also a distinct, irrelevant connector epoch.
	db.revoke(t, f, "SESSION", "", f.session, "EFFECTIVE", 3)
	if got, err := exact.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || got.Allowed {
		t.Fatal("effective session denial was inferred to be an old safe epoch")
	}
}

func TestPostgresAuthorizationSessionDenyUnknownWorkspaceRelation(t *testing.T) {
	db := authorityDB(t)
	f := db.seed(t)
	// Exact SESSION subject must survive the SQL projection even if its workspace
	// relation is inconsistent/unknown. This does not invent global semantics: the
	// application holds the scenario instead of silently dropping the deny.
	db.revoke(t, f, "SESSION", f.foreignWorkspace, f.session, "PENDING", 4)
	db.revoke(t, f, "SESSION", f.foreignWorkspace, f.foreignSession, "PENDING", 99)
	got := db.read(t, f.request())
	if len(got.Revocations) != 1 || got.Revocations[0].SubjectID != f.session || got.Revocations[0].WorkspaceID != f.foreignWorkspace {
		t.Fatal("exact session deny was hidden or unrelated session authority leaked")
	}
	service := db.service(t, f, authorityOwner{operation: "listCases"}, f.gate(false), f.caller)
	if result, err := service.Execute(context.Background(), f.raw, f.action("CASE_READ")); err == nil || result.Allowed || result.WorkspaceID != "" {
		t.Fatal("unknown session/workspace deny relation became permission")
	}
}
