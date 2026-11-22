//go:build integration

package casedeletion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testClock struct {
	at      time.Time
	trusted bool
}

func (c testClock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: c.at, Trusted: c.trusted}, ctx.Err()
}

type fixture struct {
	admin, application               *pgxpool.Pool
	now                              time.Time
	workspaces, principals, sessions [2]string
	secrets                          [2]string
}

func uuid(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("synthetic fixture entropy unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func testDatabase(t *testing.T) (admin, application *pgxpool.Pool) {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_authorization.py for isolated PostgreSQL 18.7")
	}
	adminCfg, aErr := pgxpool.ParseConfig(adminDSN)
	appCfg, bErr := pgxpool.ParseConfig(appDSN)
	if aErr != nil || bErr != nil {
		t.Fatal("invalid isolated PostgreSQL configuration; values withheld")
	}
	for _, config := range []*pgxpool.Config{adminCfg, appCfg} {
		if config.ConnConfig.Host != "127.0.0.1" ||
			!strings.HasPrefix(config.ConnConfig.Database, "hit68_test_") ||
			config.ConnConfig.Password != "" || len(config.ConnConfig.Fallbacks) != 0 {
			t.Fatal("fixture requires the existing isolated loopback database and synthetic roles")
		}
		config.MinConns, config.MaxConns = 0, 4
	}
	if adminCfg.ConnConfig.Database != appCfg.ConnConfig.Database || adminCfg.ConnConfig.Port != appCfg.ConnConfig.Port ||
		adminCfg.ConnConfig.User != "postgres" || appCfg.ConnConfig.User != "hp_hit81_app" {
		t.Fatal("fixture requires accepted synthetic application and setup identities in the same database")
	}
	admin, err := pgxpool.NewWithConfig(t.Context(), adminCfg)
	if err != nil {
		t.Fatal("fixture setup connection unavailable")
	}
	t.Cleanup(admin.Close)
	application, err = pgxpool.NewWithConfig(t.Context(), appCfg)
	if err != nil {
		t.Fatal("fixture application connection unavailable")
	}
	t.Cleanup(application.Close)
	var version string
	if err := admin.QueryRow(t.Context(), `SHOW server_version`).Scan(&version); err != nil || version != "18.7" {
		t.Fatal("fixture requires the pinned PostgreSQL 18.7 database")
	}
	return admin, application
}

func (f fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if _, err := f.admin.Exec(ctx, sql, args...); err != nil {
		var pgError *pgconn.PgError
		key := "unavailable"
		if errors.As(err, &pgError) {
			key = pgError.Code
		}
		t.Fatalf("synthetic SQL fixture failed (SQLSTATE=%s, statement digest=%x)", key, sha256.Sum256([]byte(sql)))
	}
}

func seed(t *testing.T) fixture {
	t.Helper()
	admin, application := testDatabase(t)
	f := fixture{admin: admin, application: application, now: time.Now().UTC().Truncate(time.Microsecond)}
	for i := range f.workspaces {
		f.workspaces[i], f.principals[i], f.sessions[i] = uuid(t), uuid(t), uuid(t)
		f.secrets[i] = "HIT154-SYNTHETIC-OWNER-SESSION-" + uuid(t)
	}
	t.Cleanup(func() {
		for i := range f.workspaces {
			f.exec(t, `DELETE FROM hp_iam.memberships WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[i], f.principals[i])
			f.exec(t, `DELETE FROM hp_iam.sessions WHERE session_id=$1`, f.sessions[i])
			f.exec(t, `DELETE FROM hp_iam.workspaces WHERE workspace_id=$1`, f.workspaces[i])
			f.exec(t, `DELETE FROM hp_iam.principals WHERE principal_id=$1`, f.principals[i])
		}
	})
	for i, workspace := range f.workspaces {
		principal := f.principals[i]
		f.exec(t, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
 VALUES($1::uuid,'https://fixture.invalid',$1::text,$2)`, principal, f.now.Add(-time.Hour))
		digest, csrf := sha256.Sum256([]byte(f.secrets[i])), sha256.Sum256([]byte("HIT154 synthetic csrf "+uuid(t)))
		f.exec(t, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
 VALUES($1,$2,$3,$4,$5,$6,$7,3)`, f.sessions[i], digest[:], principal, csrf[:],
			f.now.Add(-time.Minute), f.now.Add(time.Hour), f.now.Add(30*time.Minute))
		f.exec(t, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
 VALUES($1,$2,2,7,'ACTIVE',$3,$3)`, workspace, uuid(t), f.now)
		f.exec(t, `INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at)
 VALUES($1,$2,'OWNER','ACTIVE',5,$3)`, workspace, principal, f.now)
	}
	return f
}

func (f fixture) authority(t *testing.T) *OwnerAuthority {
	t.Helper()
	authority, err := New(f.application, testClock{f.now, true})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func (f fixture) check(index int) app.OwnerCheck {
	return app.OwnerCheck{Session: f.secrets[index], WorkspaceID: f.workspaces[index]}
}

func requireDenied(t *testing.T, a *OwnerAuthority, input app.OwnerCheck, expected error) {
	t.Helper()
	var committed bool
	err := a.WithCurrentOwner(t.Context(), input, func(context.Context, app.OwnerIdentity) error {
		committed = true
		return nil
	})
	if !errors.Is(err, expected) || committed {
		t.Fatalf("authority rejection: error=%v commit_called=%t", err, committed)
	}
}

func TestPostgresCaseDeletionOnlyCurrentOwner(t *testing.T) {
	f := seed(t)
	a := f.authority(t)
	for i := range f.workspaces {
		calls := 0
		err := a.WithCurrentOwner(t.Context(), f.check(i), func(ctx context.Context, identity app.OwnerIdentity) error {
			calls++
			if ctx.Err() != nil || identity.PrincipalID != f.principals[i] {
				t.Error("wrong current owner authority observed")
			}
			return nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("synthetic current owner not accepted exactly once: error=%v callbacks=%d", err, calls)
		}
	}
	requireDenied(t, a, app.OwnerCheck{Session: f.secrets[0], WorkspaceID: f.workspaces[1]}, app.ErrUnauthorized)
	requireDenied(t, a, app.OwnerCheck{Session: f.sessions[0], WorkspaceID: f.workspaces[0]}, app.ErrUnauthorized)
	requireDenied(t, a, app.OwnerCheck{Session: "HIT154 fake owner session value " + uuid(t), WorkspaceID: f.workspaces[0]}, app.ErrUnauthorized)

	f.exec(t, `UPDATE hp_iam.memberships SET role='OPERATOR',revision=revision+1 WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[0], f.principals[0])
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	f.exec(t, `UPDATE hp_iam.memberships SET role='OWNER',status='REVOKED',revoked_at=$3,revision=revision+1
 WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[0], f.principals[0], f.now)
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	f.exec(t, `UPDATE hp_iam.memberships SET status='ACTIVE',revoked_at=NULL,revision=revision+1
 WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[0], f.principals[0])
	f.exec(t, `UPDATE hp_iam.workspaces SET lifecycle='DELETING' WHERE workspace_id=$1`, f.workspaces[0])
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	f.exec(t, `UPDATE hp_iam.workspaces SET lifecycle='ACTIVE' WHERE workspace_id=$1`, f.workspaces[0])
	f.exec(t, `UPDATE hp_iam.principals SET disabled_at=$2 WHERE principal_id=$1`, f.principals[0], f.now)
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	f.exec(t, `UPDATE hp_iam.principals SET disabled_at=NULL WHERE principal_id=$1`, f.principals[0])
	f.exec(t, `UPDATE hp_iam.sessions SET idle_expires_at=$2 WHERE session_id=$1`, f.sessions[0], f.now)
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	f.exec(t, `UPDATE hp_iam.sessions SET idle_expires_at=$2,revoked_at=$3 WHERE session_id=$1`, f.sessions[0], f.now.Add(time.Minute), f.now)
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	f.exec(t, `UPDATE hp_iam.sessions SET revoked_at=NULL WHERE session_id=$1`, f.sessions[0])

	revocation := uuid(t)
	t.Cleanup(func() { f.exec(t, `DELETE FROM hp_iam.revocations WHERE revocation_id=$1`, revocation) })
	f.exec(t, `INSERT INTO hp_iam.revocations
 (revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,'SESSION',$2,4,$3,$4,'PENDING','HIT154_TEST')`, revocation, f.sessions[0], f.now, f.now.Add(5*time.Second))
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	if err := a.WithCurrentOwner(t.Context(), f.check(1), func(context.Context, app.OwnerIdentity) error { return nil }); err != nil {
		t.Fatalf("unrelated workspace was denied: %v", err)
	}
	t.Log("PG18.7: both exact owners accepted; cross-workspace, public session ID, unknown session, OPERATOR, role revoke, deleting workspace, disabled principal, expired and revoked sessions, pending SESSION deny all fail closed; unrelated workspace remains allowed")
}

func TestPostgresCaseDeletionOwnerRemainsCurrentThroughRegistryCommit(t *testing.T) {
	f := seed(t)
	a := f.authority(t)
	entered, release, result := make(chan app.OwnerIdentity, 1), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	go func() {
		result <- a.WithCurrentOwner(t.Context(), f.check(0), func(ctx context.Context, identity app.OwnerIdentity) error {
			entered <- identity
			select {
			case <-release:
				return ctx.Err()
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case owner := <-entered:
		if owner.PrincipalID != f.principals[0] {
			t.Fatal("callback did not hold the actual owner")
		}
	case err := <-result:
		t.Fatalf("owner check ended before local callback: %v", err)
	case <-time.After(time.Second):
		t.Fatal("owner check did not enter local callback")
	}
	conn, err := f.admin.Acquire(t.Context())
	if err != nil {
		t.Fatal("synthetic concurrent writer unavailable")
	}
	defer conn.Release()
	var pgError *pgconn.PgError
	if _, err := conn.Exec(t.Context(), `SET lock_timeout='100ms'`); err != nil {
		t.Fatal("could not bound synthetic concurrent writer")
	}
	_, err = conn.Exec(t.Context(), `UPDATE hp_iam.memberships SET role='OPERATOR',revision=revision+1
 WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[0], f.principals[0])
	if !errors.As(err, &pgError) || pgError.Code != "55P03" {
		t.Fatal("owner role change was not locked during local commit")
	}
	// The other transaction can't insert an overlay even though the current
	// session had no row in revocations to row-lock.
	_, err = conn.Exec(t.Context(), `INSERT INTO hp_iam.revocations
 (revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,'SESSION',$2,4,$3,$4,'PENDING','HIT154_TEST')`, uuid(t), f.sessions[0], f.now, f.now.Add(5*time.Second))
	if !errors.As(err, &pgError) || pgError.Code != "55P03" {
		t.Fatal("new revocation overlay was not locked during local commit")
	}
	finish()
	if err := <-result; err != nil {
		t.Fatalf("local callback did not complete: %v", err)
	}
	if _, err := conn.Exec(t.Context(), `UPDATE hp_iam.memberships SET role='OPERATOR',revision=revision+1
 WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[0], f.principals[0]); err != nil {
		t.Fatal("lock did not release after local commit")
	}
	requireDenied(t, a, f.check(0), app.ErrUnauthorized)
	t.Log("actual concurrent role update and new SESSION deny were blocked while publication callback held current IAM authority; later role downgrade was observed")
}

func TestPostgresCaseDeletionRejectsWrongDatabaseAuthorityAndUntrustedTime(t *testing.T) {
	f := seed(t)
	overprivileged, err := New(f.admin, testClock{f.now, true})
	if err != nil {
		t.Fatal(err)
	}
	requireDenied(t, overprivileged, f.check(0), app.ErrUnavailable)
	untrusted, err := New(f.application, testClock{f.now, false})
	if err != nil {
		t.Fatal(err)
	}
	requireDenied(t, untrusted, f.check(0), app.ErrUnavailable)
	a := f.authority(t)
	f.exec(t, `UPDATE hp_iam.sessions SET idle_expires_at=$2 WHERE session_id=$1`, f.sessions[0], f.now.Add(25*time.Millisecond))
	var exposed bool
	err = a.WithCurrentOwner(t.Context(), f.check(0), func(ctx context.Context, _ app.OwnerIdentity) error {
		exposed = true
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 25*time.Millisecond {
			t.Error("commit context not bounded by the remaining session lifetime")
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if !exposed || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("publication window escaped the session boundary: callback=%t, error=%v", exposed, err)
	}
	t.Log("administrator pool and untrusted time rejected; commit context stops at actual session policy deadline")
}
