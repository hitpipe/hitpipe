//go:build integration

package profileupdate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionissuer"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testClock struct{}

func (testClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: true}, nil
}

type verifiedFixtureLogin struct {
	login    issuesession.VerifiedLogin
	consumed bool
}

func (l *verifiedFixtureLogin) Consume(context.Context, string) (issuesession.VerifiedLogin, error) {
	if l.consumed {
		return issuesession.VerifiedLogin{}, errors.New("fixture login already consumed")
	}
	l.consumed = true
	return l.login, nil
}

type allowFixtureIssuance struct{}

func (allowFixtureIssuance) Check(context.Context, issuesession.Identity) (issuesession.GuardDecision, error) {
	return issuesession.GuardDecision{State: issuesession.IssuanceAllowed, Epoch: 1}, nil
}

type fixture struct {
	t                 *testing.T
	admin, iam        *pgxpool.Pool
	store             *Store
	issued            issuesession.Result
	frozen, untouched app.Pointer
	reference, other  string
	caller, local     runtimeaccess.Registration
}

func uuid(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("fixture randomness unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func nextPointer(t *testing.T, ch string) app.Pointer {
	return app.Pointer{Ref: uuid(t), Digest: "sha256:" + strings.Repeat(ch, 64)}
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
	open := func(config *pgxpool.Config) *pgxpool.Pool {
		t.Helper()
		config.MaxConns, config.MinConns = 5, 0
		pool, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal("fixture pool unavailable")
		}
		t.Cleanup(pool.Close)
		return pool
	}
	f := fixture{t: t, admin: open(admin), iam: open(iam), other: uuid(t),
		frozen: nextPointer(t, "a"), untouched: nextPointer(t, "b"),
		caller: runtimeaccess.Registration{RuntimeID: uuid(t), Service: "BFF", Generation: 1,
			BootID: uuid(t), CertificateFingerprint: "sha256:" + strings.Repeat("c", 64)},
		local: runtimeaccess.Registration{RuntimeID: uuid(t), Service: "IAM", Generation: 1,
			BootID: uuid(t), CertificateFingerprint: "sha256:" + strings.Repeat("d", 64)}}
	var version string
	if f.admin.QueryRow(t.Context(), `SHOW server_version_num`).Scan(&version) != nil || version != "180007" {
		t.Fatal("fixture requires PostgreSQL 18.7")
	}

	issuer, err := sessionissuer.New(f.iam)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	login := &verifiedFixtureLogin{login: issuesession.VerifiedLogin{
		Issuer: "https://profile-writer-fixture.invalid/" + uuid(t), Subject: uuid(t), EmailVerified: true,
		AttemptCreatedAt: now.Add(-time.Second), AttemptExpiresAt: now.Add(time.Minute),
	}}
	service, err := issuesession.New(login, testClock{}, allowFixtureIssuance{}, issuer,
		issuesession.Policy{AbsoluteLifetime: time.Hour, IdleLifetime: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	f.issued, err = service.Execute(t.Context(), "profile-writer-fixture-proof")
	if err != nil || !f.issued.IsIssued() {
		t.Fatal("real session issuer did not create a session")
	}
	f.reference, err = sessionlookup.Reference(f.issued.Session.Reveal())
	if err != nil {
		t.Fatal("issuer reference unavailable")
	}
	t.Cleanup(func() {
		f.exec(`DELETE FROM hp_iam.revocations WHERE scope='SESSION' AND subject_id=$1`, f.issued.SessionID)
		f.exec(`DELETE FROM hp_iam.runtime_registry WHERE runtime_id IN ($1,$2)`, f.caller.RuntimeID, f.local.RuntimeID)
		f.exec(`DELETE FROM hp_iam.sessions WHERE session_id=$1`, f.issued.SessionID)
		f.exec(`DELETE FROM hp_iam.principals WHERE principal_id IN($1,$2)`, f.issued.PrincipalID, f.other)
	})
	f.exec(`UPDATE hp_iam.principals SET profile_content_ref=$2,profile_digest=$3 WHERE principal_id=$1`, f.issued.PrincipalID, f.frozen.Ref, f.frozen.Digest)
	f.exec(`INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at,profile_content_ref,profile_digest)
 VALUES($1,'https://profile-writer-other-fixture.invalid',$1::uuid::text,$2,$3,$4)`, f.other, now, f.untouched.Ref, f.untouched.Digest)
	for _, runtime := range []runtimeaccess.Registration{f.caller, f.local} {
		f.exec(`INSERT INTO hp_iam.runtime_registry
 (runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at)
 VALUES($1,$2,$3,$4,$5,'OPEN',$6,$6)`, runtime.RuntimeID, runtime.Service, runtime.Generation,
			runtime.BootID, runtime.CertificateFingerprint, now)
	}
	f.store, err = New(f.iam, testClock{})
	if err != nil {
		t.Fatal(err)
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
			f.t.Fatalf("fixture statement failed (SQLSTATE %s)", failure.Code)
		}
		f.t.Fatal("fixture statement failed")
	}
}

func (f fixture) identity() app.Identity {
	f.t.Helper()
	who, err := f.store.Identity(f.t.Context(), f.reference, f.caller, f.local)
	if err != nil || who.PrincipalID != f.issued.PrincipalID || who.SessionID != f.issued.SessionID ||
		who.SessionEpoch != 1 || who.CSRFHash != sha256.Sum256([]byte(f.issued.CSRF.Reveal())) ||
		!who.CreatedAt.Equal(f.issued.CreatedAt.Truncate(time.Microsecond)) ||
		!who.ExpiresAt.Equal(f.issued.ExpiresAt.Truncate(time.Microsecond)) ||
		!who.ValidUntil.Equal(f.issued.IdleExpiresAt.Truncate(time.Microsecond)) {
		f.t.Fatalf("exact persisted session identity unavailable: %v", err)
	}
	return who
}

func (f fixture) readPointer(principal string) app.Pointer {
	f.t.Helper()
	var pointer app.Pointer
	err := f.admin.QueryRow(f.t.Context(), `SELECT profile_content_ref::text,profile_digest FROM hp_iam.principals WHERE principal_id=$1`,
		principal).Scan(&pointer.Ref, &pointer.Digest)
	if err != nil {
		f.t.Fatal("fixture could not independently read the profile pointer")
	}
	return pointer
}

func TestPostgresProfileUpdateSerializesCASAndKeepsOwner(t *testing.T) {
	f := isolatedFixture(t)
	who := f.identity()
	results := make(chan error, 2)
	pointers := [2]app.Pointer{nextPointer(t, "e"), nextPointer(t, "f")}
	start := make(chan struct{})
	for _, next := range pointers {
		go func() {
			<-start
			results <- f.store.WithProfile(t.Context(), who, func(ctx context.Context, tx app.ProfileTx) error {
				return tx.Switch(ctx, f.frozen, next)
			})
		}()
	}
	close(start)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, app.ErrStale)) || (second == nil && errors.Is(first, app.ErrStale))) {
		t.Fatalf("concurrent CAS did not yield one exact win and one stale result: %v, %v", first, second)
	}
	if actual := f.readPointer(who.PrincipalID); actual != pointers[0] && actual != pointers[1] {
		t.Fatal("CAS winner not committed")
	}
	if f.readPointer(f.other) != f.untouched {
		t.Fatal("unrelated principal changed")
	}
}

func TestPostgresProfileUpdateRollsBackAndRechecksCurrentIdentity(t *testing.T) {
	f := isolatedFixture(t)
	who := f.identity()
	next := nextPointer(t, "e")
	err := f.store.WithProfile(t.Context(), who, func(ctx context.Context, tx app.ProfileTx) error {
		if err := tx.Switch(ctx, tx.Pointer(), next); err != nil {
			return err
		}
		return app.ErrConflict
	})
	if !errors.Is(err, app.ErrConflict) || f.readPointer(who.PrincipalID) != f.frozen {
		t.Fatal("callback failure did not roll back the SQL pointer")
	}
	cancelCtx, cancel := context.WithCancel(t.Context())
	err = f.store.WithProfile(cancelCtx, who, func(ctx context.Context, tx app.ProfileTx) error {
		if err := tx.Switch(ctx, tx.Pointer(), next); err != nil {
			return err
		}
		cancel()
		return nil
	})
	cancel()
	if !errors.Is(err, app.ErrUnavailable) || f.readPointer(who.PrincipalID) != f.frozen {
		t.Fatal("cancellation before commit did not roll back the SQL pointer")
	}

	called := false
	use := func(context.Context, app.ProfileTx) error {
		called = true
		return nil
	}
	f.exec(`UPDATE hp_iam.sessions SET epoch=2 WHERE session_id=$1`, who.SessionID)
	if err := f.store.WithProfile(t.Context(), who, use); !errors.Is(err, app.ErrStale) || called {
		t.Fatal("an old session epoch reached the profile callback")
	}
	f.exec(`UPDATE hp_iam.sessions SET epoch=1 WHERE session_id=$1`, who.SessionID)
	f.exec(`UPDATE hp_iam.principals SET disabled_at=clock_timestamp() WHERE principal_id=$1`, who.PrincipalID)
	if err := f.store.WithProfile(t.Context(), who, use); !errors.Is(err, app.ErrUnauthenticated) || called {
		t.Fatal("a disabled owner reached the profile callback")
	}
	f.exec(`UPDATE hp_iam.principals SET disabled_at=NULL WHERE principal_id=$1`, who.PrincipalID)
	f.exec(`UPDATE hp_iam.runtime_registry SET status='CLOSED' WHERE runtime_id=$1`, f.caller.RuntimeID)
	if err := f.store.WithProfile(t.Context(), who, use); !errors.Is(err, app.ErrDenied) || called {
		t.Fatal("a closed BFF runtime reached the callback")
	}
	f.exec(`UPDATE hp_iam.runtime_registry SET status='OPEN' WHERE runtime_id=$1`, f.caller.RuntimeID)
	f.exec(`INSERT INTO hp_iam.revocations(revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,'SESSION',$2,2,statement_timestamp(),statement_timestamp()+interval '5 seconds','PENDING','PROFILE_FIXTURE')`,
		uuid(t), who.SessionID)
	if err := f.store.WithProfile(t.Context(), who, use); !errors.Is(err, app.ErrUnauthenticated) || called {
		t.Fatal("a pending global session revoke reached the profile callback")
	}
	adminStore, err := New(f.admin, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	if err := adminStore.WithProfile(t.Context(), who, use); !errors.Is(err, app.ErrUnavailable) || called {
		t.Fatal("the administrator role reached the profile callback")
	}
}
