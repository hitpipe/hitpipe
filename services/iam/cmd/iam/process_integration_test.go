//go:build integration && linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/iamprocess"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	loginPath     = "/auth/login"
	callbackPath  = "/auth/callback"
	profilePath   = "/api/v1/profile"
	authorizePath = "/internal/v1/authorize"
	registerPath  = "/internal/v1/runtime/register"
	healthPath    = "/internal/v1/health/iam"
	childConfig   = "HIT138_PROCESS_CHILD_CONFIG"
	childOwner    = "HIT138_PROCESS_CHILD_OWNER"
)

type processChild struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func startChild(t *testing.T, exe string, args []string, environment ...string) *processChild {
	t.Helper()
	p := &processChild{cmd: exec.Command(exe, args...), done: make(chan struct{})}
	p.cmd.Env = append([]string{"TZ=UTC"}, environment...)
	p.cmd.Stdout, p.cmd.Stderr = io.Discard, io.Discard
	if err := p.cmd.Start(); err != nil {
		t.Fatal("start isolated IAM subprocess")
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		_ = p.cmd.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			t.Error("process cleanup timed out")
		}
	})
	return p
}

func (p *processChild) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-p.done:
		return p.err
	case <-time.After(8 * time.Second):
		t.Fatal("IAM subprocess did not exit within the test deadline")
		return nil
	}
}

func (p *processChild) terminate(t *testing.T) {
	t.Helper()
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal("send SIGTERM to the fixture process")
	}
	if err := p.wait(t); err != nil {
		t.Fatal("fixture process did not shut down successfully under SIGTERM")
	}
}

func processClient(t *testing.T, ca processCA, peer *processPeer) *http.Client {
	t.Helper()
	config := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.roots}
	if peer != nil {
		config.Certificates = []tls.Certificate{peer.tls}
	}
	transport := &http.Transport{TLSClientConfig: config, DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

type requestResult struct {
	code   int
	header http.Header
	body   []byte
	err    error
}

func processRequest(ctx context.Context, client *http.Client, addr, path, session string, payload any) requestResult {
	var input io.Reader
	method := http.MethodGet
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return requestResult{err: err}
		}
		input, method = bytes.NewReader(raw), http.MethodPost
	}
	request, err := http.NewRequestWithContext(ctx, method, "https://"+addr+path, input)
	if err != nil {
		return requestResult{err: err}
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", processID())
	}
	if session != "" {
		request.Header.Set("X-HitPipe-Session", session)
	}
	response, err := client.Do(request)
	if err != nil {
		return requestResult{err: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<10))
	return requestResult{code: response.StatusCode, header: response.Header, body: data, err: err}
}

func requireStatus(t *testing.T, result requestResult, statuses ...int) {
	t.Helper()
	for _, status := range statuses {
		if result.err == nil && result.code == status {
			return
		}
	}
	t.Fatalf("unexpected HTTP result: status=%d error=%t, expected one of %v", result.code, result.err != nil, statuses)
}

func rejectTransport(t *testing.T, result requestResult) {
	t.Helper()
	if result.err == nil {
		t.Fatalf("unapproved client completed private mTLS: status=%d", result.code)
	}
}

func healthSample(t *testing.T, r requestResult, revision, want string) {
	t.Helper()
	requireStatus(t, r, http.StatusOK)
	var h struct {
		Status, Service string
		BuildRevision   string    `json:"build_revision"`
		ProtocolVersion string    `json:"protocol_version"`
		CheckedAt       time.Time `json:"checked_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(r.body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&h) != nil || h.Status != want || h.Service != "IAM" || h.BuildRevision != revision ||
		h.ProtocolVersion != "1.0" || h.CheckedAt.IsZero() || decoder.Decode(new(any)) != io.EOF {
		t.Fatalf("unexpected technical health schema or non-business state; expected %q", want)
	}
}

func awaitHealth(t *testing.T, child *processChild, client *http.Client, revision, want string) {
	t.Helper()
	deadline, cancel := context.WithTimeout(t.Context(), 9*time.Second)
	defer cancel()
	for {
		answer := processRequest(deadline, client, opsAddr, healthPath, "", nil)
		var state struct{ Status string }
		if answer.err == nil && json.Unmarshal(answer.body, &state) == nil && state.Status == want {
			healthSample(t, answer, revision, want)
			return
		}
		select {
		case <-child.done:
			t.Fatal("IAM exited before exposing the expected technical state")
		case <-deadline.Done():
			t.Fatalf("did not observe expected health state %q", want)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func assertListenersGone(t *testing.T) {
	t.Helper()
	for _, address := range []string{businessAddr, opsAddr} {
		conn, err := net.DialTimeout("tcp4", address, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			t.Fatal("IAM listener survived subprocess exit")
		}
	}
}

func tracePool(t *testing.T, db *pgxpool.Pool, database string, active bool) {
	t.Helper()
	deadline, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		var count int
		err := db.QueryRow(deadline, `
 SELECT count(*) FROM pg_stat_activity
 WHERE datname=$1 AND application_name='hitpipe-iam' AND usename='hp_hit81_app'`, database).Scan(&count)
		if err == nil && active == (count > 0) {
			return
		}
		select {
		case <-deadline.Done():
			t.Fatalf("expected active app sessions=%t after changing process state", active)
		case <-time.After(35 * time.Millisecond):
		}
	}
}

func acceptFixturePeer(t *testing.T, db *pgxpool.Pool, peer runtimeaccess.Binding) {
	t.Helper()
	// Exercise the installation's real roster/readback. The independently
	// accepted BFF boot is a fixture prerequisite, not an IAM-granted permit.
	processSQL(t, db, `INSERT INTO hp_iam.runtime_registry
 (runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at)
 VALUES($1,$2,$3,$4,$5,'OPEN',clock_timestamp(),clock_timestamp())`,
		peer.RuntimeID, peer.Service, peer.Generation, peer.BootID, peer.CertificateFingerprint)
}

func platformRevision(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal("pin the integration executable to this exact source revision")
	}
	return strings.TrimSpace(string(out))
}

func TestPostgresIAMProcess(t *testing.T) {
	db, dsn, database := openProcessDB(t)
	revision := platformRevision(t)
	binary := filepath.Join(t.TempDir(), "iam")
	args := []string{"build", "-race"}
	if os.Getenv("HIT138_LOCAL_COMPOSITION_SMOKE") == "1" {
		args = append(args, "-buildvcs=false", "-ldflags", "-X main.buildRevision="+revision)
	}
	build := exec.CommandContext(t.Context(), "go", append(args, "-o", binary, ".")...)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual IAM executable: %s", out)
	}
	t.Run("normal executable and TLS role matrix", func(t *testing.T) {
		f := newProcessFixture(t, db)
		acceptFixturePeer(t, db, f.caller)
		conf := f.config(t, "process.json", dsn, revision, false)
		p := startChild(t, binary, []string{"-config", conf})
		ops, edge := processClient(t, f.ca, &f.ops), processClient(t, f.ca, &f.edge)
		bff, ai := processClient(t, f.ca, &f.bff), processClient(t, f.ca, &f.ai)
		noCert, unknown := processClient(t, f.ca, nil), processClient(t, f.ca, &f.unknown)
		foreign := processClient(t, f.ca, &f.foreign)
		awaitHealth(t, p, ops, revision, "CLOSED")
		rejectTransport(t, processRequest(t.Context(), noCert, opsAddr, healthPath, "", nil))
		rejectTransport(t, processRequest(t.Context(), foreign, opsAddr, healthPath, "", nil))
		rejectTransport(t, processRequest(t.Context(), bff, opsAddr, healthPath, "", nil))
		rejectTransport(t, processRequest(t.Context(), unknown, businessAddr, loginPath, "", nil))
		legacy := processClient(t, f.ca, &f.ops)
		legacyTLS := legacy.Transport.(*http.Transport).TLSClientConfig
		legacyTLS.MinVersion, legacyTLS.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
		rejectTransport(t, processRequest(t.Context(), legacy, opsAddr, healthPath, "", nil))
		rejectTransport(t, processRequest(t.Context(), noCert, businessAddr, loginPath, "", nil))
		requireStatus(t, processRequest(t.Context(), edge, businessAddr, healthPath, "", nil), http.StatusNotFound)
		requireStatus(t, processRequest(t.Context(), ops, opsAddr, profilePath, "", nil), http.StatusNotFound)
		requireStatus(t, processRequest(t.Context(), bff, businessAddr, loginPath, "", nil), http.StatusForbidden)
		requireStatus(t, processRequest(t.Context(), edge, businessAddr, profilePath, f.opaque, nil), http.StatusForbidden)
		requireStatus(t, processRequest(t.Context(), ai, businessAddr, profilePath, f.opaque, nil), http.StatusForbidden)
		requireStatus(t, processRequest(t.Context(), edge, businessAddr, registerPath, "", f.caller.Registration), http.StatusForbidden)
		requireStatus(t, processRequest(t.Context(), edge, businessAddr, authorizePath, f.opaque,
			authorizeaction.Request{WorkspaceID: f.workspace, RuntimeID: f.caller.RuntimeID, Action: "CASE_READ", ResourceID: &f.resource}), http.StatusForbidden)

		register := processRequest(t.Context(), ai, businessAddr, registerPath, "", f.agent.Registration)
		requireStatus(t, register, http.StatusOK)
		var snapshot runtimeaccess.Snapshot
		if json.Unmarshal(register.body, &snapshot) != nil || snapshot.RuntimeID != f.agent.RuntimeID || snapshot.BootID != f.agent.BootID || snapshot.Gate != "SYNCING" {
			t.Fatal("approved runtime registration did not return the exact closed boot snapshot")
		}
		awaitHealth(t, p, ops, revision, "LIVE")
		tracePool(t, db, database, true)
		requireStatus(t, processRequest(t.Context(), bff, businessAddr, profilePath, f.opaque, nil), http.StatusServiceUnavailable, http.StatusForbidden)
		t.Log("actual cmd/iam opened the registered service; TLS handshake and wrong-role/port isolation verified; missing authorities denied profile")
		p.terminate(t)
		assertListenersGone(t)
		tracePool(t, db, database, false)

		duplicate := startChild(t, binary, []string{"-config", conf})
		awaitHealth(t, duplicate, ops, revision, "CLOSED")
		requireStatus(t, processRequest(t.Context(), bff, businessAddr, profilePath, f.opaque, nil), http.StatusForbidden, http.StatusServiceUnavailable)
		duplicate.terminate(t)
		assertListenersGone(t)
		tracePool(t, db, database, false)
		// The previous process and its connections are gone. A test-only,
		// independently supplied fence establishes this boot's prerequisite.
		processSQL(t, db, `UPDATE hp_iam.runtime_registry SET status='FENCED',fence_evidence_ref=$2
 WHERE runtime_id=$1 AND boot_id=$3 AND status='OPEN'`, f.local.RuntimeID, processID(), f.local.BootID)
		f.iam = f.ca.leaf(t, f.iam.dns, false)
		f.local.BootID, f.local.Generation, f.local.CertificateFingerprint = processID(), 2, f.iam.fingerprint()
		fresh := startChild(t, binary, []string{"-config", f.config(t, "fresh.json", dsn, revision, false)})
		awaitHealth(t, fresh, ops, revision, "LIVE")
		var boot string
		var generation int64
		if err := db.QueryRow(t.Context(), `SELECT boot_id::text,generation FROM hp_iam.runtime_registry
 WHERE runtime_id=$1 AND status='OPEN'`, f.local.RuntimeID).Scan(&boot, &generation); err != nil || boot != f.local.BootID || generation != 2 {
			t.Fatal("the qualified fresh boot did not replace the fenced prior incarnation")
		}
		fresh.terminate(t)
		assertListenersGone(t)
		tracePool(t, db, database, false)
		t.Log("SIGTERM released sockets and app pool; duplicate old boot denied; exact new boot opened only after explicit durable fixture fence")
	})
	t.Run("same process composition with explicit fixture authorities", func(t *testing.T) {
		f := newProcessFixture(t, db)
		acceptFixturePeer(t, db, f.caller)
		acceptFixturePeer(t, db, f.agent)
		conf := f.config(t, "qualified.json", dsn, revision, false)
		exe, err := os.Executable()
		if err != nil {
			t.Fatal("find the integration subprocess executable")
		}
		owner, err := json.Marshal(processOwner{f.principal, f.session, f.workspace, f.resource, f.idp.issuer})
		if err != nil {
			t.Fatal("encode explicit test owner scope")
		}
		p := startChild(t, exe, []string{"-test.run", "^TestIAMProcessChild$"}, childConfig+"="+conf, childOwner+"="+string(owner))
		ops, edge := processClient(t, f.ca, &f.ops), processClient(t, f.ca, &f.edge)
		edge.Jar, err = cookiejar.New(nil)
		if err != nil {
			t.Fatal("create fixture browser cookie jar")
		}
		bff := processClient(t, f.ca, &f.bff)
		awaitHealth(t, p, ops, revision, "LIVE")
		login := processRequest(t.Context(), edge, businessAddr, loginPath, "", nil)
		requireStatus(t, login, http.StatusFound)
		location, err := url.Parse(login.header.Get("Location"))
		if err != nil || location.Scheme+"://"+location.Host+location.Path != f.idp.issuer+"/authorize" ||
			location.Query().Get("state") == "" || location.Query().Get("nonce") == "" || location.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("accepted login route lacked the configured real HTTPS OAuth redirect")
		}
		requireStatus(t, processRequest(t.Context(), edge, businessAddr, callbackPath, "", nil), http.StatusBadRequest)
		code := f.idp.issueTestCode(location.Query().Get("nonce"), location.Query().Get("code_challenge"))
		callbackReq := callbackPath + "?" + url.Values{"state": {location.Query().Get("state")}, "code": {code}}.Encode()
		callback := processRequest(t.Context(), edge, businessAddr, callbackReq, "", nil)
		requireStatus(t, callback, http.StatusSeeOther)
		if callback.header.Get("Location") != "/" {
			t.Fatal("successful callback did not redirect to the configured path")
		}
		var issued string
		for _, cookie := range (&http.Response{Header: callback.header}).Cookies() {
			if cookie.Name == "__Host-hp_session" && cookie.Secure && cookie.HttpOnly && cookie.Path == "/" &&
				cookie.SameSite == http.SameSiteLaxMode && cookie.Domain == "" {
				issued = cookie.Value
			}
		}
		digest := sha256.Sum256([]byte(issued))
		var created string
		var epoch int64
		if issued == "" || db.QueryRow(t.Context(), `SELECT session_id::text,epoch FROM hp_iam.sessions
 WHERE principal_id=$1 AND session_hash=$2`, f.principal, digest[:]).Scan(&created, &epoch) != nil ||
			created == f.session || epoch != 9 {
			t.Fatal("callback did not persist the exact principal, digest, and admitted epoch")
		}
		requireStatus(t, processRequest(t.Context(), edge, businessAddr, callbackReq, "", nil), http.StatusBadRequest)
		var count int
		if err := db.QueryRow(t.Context(), "SELECT count(*) FROM hp_iam.sessions WHERE principal_id=$1", f.principal).Scan(&count); err != nil || count != 2 {
			t.Fatal("callback replay created an additional persisted session")
		}
		profile := processRequest(t.Context(), bff, businessAddr, profilePath, f.opaque, nil)
		requireStatus(t, profile, http.StatusOK)
		var actual getprofile.Profile
		if json.Unmarshal(profile.body, &actual) != nil || actual.UserID != f.principal || actual.Revision != 4 || actual.DisplayName != "IAM process test" {
			t.Fatal("BFF profile did not return the current IAM session and owner file")
		}
		action := authorizeaction.Request{WorkspaceID: f.workspace, RuntimeID: f.caller.RuntimeID, Action: "CASE_READ", ResourceID: &f.resource}
		allowed := processRequest(t.Context(), bff, businessAddr, authorizePath, f.opaque, action)
		requireStatus(t, allowed, http.StatusOK)
		var decision authorizeaction.Authorization
		if json.Unmarshal(allowed.body, &decision) != nil || !decision.Allowed || decision.PrincipalID != f.principal ||
			decision.SessionID != f.session || decision.Epoch != 7 || decision.Role != "OWNER" {
			t.Fatal("authorize did not use the configured owner proof and current SQL access")
		}
		processSQL(t, db, `UPDATE hp_iam.sessions SET revoked_at=clock_timestamp() WHERE session_id=$1`, f.session)
		requireStatus(t, processRequest(t.Context(), bff, businessAddr, profilePath, f.opaque, nil), http.StatusForbidden, http.StatusUnauthorized)
		requireStatus(t, processRequest(t.Context(), bff, businessAddr, authorizePath, f.opaque, action), http.StatusForbidden, http.StatusUnauthorized)
		t.Log("EDGE login and callback through a controlled TLS IdP saved one exact session; BFF profile/authorize verified current access and rejected SQL session revoke")
		p.terminate(t)
		assertListenersGone(t)
		tracePool(t, db, database, false)
	})
	t.Run("invalid TLS materials and unavailable database", func(t *testing.T) {
		f := newProcessFixture(t, db)
		bad := f.config(t, "tls-invalid.json", dsn, revision, true)
		failed := startChild(t, binary, []string{"-config", bad})
		if err := failed.wait(t); err == nil {
			t.Fatal("invalid TLS configuration was silently accepted")
		}
		assertListenersGone(t)
		otherRevision := strings.Repeat("0", 40)
		if otherRevision == revision {
			otherRevision = strings.Repeat("1", 40)
		}
		mismatch := f.config(t, "build-mismatch.json", dsn, otherRevision, false)
		wrongBuild := startChild(t, binary, []string{"-config", mismatch})
		var badExit *exec.ExitError
		if err := wrongBuild.wait(t); !errors.As(err, &badExit) || badExit.ExitCode() != 2 {
			t.Fatal("valid TLS settings with a mismatched build revision did not fail closed")
		}
		assertListenersGone(t)
		broken, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("could not parse disposable test DSN")
		}
		// The loopback server is real PostgreSQL, but this random database
		// does not exist. This changes only the process-side configuration.
		broken.Path = "/unavailable-" + processID()
		offline := f.config(t, "db-unavailable.json", broken.String(), revision, false)
		p := startChild(t, binary, []string{"-config", offline})
		awaitHealth(t, p, processClient(t, f.ca, &f.ops), revision, "CLOSED")
		requireStatus(t, processRequest(t.Context(), processClient(t, f.ca, &f.iam), businessAddr, registerPath, "", f.local.Registration), http.StatusServiceUnavailable)
		p.terminate(t)
		assertListenersGone(t)
		t.Log("invalid TLS or build revision bound neither listener; failed real PostgreSQL connection exposed technical CLOSED and denied registration")
	})
}

type processClock struct{}

func (processClock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: ctx.Err() == nil}, ctx.Err()
}

type processOwner struct {
	Principal, Session, Workspace, Resource, Issuer string
}

func (o processOwner) Check(ctx context.Context, identity issuesession.Identity) (issuesession.GuardDecision, error) {
	if ctx.Err() != nil || identity.Issuer != o.Issuer || identity.Subject != o.Principal {
		return issuesession.GuardDecision{}, issuesession.ErrIssuanceDenied
	}
	return issuesession.GuardDecision{State: issuesession.IssuanceAllowed, Epoch: 9}, nil
}

func (o processOwner) Prove(ctx context.Context, actor authorizeaction.Actor, request authorizeaction.Request) (authorizeaction.Proof, error) {
	if ctx.Err() != nil || actor.PrincipalID != o.Principal || actor.SessionID != o.Session || actor.WorkspaceID != o.Workspace ||
		request.WorkspaceID != o.Workspace || request.Action != "CASE_READ" || request.ResourceID == nil || *request.ResourceID != o.Resource {
		return authorizeaction.Proof{}, authorizeaction.ErrDenied
	}
	return authorizeaction.Proof{Verified: true, PrincipalID: actor.PrincipalID, WorkspaceID: actor.WorkspaceID,
		ResourceID: request.ResourceID, Action: request.Action, Operation: "getCase"}, nil
}

func TestIAMProcessChild(t *testing.T) {
	path := os.Getenv(childConfig)
	if path == "" {
		t.Skip("exercised only by the separately launched integration child")
	}
	var owner processOwner
	if err := json.Unmarshal([]byte(os.Getenv(childOwner)), &owner); err != nil || owner.Principal == "" {
		t.Fatal("isolated child requires explicit test authority")
	}
	cfg, err := iamprocess.LoadConfig(path)
	if err != nil {
		t.Fatal("load same configuration as the real executable")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	pool, err := cfg.OpenPool(ctx)
	if err != nil {
		t.Fatal("open same real app pool as the executable")
	}
	defer pool.Close()
	err = iamprocess.RunWithAuthorities(ctx, cfg, pool, iamprocess.Authorities{Clock: processClock{}, Issuance: owner, Resource: owner})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal("independent test process composition failed")
	}
}
