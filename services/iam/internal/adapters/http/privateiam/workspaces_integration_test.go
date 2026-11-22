//go:build integration

package privateiam_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wired "github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/content/workspacefiles"
	workspaceHTTP "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/workspaces"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixtureWorkspaceCandidates struct {
	principal string
	mu        sync.Mutex
	ids       []string
}

func (f *fixtureWorkspaceCandidates) WorkspaceIDs(ctx context.Context, principal string) ([]string, error) {
	if ctx.Err() != nil || principal != f.principal {
		return nil, app.ErrDenied
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ids), nil
}

func (f *fixtureWorkspaceCandidates) replace(ids []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = slices.Clone(ids)
}

type fixtureOnboarding map[string]app.Grant

func (f fixtureOnboarding) WithGrant(ctx context.Context, principal, request string, use func(context.Context, app.Grant) error) error {
	grant, found := f[request]
	if ctx.Err() != nil || !found || principal != grant.PrincipalID {
		return app.ErrDenied
	}
	return use(ctx, grant)
}

type discoveryRollback struct{}

// The probe observes the existing SQL transaction lifecycle and pauses only
// after the first workspace policy read has released its SQL locks. It never
// rewrites a statement, records parameters or replaces the application pool.
type discoveryProbe struct {
	armed         atomic.Bool
	scoped        sync.Map
	first, resume chan struct{}
}

func (p *discoveryProbe) TraceQueryStart(ctx context.Context, conn *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if p.armed.Load() && strings.Contains(query.SQL, "FROM hp_iam.workspaces w JOIN hp_iam.memberships m") {
		p.scoped.Store(conn, struct{}{})
	}
	if query.SQL == "rollback" {
		if _, scoped := p.scoped.LoadAndDelete(conn); scoped {
			return context.WithValue(ctx, discoveryRollback{}, true)
		}
	}
	return ctx
}

func (p *discoveryProbe) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, result pgx.TraceQueryEndData) {
	if result.Err == nil && ctx.Value(discoveryRollback{}) == true && p.armed.CompareAndSwap(true, false) {
		close(p.first)
		select {
		case <-p.resume:
		case <-ctx.Done():
		}
	}
}

type responseLossProbe struct {
	app.Content
	workspace string
	armed     atomic.Bool
	entered   chan struct{}
}

func (p *responseLossProbe) Read(ctx context.Context, workspace, ref string) (string, error) {
	if workspace == p.workspace && p.armed.CompareAndSwap(true, false) {
		// Create has already committed when it reads the owner's projected
		// name. Wait for the client to disconnect before this response starts.
		close(p.entered)
		<-ctx.Done()
		return "", ctx.Err()
	}
	return p.Content.Read(ctx, workspace, ref)
}

func startWorkspacesFixture(t *testing.T, service *wired.Server, peers labPeers, iam *http.Client,
	files app.Content, candidates app.Candidates, grants app.Onboarding, expectedGate string) (string, func()) {
	t.Helper()
	const address = "127.0.0.1:8443"
	probe, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatal("isolated IAM port is already in use")
	}
	_ = probe.Close()
	listenContext, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	var listenError error
	go func() {
		defer close(stopped)
		listenError = service.ListenWorkspaces(listenContext, address, peers.server, peers.ca.roots, files, candidates, grants)
	}()
	stop := sync.OnceFunc(func() {
		cancel()
		select {
		case <-stopped:
			if listenError != nil {
				t.Error("workspaces listener failed")
			}
		case <-time.After(5 * time.Second):
			t.Error("workspaces listener remained active")
		}
	})
	t.Cleanup(stop)
	base := "https://" + address
	ready, stopWaiting := context.WithTimeout(t.Context(), 4*time.Second)
	defer stopWaiting()
	for {
		registration := post(ready, iam, base+registerPath, peers.local.Registration, "")
		if registration.err == nil {
			if snapshot := assertRegistration(t, registration, peers.local); snapshot.Gate != expectedGate {
				t.Fatal("listener changed the established runtime gate during bootstrap")
			}
			return base, stop
		}
		select {
		case <-stopped:
			t.Fatal("workspaces listener exited during bootstrap")
		case <-ready.Done():
			t.Fatal("the private listener did not accept its verified mTLS bootstrap")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

type workspacePayload struct {
	Command struct {
		RequestID string `json:"request_id"`
	} `json:"command"`
	Name string `json:"name"`
}

func workspaceBody(t *testing.T, id, name string) []byte {
	t.Helper()
	payload := workspacePayload{Name: name}
	payload.Command.RequestID = id
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal("could not encode the synthetic workspace command")
	}
	return body
}

func callWorkspaces(ctx context.Context, client *http.Client, base, method, session, csrf, id string, body []byte, extra http.Header) result {
	request, err := http.NewRequestWithContext(ctx, method, base+workspaceHTTP.Path, bytes.NewReader(body))
	if err != nil {
		return result{err: err}
	}
	for key, values := range extra {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	if session != "" {
		request.Header.Set("X-HitPipe-Session", session)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	if id != "" {
		request.Header.Set("X-Request-ID", id)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return result{err: err}
	}
	defer response.Body.Close()
	if response.TLS == nil || response.TLS.Version != tls.VersionTLS13 || len(response.TLS.VerifiedChains) == 0 ||
		response.Header.Get("Cache-Control") != "no-store" {
		return result{status: response.StatusCode, err: errors.New("workspaces test response did not use the verified private transport")}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return result{status: response.StatusCode, body: data, err: err}
}

func seedWorkspaceName(t *testing.T, db labDatabase, files *workspacefiles.Store, id, syntheticName string) {
	t.Helper()
	var ref string
	if err := db.admin.QueryRow(t.Context(), `SELECT name_content_ref::text FROM hp_iam.workspaces WHERE workspace_id=$1`, id).Scan(&ref); err != nil {
		t.Fatal("could not read the fixture's workspace content reference")
	}
	body, err := json.Marshal(struct {
		Schema    string `json:"schema"`
		Workspace string `json:"workspace_id"`
		Name      string `json:"name"`
	}{"hitpipe.workspace.name.v1", id, syntheticName})
	if err != nil || files.Put(t.Context(), id, ref, string(body)) != nil {
		t.Fatal("could not durably seed the fixture workspace name")
	}
}

func cleanWorkspaceFixture(t *testing.T, db labDatabase, workspace string) {
	t.Helper()
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM hp_iam.outbox WHERE workspace_id=$1`,
			`DELETE FROM hp_iam.audit_minimal WHERE workspace_id=$1`,
			`DELETE FROM hp_iam.command_receipts WHERE workspace_id=$1`,
			`DELETE FROM hp_iam.memberships WHERE workspace_id=$1`,
			`DELETE FROM hp_iam.workspaces WHERE workspace_id=$1`,
		} {
			sqlExec(t, db.admin, query, workspace)
		}
	})
}

func checkWorkspaceWrite(t *testing.T, db labDatabase, files *workspacefiles.Store, actor string, grant app.Grant, syntheticName string) {
	t.Helper()
	// The actual application pool cannot read the new tenant outside the GUC.
	var unscoped int
	if err := db.app.QueryRow(t.Context(), `SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id=$1`, grant.WorkspaceID).Scan(&unscoped); err != nil || unscoped != 0 {
		t.Fatal("the application role bypassed tenant RLS")
	}
	tx, err := db.app.Begin(t.Context())
	if err != nil {
		t.Fatal("could not read back through the application role")
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `SELECT set_config('hp.workspace_id',$1,true)`, grant.WorkspaceID); err != nil {
		t.Fatal("could not select the exact workspace for RLS readback")
	}
	var rowCount, memberCount, receiptCount, auditCount, messageCount int
	const cardinality = `SELECT (SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.memberships WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.command_receipts WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.audit_minimal WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_iam.outbox WHERE workspace_id=$1)`
	if err := tx.QueryRow(t.Context(), cardinality, grant.WorkspaceID).Scan(&rowCount, &memberCount, &receiptCount, &auditCount, &messageCount); err != nil ||
		rowCount != 1 || memberCount != 1 || receiptCount != 1 || auditCount != 1 || messageCount != 1 {
		t.Fatal("durable create and repeat did not leave exactly one of each accepted record")
	}
	var nameRef, messageRef, messageDigest string
	var exact, private bool
	const joined = `SELECT w.name_content_ref::text,o.payload_content_ref::text,o.digest,
 (w.revision=1 AND w.security_epoch=1 AND w.lifecycle='ACTIVE' AND w.owner_confirmed_at=$4 AND w.initial_activation_at IS NULL
  AND m.principal_id=$2 AND m.role='OWNER' AND m.status='ACTIVE' AND m.revision=1
  AND c.actor_id=$2 AND c.request_id=$3 AND c.result_id=w.workspace_id
  AND a.actor_id=$2 AND a.resource_id=w.workspace_id AND a.action_code=$5 AND a.reason_code=$6
  AND o.kind=$7 AND o.state='PENDING' AND o.payload_expires_at=$8),
 strpos(concat(row_to_json(w),row_to_json(m),row_to_json(c),row_to_json(a),row_to_json(o)),$9)=0
 FROM hp_iam.workspaces w JOIN hp_iam.memberships m USING(workspace_id)
 JOIN hp_iam.command_receipts c USING(workspace_id) JOIN hp_iam.audit_minimal a USING(workspace_id) JOIN hp_iam.outbox o USING(workspace_id)
 WHERE w.workspace_id=$1`
	if err := tx.QueryRow(t.Context(), joined, grant.WorkspaceID, actor, grant.RequestID, grant.OwnerConfirmedAt,
		grant.AuditAction, grant.AuditReason, grant.MessageKind, grant.PayloadExpiresAt, syntheticName).Scan(&nameRef, &messageRef, &messageDigest, &exact, &private); err != nil || !exact || !private {
		t.Fatal("committed rows did not contain the accepted scoped metadata or exposed free text")
	}
	stored, err := files.Read(t.Context(), grant.WorkspaceID, nameRef)
	if err != nil || !strings.Contains(stored, syntheticName) {
		t.Fatal("the exact committed name reference did not resolve to owner content")
	}
	payload, err := files.Read(t.Context(), grant.WorkspaceID, messageRef)
	if err != nil || messageDigest != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(payload))) || strings.Contains(payload, syntheticName) {
		t.Fatal("the pending outbox reference did not resolve to minimal immutable content")
	}
}

func assertWorkspaceStatus(t *testing.T, r result, status int, private string) {
	t.Helper()
	if r.err != nil || r.status != status {
		t.Fatalf("workspace endpoint returned HTTP %d; expected %d", r.status, status)
	}
	if status >= 400 {
		assertDenied(t, r, private)
	}
}

func TestPostgresWorkspacesPrivateListener(t *testing.T) {
	db := openLabDatabase(t)
	f := seedLabData(t, db)
	peers := newLabPeers(t, f)
	inventory := []runtimeaccess.Binding{peers.local, peers.remote}
	discovery := &discoveryProbe{first: make(chan struct{}), resume: make(chan struct{})}
	config := db.app.Config()
	config.ConnConfig.Tracer = discovery
	runtimePool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal("could not create the probe's least-privilege application pool")
	}
	t.Cleanup(runtimePool.Close)
	service := newLabApp(t, labDatabase{admin: db.admin, app: runtimePool}, f, peers.local, inventory, labOwner{data: f})
	root := filepath.Join(t.TempDir(), "iam-owner")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal("could not create the dedicated owner directory")
	}
	files, err := workspacefiles.New(root)
	if err != nil {
		t.Fatal("could not open the dedicated owner directory")
	}
	t.Cleanup(func() { _ = files.Close() })
	const existingName, foreignName, newName = "Текущая команда", "Чужое рабочее пространство", "Команда HIT-124"
	seedWorkspaceName(t, db, files, f.workspace, existingName)
	seedWorkspaceName(t, db, files, f.otherWorkspace, foreignName)
	csrf := "workspace-only-synthetic-fixture-" + labUUID()
	hash := sha256.Sum256([]byte(csrf))
	sqlExec(t, db.admin, `UPDATE hp_iam.sessions SET csrf_hash=$1 WHERE session_id=$2`, hash[:], f.session)
	grant := app.Grant{PrincipalID: f.principal, RequestID: labUUID(), WorkspaceID: labUUID(), Lifecycle: "ACTIVE",
		AuditAction: "WORKSPACE_CREATE", AuditReason: "FIXTURE_OWNER_APPROVAL", MessageKind: "WORKSPACE_CREATED_FIXTURE",
		OwnerConfirmedAt: f.now.Add(-time.Second), PayloadExpiresAt: f.now.Add(time.Hour)}
	onboardingOnly := grant
	onboardingOnly.RequestID, onboardingOnly.WorkspaceID, onboardingOnly.Lifecycle = labUUID(), labUUID(), "ONBOARDING"
	cleanWorkspaceFixture(t, db, grant.WorkspaceID)
	cleanWorkspaceFixture(t, db, onboardingOnly.WorkspaceID)
	baseIDs := []string{f.otherWorkspace, grant.WorkspaceID, f.workspace}
	candidates := &fixtureWorkspaceCandidates{principal: f.principal, ids: baseIDs}
	grants := fixtureOnboarding{grant.RequestID: grant, onboardingOnly.RequestID: onboardingOnly}
	loss := &responseLossProbe{Content: files, workspace: grant.WorkspaceID, entered: make(chan struct{})}
	iam, bff := labClient(t, peers.ca, peers.iam), labClient(t, peers.ca, peers.bff)
	base, stop := startWorkspacesFixture(t, service, peers, iam, loss, candidates, grants, "SYNCING")
	fixtureTest := t
	ctx := t.Context()
	remote := assertRegistration(t, post(ctx, bff, base+registerPath, peers.remote.Registration, ""), peers.remote)
	if remote.Gate != "SYNCING" {
		t.Fatal("remote registration alone admitted the BFF")
	}
	registerFixtureBFF := func() {
		t.Helper()
		// A distinct BFF is explicitly provisioned OPEN as an integration
		// fixture. An IAM registration reply cannot open a remote gate.
		sqlExec(t, db.admin, `UPDATE hp_iam.runtime_registry SET status='OPEN' WHERE runtime_id=$1 AND boot_id=$2`, peers.remote.RuntimeID, peers.remote.BootID)
		checkRuntimeRow(t, db, peers.remote, "OPEN")
	}
	list := func(client *http.Client, session string) result {
		return callWorkspaces(ctx, client, base, http.MethodGet, session, "", "", nil, nil)
	}
	create := func(client *http.Client, session, csrf, id string, body []byte) result {
		return callWorkspaces(ctx, client, base, http.MethodPost, session, csrf, id, body, nil)
	}
	originalBody := workspaceBody(t, grant.RequestID, newName)
	assertDenied(t, list(bff, f.opaqueSession), f.opaqueSession)
	registerFixtureBFF()
	assertDenied(t, create(bff, f.opaqueSession, csrf, grant.RequestID, originalBody), csrf)
	if err := service.Start(ctx); err != nil {
		t.Fatal("the exact IAM boot could not open with the provisioned roster")
	}
	assertAllowed(t, post(ctx, iam, base+authorizePath, authorizeRequest(f, peers.local.RuntimeID), f.opaqueSession), f, "OWNER")

	t.Run("the list rejects an epoch change after discovery", func(t *testing.T) {
		discovery.armed.Store(true)
		resume := sync.OnceFunc(func() { close(discovery.resume) })
		defer resume()
		out := make(chan result, 1)
		go func() { out <- list(bff, f.opaqueSession) }()
		waitSignal(t, discovery.first, "initial SQL workspace decision ended")
		sqlExec(t, db.admin, `UPDATE hp_iam.workspaces SET security_epoch=8 WHERE workspace_id=$1`, f.workspace)
		resume()
		select {
		case got := <-out:
			assertWorkspaceStatus(t, got, http.StatusPreconditionFailed, f.opaqueSession)
		case <-time.After(3 * time.Second):
			t.Fatal("the stale list request did not finish")
		}
	})

	t.Run("real TLS and fresh owner authority", func(t *testing.T) {
		assertWorkspaceStatus(t, list(iam, f.opaqueSession), http.StatusForbidden, f.opaqueSession)
		assertWorkspaceStatus(t, list(bff, ""), http.StatusUnauthorized, f.opaqueSession)
		for _, certs := range [][]tls.Certificate{
			nil,
			{peers.ca.leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, true)},
			{peers.ca.leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, false)},
			{newLabCA(t).leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, false)},
		} {
			if got := list(labClient(t, peers.ca, certs...), f.opaqueSession); got.err == nil || got.status != 0 {
				t.Fatal("missing, expired, unlisted or untrusted client certificate passed mTLS")
			}
		}
		tls12 := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: peers.ca.roots, Certificates: []tls.Certificate{peers.bff}, MaxVersion: tls.VersionTLS12}}
		t.Cleanup(tls12.CloseIdleConnections)
		if got := list(&http.Client{Transport: tls12, Timeout: time.Second}, f.opaqueSession); got.err == nil || got.status != 0 {
			t.Fatal("the IAM listener accepted TLS 1.2")
		}
		allowed := list(bff, f.opaqueSession)
		assertWorkspaceStatus(t, allowed, http.StatusOK, f.opaqueSession)
		var listed struct{ Items []app.Workspace }
		if json.Unmarshal(allowed.body, &listed) != nil || len(listed.Items) != 1 || listed.Items[0].WorkspaceID != f.workspace ||
			listed.Items[0].Name != existingName || listed.Items[0].Role != "OWNER" || bytes.Contains(allowed.body, []byte(foreignName)) {
			t.Fatal("candidate list overrode current membership or owner content")
		}
		sqlExec(t, db.admin, `UPDATE hp_iam.memberships SET role='OPERATOR',revision=2 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal)
		updated := list(bff, f.opaqueSession)
		assertWorkspaceStatus(t, updated, http.StatusOK, f.opaqueSession)
		if json.Unmarshal(updated.body, &listed) != nil || len(listed.Items) != 1 || listed.Items[0].Role != "OPERATOR" {
			t.Fatal("workspace list retained a stale role")
		}
		sqlExec(t, db.admin, `UPDATE hp_iam.memberships SET status='REVOKED',revoked_at=clock_timestamp(),revision=3 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal)
		empty := list(bff, f.opaqueSession)
		assertWorkspaceStatus(t, empty, http.StatusOK, f.opaqueSession)
		if string(empty.body) != `{"items":[]}` {
			t.Fatal("revoked membership remained visible or empty schema was not preserved")
		}
	})

	t.Run("the list cannot return more than fifty current workspaces", func(t *testing.T) {
		ids := make([]string, 0, 51)
		for range 51 {
			id := labUUID()
			cleanWorkspaceFixture(t, db, id)
			sqlExec(t, db.admin, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
 VALUES($1,$2,1,1,'ACTIVE',$3,$3)`, id, labUUID(), f.now)
			sqlExec(t, db.admin, `INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at)
 VALUES($1,$2,'READER','ACTIVE',1,$3)`, id, f.principal, f.now)
			seedWorkspaceName(t, db, files, id, "Синтетическое тестовое пространство")
			ids = append(ids, id)
		}
		candidates.replace(ids)
		defer candidates.replace(baseIDs)
		bounded := list(bff, f.opaqueSession)
		assertWorkspaceStatus(t, bounded, http.StatusOK, f.opaqueSession)
		var listed struct{ Items []app.Workspace }
		if json.Unmarshal(bounded.body, &listed) != nil || len(listed.Items) != 50 {
			t.Fatal("the real workspace list did not preserve the accepted item bound")
		}
		slices.Sort(ids)
		for i, item := range listed.Items {
			if item.WorkspaceID != ids[i] || item.Role != "READER" {
				t.Fatal("the bounded response did not contain the deterministic permitted subset")
			}
		}
	})

	t.Run("strict command boundary and durable same-ID replay", func(t *testing.T) {
		invalidID := labUUID()
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, csrf, invalidID, workspaceBody(t, invalidID, newName)), http.StatusForbidden, csrf)
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, csrf, onboardingOnly.RequestID, workspaceBody(t, onboardingOnly.RequestID, newName)), http.StatusForbidden, csrf)
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, strings.Repeat("z", 43), grant.RequestID, originalBody), http.StatusForbidden, csrf)
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, "", grant.RequestID, originalBody), http.StatusBadRequest, csrf)
		malformed := append([]byte(`{"name":"duplicate",`), originalBody[1:]...)
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, csrf, grant.RequestID, malformed), http.StatusBadRequest, csrf)
		if err := os.Chmod(root, 0755); err != nil {
			t.Fatal("could not simulate an unsafe owner directory")
		}
		unavailable := create(bff, f.opaqueSession, csrf, grant.RequestID, originalBody)
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal("could not restore the exact owner directory permissions")
		}
		assertWorkspaceStatus(t, unavailable, http.StatusServiceUnavailable, csrf)
		var before int
		if err := db.admin.QueryRow(ctx, `SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id IN($1,$2)`, grant.WorkspaceID, onboardingOnly.WorkspaceID).Scan(&before); err != nil || before != 0 {
			t.Fatal("denial, invalid input or unavailable content left a SQL workspace")
		}
		if _, err := os.Lstat(filepath.Join(root, grant.WorkspaceID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("a denied command created owner content")
		}

		// Disconnect after the durable SQL outcome and before the callback
		// can start its actual HTTPS response. Restart the listener and owner
		// adapter, then retry with the original command ID and payload.
		loss.armed.Store(true)
		abandoned, disconnect := context.WithCancel(ctx)
		defer disconnect()
		lost := make(chan result, 1)
		go func() {
			lost <- callWorkspaces(abandoned, bff, base, http.MethodPost, f.opaqueSession, csrf, grant.RequestID, originalBody, nil)
		}()
		waitSignal(t, loss.entered, "a durable create reached its response content read")
		var durable int
		if err := db.admin.QueryRow(ctx, `SELECT count(*) FROM hp_iam.command_receipts WHERE workspace_id=$1 AND actor_id=$2 AND request_id=$3`,
			grant.WorkspaceID, f.principal, grant.RequestID).Scan(&durable); err != nil || durable != 1 {
			t.Fatal("the abandoned response had no durable receipt")
		}
		disconnect()
		select {
		case got := <-lost:
			if got.err == nil || got.status != 0 {
				t.Fatal("the abandoned client received a response")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("the abandoned client did not disconnect")
		}
		stop()
		if err := files.Close(); err != nil {
			t.Fatal("could not close the first owner content adapter")
		}
		reopened, err := workspacefiles.New(root)
		if err != nil {
			t.Fatal("could not reopen the same owner content for replay")
		}
		files = reopened
		base, stop = startWorkspacesFixture(fixtureTest, service, peers, iam, files, candidates, grants, "OPEN")

		// Competing same-ID requests cross the real TLS listener and SQL
		// boundary. Every authorized replay must report the durable result.
		const replays = 4
		var group sync.WaitGroup
		out := make(chan result, replays)
		for range replays {
			group.Go(func() { out <- create(bff, f.opaqueSession, csrf, grant.RequestID, originalBody) })
		}
		group.Wait()
		close(out)
		for got := range out {
			assertWorkspaceStatus(t, got, http.StatusCreated, csrf)
			var workspace app.Workspace
			if json.Unmarshal(got.body, &workspace) != nil || workspace.WorkspaceID != grant.WorkspaceID || workspace.Name != newName ||
				workspace.Revision != 1 || workspace.SecurityEpoch != 1 || workspace.Role != "OWNER" || workspace.Lifecycle != "ACTIVE" {
				t.Fatal("the admitted create and replay did not return the committed workspace")
			}
		}
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, csrf, grant.RequestID, workspaceBody(t, grant.RequestID, "Конфликт")), http.StatusConflict, csrf)
		checkWorkspaceWrite(t, db, files, f.principal, grant, newName)
	})

	t.Run("unprovisioned onboarding and candidate authorities stay closed", func(t *testing.T) {
		stop()
		base, stop = startWorkspacesFixture(fixtureTest, service, peers, iam, files, nil, nil, "OPEN")
		assertWorkspaceStatus(t, list(bff, f.opaqueSession), http.StatusServiceUnavailable, f.opaqueSession)
		assertWorkspaceStatus(t, create(bff, f.opaqueSession, csrf, grant.RequestID, originalBody), http.StatusServiceUnavailable, csrf)
		checkWorkspaceWrite(t, db, files, f.principal, grant, newName)
		stop()
		base, stop = startWorkspacesFixture(fixtureTest, service, peers, iam, files, candidates, grants, "OPEN")
	})

	t.Run("durable runtime and session decisions", func(t *testing.T) {
		current := list(bff, f.opaqueSession)
		assertWorkspaceStatus(t, current, http.StatusOK, f.opaqueSession)
		if !bytes.Contains(current.body, []byte(grant.WorkspaceID)) || bytes.Contains(current.body, []byte(f.workspace)) {
			t.Fatal("new creator membership was not discoverable through the candidate source and fresh SQL")
		}
		sqlExec(t, db.admin, `UPDATE hp_iam.runtime_registry SET status='SYNCING' WHERE runtime_id=$1`, peers.remote.RuntimeID)
		assertDenied(t, list(bff, f.opaqueSession), f.opaqueSession)
		registerFixtureBFF()
		sqlExec(t, db.admin, `INSERT INTO hp_iam.revocations(revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,'SESSION',$2,4,$3,$3::timestamptz+interval '5 seconds','PENDING','FIXTURE_SESSION_DENY')`, labUUID(), f.session, f.now)
		assertDenied(t, list(bff, f.opaqueSession), f.opaqueSession)
		assertDenied(t, create(bff, f.opaqueSession, csrf, grant.RequestID, originalBody), f.opaqueSession)
		checkWorkspaceWrite(t, db, files, f.principal, grant, newName)
	})

	stop()
}
