//go:build integration

package revocation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixtures struct {
	admin, application *pgxpool.Pool
	appConfig          *pgxpool.Config
}

func database(t *testing.T) fixtures {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_authorization.py for disposable PostgreSQL tests")
	}
	admin, e1 := pgxpool.ParseConfig(adminDSN)
	application, e2 := pgxpool.ParseConfig(appDSN)
	if e1 != nil || e2 != nil || admin.ConnConfig.Host != "127.0.0.1" || application.ConnConfig.Host != "127.0.0.1" ||
		admin.ConnConfig.User != "postgres" || application.ConnConfig.User != "hp_hit81_app" ||
		!strings.HasPrefix(admin.ConnConfig.Database, "hit68_test_") ||
		application.ConnConfig.Database != admin.ConnConfig.Database || application.ConnConfig.Port != admin.ConnConfig.Port {
		t.Fatal("requires disposable loopback PostgreSQL and existing IAM roles")
	}
	return fixtures{admin: testPool(t, admin), application: testPool(t, application), appConfig: application}
}

func testPool(t *testing.T, config *pgxpool.Config) *pgxpool.Pool {
	t.Helper()
	config.MaxConns, config.MinConns = 5, 0
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal("isolated pool unavailable")
	}
	t.Cleanup(pool.Close)
	return pool
}

func mustStore(t *testing.T, pool *pgxpool.Pool) *Store {
	t.Helper()
	s, err := New(pool)
	if err != nil {
		t.Fatalf("cannot construct valid store: %v", err)
	}
	return s
}

func testID(t *testing.T) string {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatal("fixture UUID unavailable")
	}
	return id
}

func baseTime() time.Time { return app.NormalizeTime(time.Now()) }

func (d fixtures) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := d.admin.Exec(ctx, sql, args...); err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			t.Fatalf("isolated fixture command failed (SQLSTATE %s)", pgError.Code)
		}
		t.Fatal("isolated fixture command failed")
	}
}

func (d fixtures) runtime(t *testing.T, gate string) startgate.Identity {
	t.Helper()
	ident := startgate.Identity{RuntimeID: testID(t), BootID: testID(t), Generation: 1}
	t.Cleanup(func() { d.exec(t, `DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1`, ident.RuntimeID) })
	d.exec(t, `INSERT INTO hp_iam.runtime_registry(runtime_id,service,generation,boot_id,certificate_fingerprint,
		status,registered_at,last_observed_at)
		VALUES($1,'BFF',$2,$3,$4,$5,statement_timestamp(),statement_timestamp()-interval '1 hour')`,
		ident.RuntimeID, ident.Generation, ident.BootID, "sha256:"+strings.Repeat("a", 64), gate)
	return ident
}

func (d fixtures) session(t *testing.T, now time.Time) app.Intent {
	t.Helper()
	principal, session, revoke := testID(t), testID(t), testID(t)
	hash := sha256.Sum256([]byte(session))
	t.Cleanup(func() {
		d.exec(t, `DELETE FROM hp_iam.principals WHERE principal_id=$1`, principal)
	})
	d.exec(t, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
		VALUES($1,'https://issuer.invalid',$2,$3)`, principal, principal, now.Add(-time.Second))
	t.Cleanup(func() { d.exec(t, `DELETE FROM hp_iam.sessions WHERE session_id=$1`, session) })
	d.exec(t, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,
		expires_at,idle_expires_at,epoch) VALUES($1,$2,$3,$2,$4,$5,$5,7)`,
		session, hash[:], principal, now.Add(-time.Second), now.Add(time.Hour))
	d.removeIntent(t, revoke)
	return app.Intent{ID: revoke, Subject: startgate.Subject{Scope: startgate.Session, SubjectID: session}, ReasonCode: "TEST_SESSION"}
}

func (d fixtures) workspace(t *testing.T) app.Intent {
	t.Helper()
	workspace, revoke := testID(t), testID(t)
	t.Cleanup(func() { d.exec(t, `DELETE FROM hp_iam.workspaces WHERE workspace_id=$1`, workspace) })
	d.exec(t, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at)
		VALUES($1,$2,1,12,'ACTIVE',statement_timestamp())`, workspace, testID(t))
	d.removeIntent(t, revoke)
	return app.Intent{ID: revoke, Subject: startgate.Subject{Scope: startgate.Workspace, SubjectID: workspace, WorkspaceID: &workspace},
		ReasonCode: "TEST_WORKSPACE"}
}

func (d fixtures) removeIntent(t *testing.T, revoke string) {
	t.Helper()
	t.Cleanup(func() {
		d.exec(t, `DELETE FROM hp_iam.revocation_participants WHERE revocation_id=$1`, revoke)
		d.exec(t, `DELETE FROM hp_iam.revocations WHERE revocation_id=$1`, revoke)
	})
}

func sessionState(t *testing.T, d fixtures, intent app.Intent) (epoch uint64, revoked *time.Time) {
	t.Helper()
	if err := d.admin.QueryRow(t.Context(), `SELECT epoch,revoked_at FROM hp_iam.sessions WHERE session_id=$1`,
		intent.Subject.SubjectID).Scan(&epoch, &revoked); err != nil {
		t.Fatal("cannot read committed session state from a different connection")
	}
	return
}

func ackFor(r app.Record, ident startgate.Identity, at time.Time) startgate.BarrierAck {
	return startgate.BarrierAck{RevocationID: r.ID, RuntimeID: ident.RuntimeID, BootID: ident.BootID,
		Generation: ident.Generation, Subject: r.Subject, AppliedEpoch: r.Epoch, OldStartSectionsDrained: true, AcknowledgedAt: at}
}

func TestPostgresRevocationAtomicallyFreezesRegistryAndUpdatesSubject(t *testing.T) {
	d := database(t)
	ctx, now := t.Context(), baseTime()
	a, b := d.runtime(t, "OPEN"), d.runtime(t, "SYNCING")
	session, workspace := d.session(t, now), d.workspace(t)
	s := mustStore(t, d.application)
	rec, err := s.Begin(ctx, session, now)
	if err != nil || rec.Status != app.Pending || rec.Epoch != 8 || rec.Subject.WorkspaceID != nil ||
		!rec.DeadlineAt.Equal(now.Add(app.Deadline)) {
		t.Fatalf("expected a durable global session denial, PENDING intent and five-second deadline: %v", err)
	}
	if epoch, revoked := sessionState(t, d, session); epoch != 8 || revoked == nil || !revoked.Equal(now) {
		t.Fatal("PENDING returned without a committed session denial")
	}
	for _, required := range []startgate.Identity{a, b} {
		if !slices.ContainsFunc(rec.Participants, func(p app.Participant) bool { return p.Identity == required }) {
			t.Fatal("registered runtime lost from the mandatory roster")
		}
	}
	var rows, nullWorkspace int
	if err := d.admin.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE workspace_id IS NULL)
		FROM hp_iam.revocation_participants WHERE revocation_id=$1`, rec.ID).Scan(&rows, &nullWorkspace); err != nil ||
		rows != 2 || nullWorkspace != rows {
		t.Fatal("session roster not durably committed with NULL workspace")
	}
	// Add a runtime after the acceptance; same-ID recovery cannot widen the
	// original roster or increase the already committed epoch a second time.
	c := d.runtime(t, "CLOSED")
	d.removeIntent(t, workspace.ID) // Remove its FK before the later-registered runtime cleanup.
	retry, err := s.Begin(ctx, session, now.Add(time.Second))
	if err != nil || retry.Epoch != rec.Epoch || len(retry.Participants) != 2 || !retry.AcceptedAt.Equal(now) ||
		!retry.DeadlineAt.Equal(rec.DeadlineAt) {
		t.Fatalf("exact retry changed the original command: %v", err)
	}
	wrongReason := session
	wrongReason.ReasonCode = "OTHER_REASON"
	if _, err := s.Begin(ctx, wrongReason, now); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("same ID with different intent accepted: %v", err)
	}
	wrongID := session
	wrongID.ID = testID(t)
	if _, err := s.Begin(ctx, wrongID, now); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("second independent revocation of same subject accepted: %v", err)
	}
	w, err := s.Begin(ctx, workspace, now)
	if err != nil || w.Status != app.Pending || w.Epoch != 13 || len(w.Participants) != 3 ||
		!slices.ContainsFunc(w.Participants, func(p app.Participant) bool { return p.Identity == c }) {
		t.Fatalf("workspace decision lacks exact registered roster/epoch: %v", err)
	}
	var rawEpoch uint64
	var lifecycle string
	if err := d.admin.QueryRow(ctx, `SELECT security_epoch,lifecycle FROM hp_iam.workspaces WHERE workspace_id=$1`,
		workspace.Subject.SubjectID).Scan(&rawEpoch, &lifecycle); err != nil || rawEpoch != 13 || lifecycle != "ACTIVE" {
		t.Fatal("workspace epoch not committed or unrelated lifecycle changed")
	}
	if _, err := s.Begin(ctx, app.Intent{ID: session.ID, Subject: workspace.Subject, ReasonCode: session.ReasonCode}, now); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("operation identity accepted for a different scope: %v", err)
	}
}

func TestPostgresRevocationExactAckRecoveryAndIncident(t *testing.T) {
	d := database(t)
	ctx, now := t.Context(), baseTime()
	a, b := d.runtime(t, "OPEN"), d.runtime(t, "OPEN")
	intent := d.session(t, now)
	s := mustStore(t, d.application)
	rec, err := s.Begin(ctx, intent, now)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	first, second := ackFor(rec, a, now.Add(time.Second)), ackFor(rec, b, now.Add(2*time.Second))
	if err := s.Acknowledge(ctx, rec.ID, first); err != nil {
		t.Fatalf("valid ACK: %v", err)
	}
	if err := s.Acknowledge(ctx, rec.ID, first); err != nil {
		t.Fatalf("exact replay of ACK: %v", err)
	}
	for name, mutate := range map[string]func(*startgate.BarrierAck){
		"epoch":      func(a *startgate.BarrierAck) { a.AppliedEpoch++ },
		"scope":      func(a *startgate.BarrierAck) { a.Subject.Scope = startgate.Workspace },
		"boot":       func(a *startgate.BarrierAck) { a.BootID = testID(t) },
		"generation": func(a *startgate.BarrierAck) { a.Generation++ },
		"revision":   func(a *startgate.BarrierAck) { a.RevocationID = testID(t) },
		"drained":    func(a *startgate.BarrierAck) { a.OldStartSectionsDrained = false },
		"late":       func(a *startgate.BarrierAck) { a.AcknowledgedAt = rec.DeadlineAt },
	} {
		t.Run(name, func(t *testing.T) {
			fake := second
			mutate(&fake)
			if err := s.Acknowledge(ctx, rec.ID, fake); !errors.Is(err, app.ErrInvalid) {
				t.Fatalf("invalid barrier ACK not rejected: %v", err)
			}
		})
	}
	pending, err := s.Finalize(ctx, rec.ID, now.Add(3*time.Second))
	if err != nil || pending.Status != app.Pending || pending.EffectiveAt != nil ||
		!slices.Equal(pending.MissingRuntimeIDs(), []string{b.RuntimeID}) {
		t.Fatalf("a partial set of ACKs declared the revocation effective: %v", err)
	}

	// Simulate the registry reflecting a changed boot. A lease or FENCED flag
	// alone is never proof; the immutable old tuple still belongs to the journal.
	newBoot := testID(t)
	d.exec(t, `UPDATE hp_iam.runtime_registry SET boot_id=$2,generation=2,status='SYNCING' WHERE runtime_id=$1`,
		b.RuntimeID, newBoot)
	other := second
	other.BootID, other.Generation = newBoot, 2
	if err := s.Acknowledge(ctx, rec.ID, other); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("new boot ACK inherited stale participant: %v", err)
	}

	// New Store represents a restarted coordinator's DB adapter. An old
	// acknowledgement already recorded before death remains durable.
	s = mustStore(t, d.application)
	ids, err := s.Open(ctx, "", 64)
	if err != nil || !slices.Contains(ids, rec.ID) {
		t.Fatalf("restart did not discover pending operation: %v", err)
	}
	incident, err := s.Finalize(ctx, rec.ID, rec.DeadlineAt)
	if err != nil || incident.Status != app.Incident || incident.IncidentID == nil ||
		incident.DeadlineBreachedAt == nil || incident.EffectiveAt != nil ||
		!slices.Equal(incident.MissingRuntimeIDs(), []string{b.RuntimeID}) {
		t.Fatalf("partial deadline did not preserve truthful incident: %v", err)
	}
	for _, p := range incident.Participants {
		if p.Identity.RuntimeID == b.RuntimeID && (p.Identity != b || p.CertificateFingerprint != "") {
			t.Fatal("recovery retargeted the frozen participant to the new boot")
		}
	}
	// Simulate recovery of an authenticated on-time original ACK whose
	// response was lost, never a new late ACK from the old or replacement boot.
	if err := s.Acknowledge(ctx, rec.ID, second); err != nil {
		t.Fatalf("original ACK recovery: %v", err)
	}
	effective, err := s.Finalize(ctx, rec.ID, rec.DeadlineAt.Add(time.Second))
	if err != nil || effective.Status != app.Effective || effective.EffectiveAt == nil ||
		effective.DeadlineBreachedAt == nil || !effective.DeadlineBreachedAt.Equal(*incident.DeadlineBreachedAt) ||
		effective.IncidentID == nil || *effective.IncidentID != *incident.IncidentID ||
		len(effective.MissingRuntimeIDs()) != 0 {
		t.Fatalf("recovered complete proof erased deadline history or didn't complete: %v", err)
	}
	ids, err = s.Open(ctx, "", 64)
	if err != nil || slices.Contains(ids, rec.ID) {
		t.Fatalf("completed revocation remains recoverable as pending: %v", err)
	}
}

func TestPostgresRevocationRejectedCommitAndCancellationLeaveNoPartialState(t *testing.T) {
	d := database(t)
	ctx, now := t.Context(), baseTime()
	_ = d.runtime(t, "OPEN")
	intent := d.session(t, now)
	s := mustStore(t, d.application)

	if _, err := mustStore(t, d.admin).Begin(ctx, intent, now); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("administrative login gained application authority: %v", err)
	}
	holder, err := d.admin.Begin(ctx)
	if err != nil {
		t.Fatal("fixture lock transaction unavailable")
	}
	defer rollback(holder)
	if _, err := holder.Exec(ctx, `LOCK TABLE hp_iam.runtime_registry IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		t.Fatal("cannot lock test registry")
	}
	blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = s.Begin(blocked, intent, now)
	cancel()
	if !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("canceled acceptance was not rejected: %v", err)
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal("fixture lock was not released")
	}

	d.exec(t, `CREATE FUNCTION public.hit116_commit_rejected() RETURNS trigger LANGUAGE plpgsql AS
		'BEGIN RAISE EXCEPTION USING MESSAGE = ''synthetic value must not reach diagnostics''; END'`)
	t.Cleanup(func() { d.exec(t, `DROP FUNCTION public.hit116_commit_rejected()`) })
	d.exec(t, `CREATE CONSTRAINT TRIGGER hit116_commit_rejected
		AFTER INSERT ON hp_iam.revocation_participants DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION public.hit116_commit_rejected()`)
	t.Cleanup(func() { d.exec(t, `DROP TRIGGER hit116_commit_rejected ON hp_iam.revocation_participants`) })
	if result, err := s.Begin(ctx, intent, now); !errors.Is(err, app.ErrUnavailable) || result.ID != "" {
		t.Fatalf("deferred rejection exposed a command or a SQL diagnostic: %v", err)
	}
	if epoch, revoked := sessionState(t, d, intent); epoch != 7 || revoked != nil {
		t.Fatal("cancelled or rejected commit leaked partial deny/epoch")
	}
	if _, err := s.Get(ctx, intent.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("rejected commit left a durable journal row: %v", err)
	}
}

// commitDropper is used only on the disposable, plaintext PostgreSQL loopback
// client. It consumes PostgreSQL's real successful COMMIT response and hides it
// from the app driver; a different physical connection can still see the data.
type commitDropper struct {
	net.Conn
	commit atomic.Bool
	proved *atomic.Bool
}

func (c *commitDropper) Write(data []byte) (int, error) {
	if len(data) == len("commit")+6 && data[0] == 'Q' && bytes.Equal(data[5:], []byte("commit\x00")) {
		c.commit.Store(true)
	}
	return c.Conn.Write(data)
}

func (c *commitDropper) Read(data []byte) (int, error) {
	if !c.commit.Swap(false) {
		return c.Conn.Read(data)
	}
	var header [5]byte
	if _, err := io.ReadFull(c.Conn, header[:]); err != nil {
		return 0, err
	}
	size := binary.BigEndian.Uint32(header[1:])
	if size != uint32(4+len("COMMIT\x00")) || header[0] != 'C' {
		return 0, errors.New("isolated fixture did not receive command complete")
	}
	var payload [len("COMMIT\x00")]byte
	if _, err := io.ReadFull(c.Conn, payload[:]); err != nil {
		return 0, err
	}
	if string(payload[:]) != "COMMIT\x00" {
		return 0, errors.New("isolated fixture received a different command complete")
	}
	c.proved.Store(true)
	_ = c.Conn.Close()
	return 0, io.ErrUnexpectedEOF
}

func TestPostgresRevocationUnknownCommitReplaysOnceWithoutAnotherEpoch(t *testing.T) {
	d := database(t)
	ctx, now := t.Context(), baseTime()
	_ = d.runtime(t, "OPEN")
	intent := d.session(t, now)
	cfg := d.appConfig.Copy()
	if cfg.ConnConfig.TLSConfig != nil || len(cfg.ConnConfig.Fallbacks) != 0 {
		t.Fatal("commit response loss fixture requires disposable plain loopback")
	}
	var dropped atomic.Bool
	dial := cfg.ConnConfig.DialFunc
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &commitDropper{Conn: conn, proved: &dropped}, nil
	}
	s := mustStore(t, testPool(t, cfg))
	result, err := s.Begin(ctx, intent, now)
	if !errors.Is(err, app.ErrUncertain) || !dropped.Load() || result.ID != "" {
		t.Fatalf("actual lost COMMIT response did not produce an empty uncertain result: %v", err)
	}

	// The server applied the commit, but only a new read with the same ID can
	// supply the command; there is no second durable denial or epoch increment.
	s = mustStore(t, d.application)
	replayed, err := s.Begin(ctx, intent, now.Add(time.Second))
	if err != nil || replayed.Status != app.Pending || replayed.Epoch != 8 ||
		!replayed.AcceptedAt.Equal(now) || len(replayed.Participants) != 1 {
		t.Fatalf("uncertain accept did not recover the original row and roster: %v", err)
	}
	if epoch, revoked := sessionState(t, d, intent); epoch != 8 || revoked == nil || !revoked.Equal(now) {
		t.Fatal("recovery repeated the denial or lost the committed time")
	}
}

func TestPostgresRevocationNeverSilentlyAcceptsAnIncompleteRoster(t *testing.T) {
	d := database(t)
	ctx, now := t.Context(), baseTime()
	intent := d.session(t, now)
	s := mustStore(t, d.application)
	if _, err := s.Begin(ctx, intent, now); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("empty durable roster was accepted: %v", err)
	}
	// Seed more than the bounded wire result can faithfully report.
	t.Cleanup(func() {
		d.exec(t, `DELETE FROM hp_iam.runtime_registry
			WHERE runtime_id IN (SELECT md5('hit116 runtime '||i)::uuid FROM generate_series(1,$1::integer) AS i)`, maxRoster+1)
	})
	d.exec(t, `INSERT INTO hp_iam.runtime_registry(runtime_id,service,generation,boot_id,certificate_fingerprint,
		status,registered_at,last_observed_at)
		SELECT md5('hit116 runtime '||i)::uuid,'DELIVERY',1,md5('hit116 boot '||i)::uuid,
		       'sha256:'||repeat('a',64),'OPEN',statement_timestamp(),statement_timestamp()
		FROM generate_series(1,$1::integer) AS i`, maxRoster+1)
	if _, err := s.Begin(ctx, intent, now); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("oversized durable roster was silently truncated: %v", err)
	}
	if epoch, revoked := sessionState(t, d, intent); epoch != 7 || revoked != nil {
		t.Fatal("unsafe roster changed session before the transaction could commit")
	}
	if _, err := s.Get(ctx, intent.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unsafe roster left a revocation row: %v", err)
	}
}
