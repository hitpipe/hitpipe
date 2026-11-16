//go:build integration

package profile

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/content/profilefiles"
	pgprofile "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/profile"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

type httpsClock struct{}

func (httpsClock) Now(context.Context) (sessionpolicy.Observation, error) {
	// A controlled test observation, not a production clock-health assertion.
	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: true}, nil
}

type httpsContent func(context.Context, string, string) (getprofile.Profile, error)

func (f httpsContent) Read(ctx context.Context, ref, digest string) (getprofile.Profile, error) {
	return f(ctx, ref, digest)
}

// The gate is explicitly a controlled, single-test authority. Its exact current
// session binding is checked; it implements no production barrier/coordinator.
type httpsGate struct {
	caller                          authorizeaction.Caller
	principal, session, ref, digest string
	epoch                           int64
	closed                          bool
}
type httpsAdmission struct {
	binding getprofile.Binding
	gate    httpsGate
}

func (a httpsAdmission) Binding() getprofile.Binding { return a.binding }
func (a httpsAdmission) Confirm(ctx context.Context, facts getprofile.Facts) error {
	if ctx.Err() != nil || facts.Caller != a.gate.caller || facts.PrincipalID != a.gate.principal || facts.SessionID != a.gate.session || facts.ProfileRef != a.gate.ref || facts.ProfileDigest != a.gate.digest || facts.SessionEpoch != a.gate.epoch || len(facts.Revocations) != 0 {
		return errors.New("synthetic gate rejected current facts")
	}
	return nil
}
func (g httpsGate) Within(ctx context.Context, binding getprofile.Binding, callback func(context.Context, getprofile.Admission) error) error {
	if g.closed || ctx.Err() != nil || binding.Caller != g.caller {
		return errors.New("synthetic gate closed")
	}
	return callback(ctx, httpsAdmission{binding: binding, gate: g})
}

type httpsFixture struct {
	admin, app                                    *pgxpool.Pool
	pki                                           profilePKI
	profile                                       getprofile.Profile
	principal, session, ref, digest, secret, root string
	epoch                                         int64
}

func httpsUUID(t *testing.T) string {
	t.Helper()
	id, err := requestID()
	if err != nil {
		t.Fatal("synthetic ID generation failed")
	}
	return id
}

func (f httpsFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := f.admin.Exec(ctx, query, args...); err != nil {
		t.Fatal("isolated HTTPS fixture SQL failed; values withheld")
	}
}

func newHTTPSFixture(t *testing.T) httpsFixture {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_profile.py for isolated real HTTPS/PostgreSQL/content checks")
	}
	adminCfg, aerr := pgxpool.ParseConfig(adminDSN)
	appCfg, perr := pgxpool.ParseConfig(appDSN)
	if aerr != nil || perr != nil {
		t.Fatal("invalid isolated fixture configuration; values withheld")
	}
	for _, cfg := range []*pgxpool.Config{adminCfg, appCfg} {
		if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") {
			t.Fatal("HTTPS integration requires controlled loopback test database")
		}
		cfg.MinConns, cfg.MaxConns = 0, 5
	}
	if adminCfg.ConnConfig.User != "postgres" || appCfg.ConnConfig.User != "hp_hit81_app" || adminCfg.ConnConfig.Database != appCfg.ConnConfig.Database || adminCfg.ConnConfig.Port != appCfg.ConnConfig.Port {
		t.Fatal("HTTPS fixture requires actual isolated IAM application login")
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), adminCfg)
	if err != nil {
		t.Fatal("fixture administrator unavailable")
	}
	t.Cleanup(admin.Close)
	app, err := pgxpool.NewWithConfig(context.Background(), appCfg)
	if err != nil {
		t.Fatal("fixture IAM application unavailable")
	}
	t.Cleanup(app.Close)
	pki := testPKI(t)
	pki.caller.RuntimeID, pki.caller.BootID = httpsUUID(t), httpsUUID(t)
	f := httpsFixture{admin: admin, app: app, pki: pki, principal: httpsUUID(t), session: httpsUUID(t), ref: httpsUUID(t), root: t.TempDir(), epoch: 5}
	f.secret = "HIT85-SYNTHETIC-HTTPS-SESSION-" + httpsUUID(t)
	f.profile = getprofile.Profile{UserID: f.principal, DisplayName: "Синтетический владелец профиля", Timezone: "Europe/Moscow", Revision: 9}
	if err := os.Chmod(f.root, 0700); err != nil {
		t.Fatal("fixture private directory unavailable")
	}
	body, err := json.Marshal(f.profile)
	if err != nil {
		t.Fatal("fixture profile serialization failed")
	}
	digest := sha256.Sum256(body)
	f.digest = "sha256:" + hex.EncodeToString(digest[:])
	file, err := os.OpenFile(filepath.Join(f.root, "pending"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("fixture private file unavailable")
	}
	if _, err = file.Write(body); err != nil {
		file.Close()
		t.Fatal("fixture file write failed")
	}
	if err = file.Sync(); err != nil {
		file.Close()
		t.Fatal("fixture file sync failed")
	}
	if err = file.Close(); err != nil {
		t.Fatal("fixture file close failed")
	}
	if err = os.Rename(filepath.Join(f.root, "pending"), filepath.Join(f.root, f.ref+".json")); err != nil {
		t.Fatal("fixture publication rename failed")
	}
	directory, err := os.Open(f.root)
	if err != nil {
		t.Fatal("fixture directory open failed")
	}
	if err = directory.Sync(); err != nil {
		directory.Close()
		t.Fatal("fixture directory sync failed")
	}
	directory.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	f.exec(t, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,profile_content_ref,profile_digest,created_at) VALUES($1,$2,$3,$4,$5,$6)`, f.principal, "https://identity.invalid/hit85", f.principal, f.ref, f.digest, now.Add(-time.Minute))
	sessionHash := sha256.Sum256([]byte(f.secret))
	csrfHash := sha256.Sum256([]byte("HIT85-SYNTHETIC-CSRF-" + f.principal))
	f.exec(t, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, f.session, sessionHash[:], f.principal, csrfHash[:], now.Add(-time.Minute), now.Add(time.Hour), now.Add(30*time.Minute), f.epoch)
	f.exec(t, `INSERT INTO hp_iam.runtime_registry(runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at) VALUES($1,'BFF',$2,$3,$4,'OPEN',$5,$5)`, pki.caller.RuntimeID, pki.caller.Generation, pki.caller.BootID, pki.caller.CertificateFingerprint, now)
	t.Cleanup(func() {
		f.exec(t, `DELETE FROM hp_iam.revocations WHERE scope='SESSION' AND subject_id=$1`, f.session)
		f.exec(t, `DELETE FROM hp_iam.sessions WHERE session_id=$1`, f.session)
		f.exec(t, `DELETE FROM hp_iam.principals WHERE principal_id=$1`, f.principal)
		f.exec(t, `DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1`, f.pki.caller.RuntimeID)
	})
	return f
}

func TestProfileHTTPSPostgresAndOwnerFiles(t *testing.T) {
	for _, name := range []string{"global profile without workspace", "forged browser claims", "wrong session", "revoked session", "disabled principal", "closed runtime", "missing content", "mismatched digest", "closed gate", "epoch changes during content read", "database unavailable"} {
		t.Run(name, func(t *testing.T) {
			f := newHTTPSFixture(t)
			reader, err := pgprofile.New(f.app)
			if err != nil {
				t.Fatal("fixture SQL reader configuration failed")
			}
			lookup, err := sessionlookup.New(f.app)
			if err != nil {
				t.Fatal("fixture session reader configuration failed")
			}
			files, err := profilefiles.New(f.root)
			if err != nil {
				t.Fatal("fixture content reader configuration failed")
			}
			t.Cleanup(func() { _ = files.Close() })
			var content getprofile.Content = files
			gate := httpsGate{caller: f.pki.caller, principal: f.principal, session: f.session, ref: f.ref, digest: f.digest, epoch: f.epoch}
			secret, status := f.secret, http.StatusOK
			switch name {
			case "wrong session":
				secret = strings.Repeat("x", 43)
				status = 401
			case "revoked session":
				f.exec(t, `UPDATE hp_iam.sessions SET revoked_at=clock_timestamp() WHERE session_id=$1`, f.session)
				status = 401
			case "disabled principal":
				f.exec(t, `UPDATE hp_iam.principals SET disabled_at=clock_timestamp() WHERE principal_id=$1`, f.principal)
				status = 401
			case "closed runtime":
				f.exec(t, `UPDATE hp_iam.runtime_registry SET status='CLOSED' WHERE runtime_id=$1`, f.pki.caller.RuntimeID)
				status = 403
			case "missing content":
				if err := os.Remove(filepath.Join(f.root, f.ref+".json")); err != nil {
					t.Fatal("fixture content removal failed")
				}
				status = 503
			case "mismatched digest":
				f.exec(t, `UPDATE hp_iam.principals SET profile_digest=$2 WHERE principal_id=$1`, f.principal, "sha256:"+strings.Repeat("f", 64))
				status = 503
			case "closed gate":
				gate.closed = true
				status = 503
			case "epoch changes during content read":
				content = httpsContent(func(ctx context.Context, ref, digest string) (getprofile.Profile, error) {
					p, e := files.Read(ctx, ref, digest)
					f.exec(t, `UPDATE hp_iam.sessions SET epoch=epoch+1 WHERE session_id=$1`, f.session)
					return p, e
				})
				status = 503
			case "database unavailable":
				f.app.Close()
				status = 503
			}
			service, err := getprofile.New(lookup, authorizeaction.ReferenceFunc(sessionlookup.Reference), httpsClock{}, reader, content, gate)
			if err != nil {
				t.Fatal("fixture profile composition configuration failed")
			}
			server := testServer(t, f.pki, service)
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/profile", nil)
			request.Header.Set("X-HitPipe-Session", secret)
			// No workspace or membership exists for this global identity. Forged
			// browser claims cannot choose a different user's profile or role.
			request.Header.Set("X-HitPipe-User", httpsUUID(t))
			request.Header.Set("X-HitPipe-Role", "OWNER")
			request.Header.Set("X-HitPipe-Workspace", httpsUUID(t))
			response, err := testClient(t, f.pki, []tls.Certificate{f.pki.bff}, tls.VersionTLS13).Do(request)
			if err != nil {
				t.Fatal("controlled real HTTPS request failed")
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected bounded HTTPS outcome: status=%d want=%d", response.StatusCode, status)
			}
			if strings.Contains(string(body), secret) || strings.Contains(string(body), f.digest) || strings.Contains(string(body), f.root) {
				t.Fatal("private credential or storage locator entered HTTP response")
			}
			if status == 200 {
				var actual getprofile.Profile
				if json.Unmarshal(body, &actual) != nil || actual != f.profile {
					t.Fatal("real database and owner content did not yield the session owner's exact profile")
				}
			} else {
				var actual errorResponse
				if json.Unmarshal(body, &actual) != nil || !validUUID(actual.RequestID) || actual.Code == "" || strings.Contains(string(body), f.profile.DisplayName) {
					t.Fatal("failure disclosed profile or malformed the accepted error")
				}
			}
		})
	}
}
