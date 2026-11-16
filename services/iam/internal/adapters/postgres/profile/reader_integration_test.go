//go:build integration

package profile

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

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileDatabase struct {
	admin, app *pgxpool.Pool
	reader     *Reader
}

func profileDB(t *testing.T) profileDatabase {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_profile.py for isolated PostgreSQL 18.7 tests")
	}
	adminCfg, adminErr := pgxpool.ParseConfig(adminDSN)
	appCfg, appErr := pgxpool.ParseConfig(appDSN)
	if adminErr != nil || appErr != nil {
		t.Fatal("fixture configuration invalid; values withheld")
	}
	for _, cfg := range []*pgxpool.Config{adminCfg, appCfg} {
		if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") {
			t.Fatal("profile tests require isolated loopback hit68_test_ database")
		}
		cfg.MaxConns, cfg.MinConns = 5, 0
	}
	if adminCfg.ConnConfig.User != "postgres" || appCfg.ConnConfig.User != "hp_hit81_app" || appCfg.ConnConfig.Database != adminCfg.ConnConfig.Database || appCfg.ConnConfig.Port != adminCfg.ConnConfig.Port {
		t.Fatal("actual application and administrator must share isolated database")
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
	reader, err := New(app)
	if err != nil {
		t.Fatal(err)
	}
	return profileDatabase{admin: admin, app: app, reader: reader}
}
func profileUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("synthetic fixture randomness unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func profileDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func (db profileDatabase) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.admin.Exec(ctx, sql, args...); err != nil {
		state := "non-postgres"
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && len(postgresError.Code) == 5 {
			safe := true
			for _, c := range []byte(postgresError.Code) {
				if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
					safe = false
				}
			}
			if safe {
				state = postgresError.Code
			}
		}
		t.Fatalf("isolated profile fixture statement failed: SQLSTATE=%s statement_sha256=%x; values withheld", state, sha256.Sum256([]byte(sql)))
	}
}

type profileFixture struct {
	principal, foreignPrincipal, session, foreignSession, profileRef, foreignRef, digest, foreignDigest, runtime, boot, unknownWorkspace, raw, foreignRaw, reference, foreignReference string
	now                                                                                                                                                                                time.Time
	caller                                                                                                                                                                             authorizeaction.Caller
}

func (db profileDatabase) seed(t *testing.T) profileFixture {
	t.Helper()
	f := profileFixture{principal: profileUUID(t), foreignPrincipal: profileUUID(t), session: profileUUID(t), foreignSession: profileUUID(t), profileRef: profileUUID(t), foreignRef: profileUUID(t), runtime: profileUUID(t), boot: profileUUID(t), unknownWorkspace: profileUUID(t), now: time.Now().UTC().Truncate(time.Microsecond)}
	f.raw = "HIT85-SYNTHETIC-SESSION-" + profileUUID(t)
	f.foreignRaw = "HIT85-SYNTHETIC-OTHER-" + profileUUID(t)
	digest, foreign := sha256.Sum256([]byte(f.raw)), sha256.Sum256([]byte(f.foreignRaw))
	csrf := sha256.Sum256([]byte("HIT85-SYNTHETIC-CSRF-" + profileUUID(t)))
	f.reference, f.foreignReference = hex.EncodeToString(digest[:]), hex.EncodeToString(foreign[:])
	f.digest, f.foreignDigest = profileDigest("HIT85-SYNTHETIC-PROFILE-A"), profileDigest("HIT85-SYNTHETIC-PROFILE-B")
	for _, p := range []struct{ id, ref, digest string }{{f.principal, f.profileRef, f.digest}, {f.foreignPrincipal, f.foreignRef, f.foreignDigest}} {
		db.exec(t, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,profile_content_ref,profile_digest,created_at) VALUES($1::uuid,'https://issuer.invalid',$2::text,$3::uuid,$4::text,$5::timestamptz)`, p.id, p.id, p.ref, p.digest, f.now.Add(-time.Minute))
	}
	for _, s := range []struct {
		id, principal string
		hash          [32]byte
	}{{f.session, f.principal, digest}, {f.foreignSession, f.foreignPrincipal, foreign}} {
		db.exec(t, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch) VALUES($1::uuid,$2::bytea,$3::uuid,$4::bytea,$5::timestamptz,$6::timestamptz,$7::timestamptz,3)`, s.id, s.hash[:], s.principal, csrf[:], f.now.Add(-time.Minute), f.now.Add(time.Hour), f.now.Add(30*time.Minute))
	}
	f.caller = authorizeaction.Caller{Service: "BFF", RuntimeID: f.runtime, BootID: f.boot, Generation: 7, CertificateFingerprint: "sha256:" + strings.Repeat("a", 64)}
	db.exec(t, `INSERT INTO hp_iam.runtime_registry(runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at) VALUES($1::uuid,'BFF',7,$2::uuid,$3::text,'OPEN',$4::timestamptz,$4::timestamptz)`, f.runtime, f.boot, f.caller.CertificateFingerprint, f.now)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, s := range []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM hp_iam.revocations WHERE subject_id IN($1,$2) OR workspace_id=$3`, []any{f.session, f.foreignSession, f.unknownWorkspace}},
			{`DELETE FROM hp_iam.sessions WHERE principal_id IN($1,$2)`, []any{f.principal, f.foreignPrincipal}},
			{`DELETE FROM hp_iam.principals WHERE principal_id IN($1,$2)`, []any{f.principal, f.foreignPrincipal}},
			{`DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1`, []any{f.runtime}},
		} {
			if _, err := db.admin.Exec(ctx, s.sql, s.args...); err != nil {
				t.Error("isolated profile fixture cleanup failed")
			}
		}
	})
	return f
}
func (db profileDatabase) read(t *testing.T, f profileFixture) getprofile.Snapshot {
	t.Helper()
	got, err := db.reader.Read(context.Background(), f.reference, f.caller)
	if err != nil {
		t.Fatalf("fresh profile projection failed with bounded error: %v", err)
	}
	return got
}

func TestPostgresProfileGlobalSessionOwnerProjection(t *testing.T) {
	db := profileDB(t)
	f := db.seed(t)
	got := db.read(t, f)
	if !got.Session.Found || !got.Session.Policy.Found || got.Session.ID != f.session || got.Session.PrincipalID != f.principal || got.Session.Epoch != 3 || got.Session.Policy.Revoked || !got.Principal.Found || got.Principal.ID != f.principal || got.Principal.Disabled || got.ProfileRef != f.profileRef || got.ProfileDigest != f.digest || !got.Runtime.Found || got.Runtime.Caller != f.caller || got.Runtime.Status != "OPEN" || len(got.Revocations) != 0 {
		t.Fatal("global own-profile authority projection mismatch")
	}
	ref, err := sessionlookup.Reference(f.raw)
	if err != nil || ref != f.reference {
		t.Fatal("existing opaque-session reference was replaced")
	}
	// No workspace or membership rows exist for either actor. A browser/GUC value
	// cannot select the other principal's global profile, or manufacture a grant.
	cfg := db.app.Config()
	cfg.MaxConns = 1
	single, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("single-connection fixture unavailable")
	}
	defer single.Close()
	reader, _ := New(single)
	conn, err := single.Acquire(context.Background())
	if err != nil {
		t.Fatal("fixture GUC connection unavailable")
	}
	if _, err := conn.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,false)`, f.unknownWorkspace); err != nil {
		conn.Release()
		t.Fatal("fixture GUC unavailable")
	}
	conn.Release()
	own, err := reader.Read(context.Background(), f.reference, f.caller)
	if err != nil || own.Principal.ID != f.principal || own.ProfileRef != f.profileRef {
		t.Fatal("workspace GUC changed own global identity")
	}
	other, err := reader.Read(context.Background(), f.foreignReference, f.caller)
	if err != nil || other.Principal.ID != f.foreignPrincipal || other.ProfileRef != f.foreignRef || other.ProfileRef == own.ProfileRef {
		t.Fatal("different saved session did not select its own exact principal")
	}
	unknown, err := reader.Read(context.Background(), strings.Repeat("f", 64), f.caller)
	if err != nil || !reflect.DeepEqual(unknown, getprofile.Snapshot{}) {
		t.Fatal("unknown session exposed profile metadata")
	}
	upper := f.caller
	upper.RuntimeID = strings.ToUpper(upper.RuntimeID)
	if actual, err := reader.Read(context.Background(), f.reference, upper); err != nil || actual.Runtime.Caller.RuntimeID != f.runtime {
		t.Fatal("valid UUID letter case changed runtime identity")
	}
}

func TestPostgresProfileFreshMetadataAndNullableContent(t *testing.T) {
	db := profileDB(t)
	f := db.seed(t)
	_ = db.read(t, f)
	nextRef, nextDigest := profileUUID(t), profileDigest("HIT85-SYNTHETIC-NEXT-PROFILE")
	db.exec(t, `UPDATE hp_iam.principals SET profile_content_ref=$2::uuid,profile_digest=$3::text WHERE principal_id=$1::uuid`, f.principal, nextRef, nextDigest)
	got := db.read(t, f)
	if got.ProfileRef != nextRef || got.ProfileDigest != nextDigest {
		t.Fatal("committed profile pointer/digest remained cached")
	}
	db.exec(t, `UPDATE hp_iam.principals SET profile_content_ref=NULL,profile_digest=NULL WHERE principal_id=$1::uuid`, f.principal)
	got = db.read(t, f)
	if !got.Principal.Found || got.ProfileRef != "" || got.ProfileDigest != "" {
		t.Fatal("missing profile link was replaced by defaults")
	}
	db.exec(t, `UPDATE hp_iam.principals SET profile_content_ref=$2::uuid WHERE principal_id=$1::uuid`, f.principal, nextRef)
	got = db.read(t, f)
	if got.ProfileRef != nextRef || got.ProfileDigest != "" {
		t.Fatal("half-missing profile metadata became complete")
	}
	db.exec(t, `UPDATE hp_iam.principals SET profile_content_ref=NULL,profile_digest='malformed-synthetic-digest' WHERE principal_id=$1::uuid`, f.principal)
	got = db.read(t, f)
	if got.ProfileRef != "" || got.ProfileDigest != "malformed-synthetic-digest" {
		t.Fatal("malformed stored digest was silently normalized")
	}
	db.exec(t, `UPDATE hp_iam.principals SET disabled_at=$2::timestamptz WHERE principal_id=$1::uuid`, f.principal, f.now)
	db.exec(t, `UPDATE hp_iam.sessions SET epoch=4,revoked_at=$2::timestamptz WHERE session_id=$1::uuid`, f.session, f.now)
	db.exec(t, `UPDATE hp_iam.runtime_registry SET generation=8,status='FENCED',boot_id=$2::uuid WHERE runtime_id=$1::uuid`, f.runtime, profileUUID(t))
	got = db.read(t, f)
	if !got.Principal.Disabled || !got.Session.Policy.Revoked || got.Session.Epoch != 4 || got.Runtime.Caller.Generation != 8 || got.Runtime.Status != "FENCED" || got.Runtime.Caller.BootID == f.boot {
		t.Fatal("committed deny/session epoch/runtime facts remained stale")
	}
}

func TestPostgresProfileExactSessionRevocations(t *testing.T) {
	db := profileDB(t)
	f := db.seed(t)
	for n, status := range []string{"PENDING", "INCIDENT", "EFFECTIVE"} {
		var workspace, effective any
		if n > 0 {
			workspace = f.unknownWorkspace
		}
		if status == "EFFECTIVE" {
			effective = f.now
		}
		db.exec(t, `INSERT INTO hp_iam.revocations(workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,effective_at,reason_code) VALUES($1::uuid,$2::uuid,'SESSION',$3::uuid,$4::bigint,$5::timestamptz,$5::timestamptz+interval'5seconds',$6::text,$7::timestamptz,'SYNTHETIC')`, workspace, profileUUID(t), f.session, int64(n+4), f.now, status, effective)
	}
	db.exec(t, `INSERT INTO hp_iam.revocations(workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code) VALUES(NULL,$1::uuid,'SESSION',$2::uuid,99,$3::timestamptz,$3::timestamptz+interval'5seconds','PENDING','SYNTHETIC')`, profileUUID(t), f.foreignSession, f.now)
	db.exec(t, `INSERT INTO hp_iam.revocations(workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code) VALUES($1::uuid,$2::uuid,'WORKSPACE',$1::uuid,100,$3::timestamptz,$3::timestamptz+interval'5seconds','PENDING','SYNTHETIC')`, f.unknownWorkspace, profileUUID(t), f.now)
	got := db.read(t, f)
	if len(got.Revocations) != 3 || got.Session.Epoch != 3 {
		t.Fatal("session denies were lost, merged or replaced by workspace authority")
	}
	for n, r := range got.Revocations {
		if r.Scope != "SESSION" || r.SubjectID != f.session || r.Epoch != int64(n+4) {
			t.Fatal("exact session deny projection mismatch")
		}
		if n == 0 && r.WorkspaceID != "" || n > 0 && r.WorkspaceID != f.unknownWorkspace {
			t.Fatal("unknown workspace relation was hidden")
		}
	}
}

func TestPostgresProfileConcurrentConsistentSnapshots(t *testing.T) {
	db := profileDB(t)
	f := db.seed(t)
	nextRef, nextDigest := profileUUID(t), profileDigest("HIT85-SYNTHETIC-CONCURRENT")
	const calls = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan getprofile.Snapshot, calls)
	errs := make(chan error, calls)
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, err := db.reader.Read(context.Background(), f.reference, f.caller)
			results <- s
			errs <- err
		}()
	}
	close(start)
	tx, err := db.admin.Begin(context.Background())
	if err != nil {
		t.Fatal("fixture change transaction unavailable")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `UPDATE hp_iam.principals SET profile_content_ref=$2::uuid,profile_digest=$3::text WHERE principal_id=$1::uuid`, f.principal, nextRef, nextDigest); err != nil {
		t.Fatal("fixture metadata change unavailable")
	}
	if _, err := tx.Exec(context.Background(), `UPDATE hp_iam.sessions SET epoch=4 WHERE session_id=$1::uuid`, f.session); err != nil {
		t.Fatal("fixture epoch change unavailable")
	}
	if _, err := tx.Exec(context.Background(), `UPDATE hp_iam.runtime_registry SET generation=8 WHERE runtime_id=$1::uuid`, f.runtime); err != nil {
		t.Fatal("fixture runtime change unavailable")
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal("fixture change commit unavailable")
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent projection failed with bounded error: %v", err)
		}
	}
	for got := range results {
		before := got.ProfileRef == f.profileRef && got.ProfileDigest == f.digest && got.Session.Epoch == 3 && got.Runtime.Caller.Generation == 7
		after := got.ProfileRef == nextRef && got.ProfileDigest == nextDigest && got.Session.Epoch == 4 && got.Runtime.Caller.Generation == 8
		if !before && !after {
			t.Fatal("profile/session/runtime facts mixed across committed snapshots")
		}
	}
	latest := db.read(t, f)
	if latest.ProfileRef != nextRef || latest.ProfileDigest != nextDigest || latest.Session.Epoch != 4 || latest.Runtime.Caller.Generation != 8 {
		t.Fatal("next request did not observe committed authority")
	}
}

func TestPostgresProfileRoleErrorsCancellationAndDatabaseFailure(t *testing.T) {
	db := profileDB(t)
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
			if got, err := reader.Read(context.Background(), f.reference, f.caller); !errors.Is(err, ErrConfiguration) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
				t.Fatal("forbidden application authority returned profile data")
			}
		})
	}
	cfg := db.app.Config()
	cfg.MaxConns = 1
	limited, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("bounded fixture pool unavailable")
	}
	defer limited.Close()
	reader, _ := New(limited)
	held, err := limited.Acquire(context.Background())
	if err != nil {
		t.Fatal("pool saturation fixture unavailable")
	}
	for _, timeout := range []time.Duration{20 * time.Millisecond, 5 * time.Second} {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		started := time.Now()
		got, err := reader.Read(ctx, f.reference, f.caller)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || !reflect.DeepEqual(got, getprofile.Snapshot{}) || time.Since(started) > 2500*time.Millisecond {
			held.Release()
			t.Fatal("pool wait was not bounded and fail closed")
		}
	}
	held.Release()
	if actual, err := reader.Read(context.Background(), f.reference, f.caller); err != nil || actual.ProfileRef != f.profileRef {
		t.Fatal("canceled read closed the caller's healthy pool")
	}
	closed, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("closed pool fixture unavailable")
	}
	closedReader, _ := New(closed)
	closed.Close()
	if got, err := closedReader.Read(context.Background(), f.reference, f.caller); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
		t.Fatal("database failure returned profile data")
	}
}
