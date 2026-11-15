//go:build integration

package sessionissuer

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
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

type issuerDatabase struct {
	admin, app *pgxpool.Pool
	issuer     *Issuer
	issuerURL  string
}

func issuerUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("synthetic fixture random unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func issuerDB(t *testing.T) issuerDatabase {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_session_issuance.py for isolated PostgreSQL 18.7 tests")
	}
	adminCfg, adminErr := pgxpool.ParseConfig(adminDSN)
	appCfg, appErr := pgxpool.ParseConfig(appDSN)
	if adminErr != nil || appErr != nil {
		t.Fatal("invalid isolated fixture configuration; values withheld")
	}
	for _, cfg := range []*pgxpool.Config{adminCfg, appCfg} {
		if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") {
			t.Fatal("issuer tests require isolated loopback hit68_test_ database")
		}
		cfg.MaxConns, cfg.MinConns = 5, 0
	}
	if adminCfg.ConnConfig.User != "postgres" || appCfg.ConnConfig.User != "hp_hit81_app" || appCfg.ConnConfig.Database != adminCfg.ConnConfig.Database || appCfg.ConnConfig.Port != adminCfg.ConnConfig.Port {
		t.Fatal("actual fixture administrator and application logins must share one isolated database")
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), adminCfg)
	if err != nil {
		t.Fatal("fixture administrator unavailable")
	}
	t.Cleanup(admin.Close)
	app, err := pgxpool.NewWithConfig(context.Background(), appCfg)
	if err != nil {
		t.Fatal("fixture application unavailable")
	}
	t.Cleanup(app.Close)
	issuer, err := New(app)
	if err != nil {
		t.Fatal(err)
	}
	db := issuerDatabase{admin: admin, app: app, issuer: issuer, issuerURL: "https://issuer.invalid/" + issuerUUID(t)}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, sql := range []string{
			`DELETE FROM hp_iam.revocations WHERE subject_id IN(SELECT s.session_id FROM hp_iam.sessions s JOIN hp_iam.principals p USING(principal_id) WHERE p.oidc_issuer=$1 OR p.oidc_issuer=$1||'/other')`,
			`DELETE FROM hp_iam.sessions WHERE principal_id IN(SELECT principal_id FROM hp_iam.principals WHERE oidc_issuer=$1 OR oidc_issuer=$1||'/other')`,
			`DELETE FROM hp_iam.principals WHERE oidc_issuer=$1 OR oidc_issuer=$1||'/other'`,
		} {
			if _, err := admin.Exec(ctx, sql, db.issuerURL); err != nil {
				t.Error("isolated issuer fixture cleanup failed")
			}
		}
	})
	return db
}

func (db issuerDatabase) record(t *testing.T) (issuesession.Record, string) {
	t.Helper()
	r := validFixtureRecord()
	r.Issuer = db.issuerURL
	r.Subject = issuerUUID(t)
	r.PrincipalID = issuerUUID(t)
	r.SessionID = issuerUUID(t)
	raw := "HIT83-SYNTHETIC-SESSION-" + issuerUUID(t)
	r.SessionHash = sha256.Sum256([]byte(raw))
	r.CSRFHash = sha256.Sum256([]byte("HIT83-SYNTHETIC-CSRF-" + issuerUUID(t)))
	r.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	r.ExpiresAt = r.CreatedAt.Add(time.Hour)
	r.IdleExpiresAt = r.CreatedAt.Add(30 * time.Minute)
	return r, raw
}

func (db issuerDatabase) mustCreate(t *testing.T, r issuesession.Record) issuesession.Persisted {
	t.Helper()
	got, err := db.issuer.Create(context.Background(), r)
	if err != nil {
		t.Fatalf("isolated session creation failed with bounded error: %v", err)
	}
	if got.PrincipalID == "" || got.SessionID != r.SessionID {
		t.Fatal("committed result identity missing")
	}
	return got
}

func (db issuerDatabase) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.admin.Exec(ctx, sql, args...); err != nil {
		t.Fatal("isolated fixture statement failed; connection and arguments withheld")
	}
}

func TestPostgresIssuerAtomicAndCompatibleLookup(t *testing.T) {
	db := issuerDB(t)
	r, raw := db.record(t)
	got := db.mustCreate(t, r)
	if got.PrincipalID != r.PrincipalID {
		t.Fatal("new principal identity changed")
	}
	var sessionHash, csrfHash []byte
	var epoch int64
	var created, expires, idle time.Time
	var revoked bool
	err := db.admin.QueryRow(context.Background(), `SELECT session_hash,csrf_hash,epoch,created_at,expires_at,idle_expires_at,revoked_at IS NOT NULL FROM hp_iam.sessions WHERE session_id=$1`, r.SessionID).Scan(&sessionHash, &csrfHash, &epoch, &created, &expires, &idle, &revoked)
	if err != nil || hex.EncodeToString(sessionHash) != hex.EncodeToString(r.SessionHash[:]) || hex.EncodeToString(csrfHash) != hex.EncodeToString(r.CSRFHash[:]) || epoch != 7 || !created.Equal(r.CreatedAt) || !expires.Equal(r.ExpiresAt) || !idle.Equal(r.IdleExpiresAt) || revoked {
		t.Fatal("canonical persisted session does not match hashes, trusted epoch and timestamps")
	}
	// pgx text output of the complete row contains hashes; it cannot contain the
	// synthetic raw bearer value, provider proof or raw CSRF secret.
	var row string
	if err := db.admin.QueryRow(context.Background(), `SELECT row_to_json(s)::text FROM hp_iam.sessions s WHERE session_id=$1`, r.SessionID).Scan(&row); err != nil || strings.Contains(row, raw) {
		t.Fatal("raw session appeared in persistence")
	}
	lookup, err := sessionlookup.New(db.app)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := sessionlookup.Reference(raw)
	if err != nil {
		t.Fatal("fixture reference invalid")
	}
	snapshot, err := lookup.Lookup(context.Background(), reference)
	if err != nil || !snapshot.Found || snapshot.Revoked || !snapshot.CreatedAt.Equal(r.CreatedAt) || !snapshot.ExpiresAt.Equal(r.ExpiresAt) || !snapshot.IdleExpiresAt.Equal(r.IdleExpiresAt) {
		t.Fatal("optimized existing lookup cannot read newly issued row")
	}
	clock := issuerClock{at: r.CreatedAt.Add(time.Second)}
	auth, err := authenticatesession.New(lookup, clock)
	if err != nil {
		t.Fatal(err)
	}
	if result := auth.Execute(context.Background(), reference); !result.IsValid() {
		t.Fatalf("existing authentication did not accept committed session: %+v", result)
	}
	db.exec(t, `UPDATE hp_iam.sessions SET revoked_at=$2 WHERE session_id=$1`, r.SessionID, r.CreatedAt.Add(time.Second))
	if result := auth.Execute(context.Background(), reference); result.State != "denied" {
		t.Fatal("revoked issued session was accepted")
	}
	db.exec(t, `UPDATE hp_iam.sessions SET revoked_at=NULL WHERE session_id=$1`, r.SessionID)
	auth, _ = authenticatesession.New(lookup, issuerClock{at: r.ExpiresAt})
	if result := auth.Execute(context.Background(), reference); result.State != "denied" {
		t.Fatal("expired issued session was accepted")
	}
}

type issuerClock struct{ at time.Time }

func (c issuerClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: c.at, Trusted: true}, nil
}

func TestPostgresIssuerExistingPrincipalPreservesProfileAndIdentity(t *testing.T) {
	db := issuerDB(t)
	first, _ := db.record(t)
	db.mustCreate(t, first)
	profile := issuerUUID(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	db.exec(t, `UPDATE hp_iam.principals SET profile_content_ref=$2,profile_digest=$3 WHERE principal_id=$1`, first.PrincipalID, profile, digest)
	next, _ := db.record(t)
	next.Subject = first.Subject
	next.CreatedAt = first.CreatedAt.Add(time.Second)
	next.ExpiresAt = next.CreatedAt.Add(time.Hour)
	next.IdleExpiresAt = next.CreatedAt.Add(30 * time.Minute)
	got := db.mustCreate(t, next)
	if got.PrincipalID != first.PrincipalID {
		t.Fatal("existing provider subject was split")
	}
	var actualProfile, actualDigest string
	var created time.Time
	if err := db.admin.QueryRow(context.Background(), `SELECT profile_content_ref::text,profile_digest,created_at FROM hp_iam.principals WHERE principal_id=$1`, first.PrincipalID).Scan(&actualProfile, &actualDigest, &created); err != nil || actualProfile != profile || actualDigest != digest || !created.Equal(first.CreatedAt) {
		t.Fatal("existing principal profile or creation time changed")
	}
	// An email-shaped subject is merely opaque provider data. Another provider or
	// another subject is never merged using an email/profile fallback.
	emailSubject := "same-email@example.invalid"
	a, _ := db.record(t)
	a.Subject = emailSubject
	left := db.mustCreate(t, a)
	b, _ := db.record(t)
	b.Subject = emailSubject
	b.Issuer += "/other"
	right := db.mustCreate(t, b)
	c, _ := db.record(t)
	c.Subject = "different-subject"
	third := db.mustCreate(t, c)
	if left.PrincipalID == right.PrincipalID || left.PrincipalID == third.PrincipalID || right.PrincipalID == third.PrincipalID {
		t.Fatal("distinct provider/subject identities merged")
	}
}

func TestPostgresIssuerConcurrentDeduplication(t *testing.T) {
	db := issuerDB(t)
	const count = 8
	records := make([]issuesession.Record, count)
	subject := issuerUUID(t)
	for n := range records {
		records[n], _ = db.record(t)
		records[n].Subject = subject
	}
	results := make([]issuesession.Persisted, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for n := range records {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			results[n], errs[n] = db.issuer.Create(context.Background(), records[n])
		}(n)
	}
	close(start)
	wg.Wait()
	for n := range results {
		if errs[n] != nil || results[n].PrincipalID == "" || results[n].PrincipalID != results[0].PrincipalID || results[n].SessionID != records[n].SessionID {
			t.Fatalf("concurrent verified issuance %d did not share the canonical principal: %v", n, errs[n])
		}
	}
	var principals, sessions int
	if err := db.admin.QueryRow(context.Background(), `SELECT count(*),(SELECT count(*) FROM hp_iam.sessions WHERE principal_id=$3) FROM hp_iam.principals WHERE oidc_issuer=$1 AND oidc_subject=$2`, db.issuerURL, subject, results[0].PrincipalID).Scan(&principals, &sessions); err != nil || principals != 1 || sessions != count {
		t.Fatal("unique constraint/transaction did not deduplicate principal atomically")
	}
}

func TestPostgresIssuerDisabledAndPendingRevocations(t *testing.T) {
	for _, state := range []string{"disabled", "PENDING", "INCIDENT", "EFFECTIVE"} {
		t.Run(state, func(t *testing.T) {
			db := issuerDB(t)
			first, _ := db.record(t)
			db.mustCreate(t, first)
			if state == "disabled" {
				db.exec(t, `UPDATE hp_iam.principals SET disabled_at=$2 WHERE principal_id=$1`, first.PrincipalID, first.CreatedAt)
			} else {
				effective := any(nil)
				if state == "EFFECTIVE" {
					effective = first.CreatedAt
				}
				db.exec(t, `INSERT INTO hp_iam.revocations(revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,effective_at,reason_code) VALUES($1,'SESSION',$2,8,$3,$3::timestamptz+interval'5seconds',$4,$5,'SYNTHETIC')`, issuerUUID(t), first.SessionID, first.CreatedAt, state, effective)
			}
			next, _ := db.record(t)
			next.Subject = first.Subject
			got, err := db.issuer.Create(context.Background(), next)
			if state == "EFFECTIVE" {
				if err != nil || got.PrincipalID != first.PrincipalID {
					t.Fatal("completed prior revocation incorrectly denied a new trusted login")
				}
				return
			}
			if !errors.Is(err, ErrDenied) || got != (issuesession.Persisted{}) {
				t.Fatal("disabled/pending/incident identity admitted")
			}
			var sessions int
			if err := db.admin.QueryRow(context.Background(), `SELECT count(*) FROM hp_iam.sessions WHERE principal_id=$1`, first.PrincipalID).Scan(&sessions); err != nil || sessions != 1 {
				t.Fatal("denied issuance mutated sessions")
			}
		})
	}
}

func TestPostgresIssuerRollbackOnInsertFailure(t *testing.T) {
	db := issuerDB(t)
	existing, _ := db.record(t)
	db.mustCreate(t, existing)
	for _, kind := range []string{"session-id", "session-hash"} {
		t.Run(kind, func(t *testing.T) {
			candidate, _ := db.record(t)
			if kind == "session-id" {
				candidate.SessionID = existing.SessionID
			} else {
				candidate.SessionHash = existing.SessionHash
			}
			got, err := db.issuer.Create(context.Background(), candidate)
			if !errors.Is(err, ErrUnavailable) || got != (issuesession.Persisted{}) {
				t.Fatal("duplicate insertion returned success or leaked database detail")
			}
			var principal bool
			if err := db.admin.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM hp_iam.principals WHERE principal_id=$1)`, candidate.PrincipalID).Scan(&principal); err != nil || principal {
				t.Fatal("failed session insert left a principal behind")
			}
		})
	}
}

func TestPostgresIssuerCancellationAndBoundedLock(t *testing.T) {
	db := issuerDB(t)
	existing, _ := db.record(t)
	db.mustCreate(t, existing)
	tx, err := db.admin.Begin(context.Background())
	if err != nil {
		t.Fatal("fixture lock transaction unavailable")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `SELECT principal_id FROM hp_iam.principals WHERE principal_id=$1 FOR UPDATE`, existing.PrincipalID); err != nil {
		t.Fatal("fixture principal lock unavailable")
	}
	for _, timeout := range []time.Duration{30 * time.Millisecond, 5 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			candidate, _ := db.record(t)
			candidate.Subject = existing.Subject
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			started := time.Now()
			got, err := db.issuer.Create(ctx, candidate)
			elapsed := time.Since(started)
			if got != (issuesession.Persisted{}) || !errors.Is(err, context.DeadlineExceeded) || elapsed > 2500*time.Millisecond {
				t.Fatalf("deadline did not bound blocked transaction: %v, %v", err, elapsed)
			}
			var exists bool
			if err := db.admin.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM hp_iam.sessions WHERE session_id=$1)`, candidate.SessionID).Scan(&exists); err != nil || exists {
				t.Fatal("cancelled lock wait persisted a session")
			}
		})
	}
}

func TestPostgresIssuerAuthorityAndDatabaseFailures(t *testing.T) {
	db := issuerDB(t)
	for _, env := range []string{"HITPIPE_IAM_TEST_ADMIN_DSN", "HITPIPE_IAM_TEST_WORKER_DSN", "HITPIPE_IAM_TEST_PURGE_DSN", "HITPIPE_IAM_MIGRATOR_DSN"} {
		t.Run(env, func(t *testing.T) {
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip("optional forbidden authority fixture not configured")
			}
			cfg, err := pgxpool.ParseConfig(dsn)
			if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Database != db.app.Config().ConnConfig.Database || cfg.ConnConfig.Port != db.app.Config().ConnConfig.Port {
				t.Fatal("forbidden-authority fixture must use same isolated loopback database")
			}
			cfg.MaxConns = 5
			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal("authority fixture unavailable")
			}
			defer pool.Close()
			bad, _ := New(pool)
			candidate, _ := db.record(t)
			if got, err := bad.Create(context.Background(), candidate); got != (issuesession.Persisted{}) || !errors.Is(err, ErrConfiguration) {
				t.Fatal("forbidden runtime authority admitted")
			}
		})
	}
	// A valid application login in a mixed-purpose database is not an IAM
	// database authority. Remove only this test's randomly named synthetic table.
	table := "hit83_foreign_" + strings.ReplaceAll(issuerUUID(t), "-", "")
	db.exec(t, `CREATE TABLE public.`+table+`(synthetic integer)`)
	t.Cleanup(func() { db.exec(t, `DROP TABLE IF EXISTS public.`+table) })
	candidate, _ := db.record(t)
	if got, err := db.issuer.Create(context.Background(), candidate); got != (issuesession.Persisted{}) || !errors.Is(err, ErrConfiguration) {
		t.Fatal("mixed-purpose database admitted")
	}
	db.exec(t, `DROP TABLE public.`+table)
	closedCfg := db.app.Config()
	closed, err := pgxpool.NewWithConfig(context.Background(), closedCfg)
	if err != nil {
		t.Fatal("closed pool fixture unavailable")
	}
	bad, _ := New(closed)
	closed.Close()
	if got, err := bad.Create(context.Background(), candidate); got != (issuesession.Persisted{}) || !errors.Is(err, ErrUnavailable) {
		t.Fatal("closed database returned success")
	}
}

type lostCommitAcknowledgement struct{ transaction }

func (tx lostCommitAcknowledgement) Commit(ctx context.Context) error {
	if err := tx.transaction.Commit(ctx); err != nil {
		return err
	}
	return errors.New("synthetic lost commit acknowledgement")
}
func TestPostgresIssuerCommitUncertainDoesNotReturnSuccess(t *testing.T) {
	db := issuerDB(t)
	candidate, _ := db.record(t)
	original := db.issuer.begin
	db.issuer.begin = func(ctx context.Context) (transaction, error) {
		tx, err := original(ctx)
		if err != nil {
			return nil, err
		}
		return lostCommitAcknowledgement{tx}, nil
	}
	got, err := db.issuer.Create(context.Background(), candidate)
	if !errors.Is(err, ErrUnavailable) || got != (issuesession.Persisted{}) {
		t.Fatal("unknown commit outcome leaked success")
	}
	// Prove the fault was after an actual commit, rather than a mocked rollback.
	var exists bool
	if err := db.admin.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM hp_iam.sessions WHERE session_id=$1 AND principal_id=$2)`, candidate.SessionID, candidate.PrincipalID).Scan(&exists); err != nil || !exists {
		t.Fatal("commit-uncertain probe did not commit real session")
	}
}

// These controlled test ports stand for already-verified provider authority and
// an atomic one-use attempt store. They are not production OIDC implementations.
type oneUseIssuerVerifier struct {
	mu    sync.Mutex
	proof string
	login issuesession.VerifiedLogin
	used  bool
}

func (v *oneUseIssuerVerifier) Consume(ctx context.Context, proof string) (issuesession.VerifiedLogin, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ctx.Err() != nil || proof != v.proof || v.used {
		return issuesession.VerifiedLogin{}, errors.New("synthetic confirmation denied")
	}
	v.used = true
	return v.login, nil
}

type issuerGuard struct{}

func (issuerGuard) Check(context.Context, issuesession.Identity) (issuesession.GuardDecision, error) {
	return issuesession.GuardDecision{State: issuesession.IssuanceAllowed, Epoch: 7}, nil
}

func TestPostgresIssuerServiceOneUseProofAndLookup(t *testing.T) {
	db := issuerDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	verifier := &oneUseIssuerVerifier{proof: "HIT83-SYNTHETIC-ONE-USE-PROOF", login: issuesession.VerifiedLogin{Issuer: db.issuerURL, Subject: issuerUUID(t), EmailVerified: true, AttemptCreatedAt: now.Add(-time.Second), AttemptExpiresAt: now.Add(time.Minute)}}
	service, err := issuesession.New(verifier, issuerClock{at: now}, issuerGuard{}, db.issuer, issuesession.Policy{AbsoluteLifetime: time.Hour, IdleLifetime: 30 * time.Minute})
	if err != nil {
		t.Fatal("verified service fixture configuration invalid")
	}
	const count = 8
	var results [count]issuesession.Result
	var errs [count]error
	var wg sync.WaitGroup
	start := make(chan struct{})
	for n := range count {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			results[n], errs[n] = service.Execute(context.Background(), verifier.proof)
		}(n)
	}
	close(start)
	wg.Wait()
	successes := 0
	var issued issuesession.Result
	for n := range count {
		if errs[n] == nil {
			if !results[n].IsIssued() {
				t.Fatal("nil error without issued result")
			}
			successes++
			issued = results[n]
		} else if !errors.Is(errs[n], issuesession.ErrConfirmationDenied) || results[n].IsIssued() || results[n].Session.Reveal() != "" || results[n].CSRF.Reveal() != "" {
			t.Fatal("reused proof did not fail closed")
		}
	}
	if successes != 1 {
		t.Fatalf("one-use proof issued %d sessions", successes)
	}
	var principals, sessions int
	if err := db.admin.QueryRow(context.Background(), `SELECT count(*),(SELECT count(*) FROM hp_iam.sessions WHERE principal_id=$2) FROM hp_iam.principals WHERE oidc_issuer=$1`, db.issuerURL, issued.PrincipalID).Scan(&principals, &sessions); err != nil || principals != 1 || sessions != 1 {
		t.Fatal("one-use service boundary did not persist exactly one principal/session")
	}
	var persistedJSON string
	if err := db.admin.QueryRow(context.Background(), `SELECT row_to_json(s)::text FROM hp_iam.sessions s WHERE session_id=$1`, issued.SessionID).Scan(&persistedJSON); err != nil {
		t.Fatal("issued row could not be inspected")
	}
	for _, raw := range []string{issued.Session.Reveal(), issued.CSRF.Reveal(), verifier.proof} {
		if strings.Contains(persistedJSON, raw) {
			t.Fatal("raw session, CSRF or login proof persisted")
		}
	}
	lookup, err := sessionlookup.New(db.app)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := sessionlookup.Reference(issued.Session.Reveal())
	if err != nil {
		t.Fatal("issued session does not meet existing reference contract")
	}
	auth, err := authenticatesession.New(lookup, issuerClock{at: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Execute(context.Background(), reference).IsValid() {
		t.Fatal("service-issued committed session not accepted by existing optimized consumer")
	}
	// The CSRF hash must select nothing: it is a distinct secret and never the
	// authentication handle. Public UUIDs likewise cannot select the session.
	csrfRef, err := sessionlookup.Reference(issued.CSRF.Reveal())
	if err != nil {
		t.Fatal("synthetic CSRF length invalid")
	}
	if snapshot, err := lookup.Lookup(context.Background(), csrfRef); err != nil || snapshot.Found {
		t.Fatal("CSRF secret selected a session")
	}
	if snapshot, err := lookup.Lookup(context.Background(), issued.SessionID); err == nil || snapshot.Found {
		t.Fatal("public session UUID selected a session")
	}
}
