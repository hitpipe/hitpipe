# Исходники и происхождение квалификации HIT-146

Код IAM собирался из отдельной чистой копии с принятой веткой HIT-116, которая включает точный результат HIT-115. Processing собран из отдельной ветки от HIT-111. Открытые исходные PR не входили в наблюдаемый main.

| Зависимость | Проверенный SHA | Фактическая ветка |
| --- | --- | --- |
| HIT-111 | b2e823012854575146595b01a6f56d5ac677fc44 | [Draft PR №72](https://github.com/hitpipe/hitpipe/pull/72) 49229e1db3f371a051a022ea9e31951dd558eecb. [PASS WITH NOTES 8/8](https://hitpipe.atlassian.net/browse/HIT-111?focusedCommentId=14274) |
| HIT-115 | 0ff7f039ccaf450d60a59cd9cc0504c13e032b08 | [Draft PR №80](https://github.com/hitpipe/hitpipe/pull/80) c7a74f55ea8111fa290f803976d256cc8a4a202e. [PASS 8/8](https://hitpipe.atlassian.net/browse/HIT-115?focusedCommentId=15478) |
| HIT-116 | d13f723511b61cb3068bfdf63d40bd7100fa1e89 | [Draft PR №87](https://github.com/hitpipe/hitpipe/pull/87) ae5fd0f35ec7d5097cace75b3b15ecb14042c654. [PASS 8/8](https://hitpipe.atlassian.net/browse/HIT-116?focusedCommentId=16116) |

[Фактические команды и коды выхода](source-commands.json). Exit 1 у проверки предка main означает, что принятый код остаётся в своей ветке. Diff реализации IAM между проверенным кодом HIT-116 и принятым head пуст.

`git merge-base --is-ancestor b2e823012854575146595b01a6f56d5ac677fc44 49229e1db3f371a051a022ea9e31951dd558eecb` → exit 0.

`git merge-base --is-ancestor b2e823012854575146595b01a6f56d5ac677fc44 origin/main` → exit 1.

`git merge-base --is-ancestor 0ff7f039ccaf450d60a59cd9cc0504c13e032b08 ae5fd0f35ec7d5097cace75b3b15ecb14042c654` → exit 0.

`git merge-base --is-ancestor 0ff7f039ccaf450d60a59cd9cc0504c13e032b08 origin/main` → exit 1.

`git merge-base --is-ancestor d13f723511b61cb3068bfdf63d40bd7100fa1e89 ae5fd0f35ec7d5097cace75b3b15ecb14042c654` → exit 0.

`git merge-base --is-ancestor d13f723511b61cb3068bfdf63d40bd7100fa1e89 origin/main` → exit 1.

`git merge-base --is-ancestor 49229e1db3f371a051a022ea9e31951dd558eecb ce317193244623c5a464a3014bb39f32875c1e98` → exit 0.

`git merge-base --is-ancestor c7a74f55ea8111fa290f803976d256cc8a4a202e ae5fd0f35ec7d5097cace75b3b15ecb14042c654` → exit 0.

`git diff --exit-code d13f723511b61cb3068bfdf63d40bd7100fa1e89 ae5fd0f35ec7d5097cace75b3b15ecb14042c654 -- services/iam/internal services/platform/startgate` → exit 0.

`git diff --exit-code 49229e1db3f371a051a022ea9e31951dd558eecb ce317193244623c5a464a3014bb39f32875c1e98 -- . :(exclude)services/processing/internal/adapters/postgres/*_test.go :(exclude)services/processing/scripts/postgres/** :(exclude)tools/hit146/iam-process.go` → exit 0.

`git diff --exit-code 49229e1db3f371a051a022ea9e31951dd558eecb 4250bb434a9a48ed436904e0a4609717135f7143 -- services/processing/internal/runtime/iam.go` → exit 0.

`git diff --name-status 49229e1db3f371a051a022ea9e31951dd558eecb ce317193244623c5a464a3014bb39f32875c1e98` → exit 0.
```
A	services/processing/internal/adapters/postgres/real_iam_process_test.go
M	services/processing/internal/adapters/postgres/runtime_process_test.go
A	services/processing/scripts/postgres/verify_real_iam.py
A	tools/hit146/iam-process.go
```

Точный патч TEST от принятой базы Processing SHA-256 ba41f110ea62eba377d2823e3abbba946b3eaa7466f6567484f256cdde92bfa6.

```diff
diff --git a/services/processing/internal/adapters/postgres/real_iam_process_test.go b/services/processing/internal/adapters/postgres/real_iam_process_test.go
new file mode 100644
index 0000000000000000000000000000000000000000..3ee4b4a39df76a6c25e30b8f7570ff7f04b038d8
--- /dev/null
+++ b/services/processing/internal/adapters/postgres/real_iam_process_test.go
@@ -0,0 +1,275 @@
+package postgres
+
+import (
+	"bytes"
+	"crypto/sha256"
+	"crypto/tls"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"io"
+	"net"
+	"net/http"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"testing"
+	"time"
+
+	"github.com/hitpipe/hitpipe/services/platform/startgate"
+	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
+	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
+	"github.com/jackc/pgx/v5"
+)
+
+type iamTrace struct {
+	Message    string `json:"msg"`
+	Status     int    `json:"status"`
+	RuntimeID  string `json:"runtime_id"`
+	BootID     string `json:"boot_id"`
+	Generation int64  `json:"generation"`
+}
+
+func awaitIAMTrace(t *testing.T, path string, want func(iamTrace) bool) iamTrace {
+	t.Helper()
+	until := time.Now().Add(6 * time.Second)
+	for time.Now().Before(until) {
+		raw, err := os.ReadFile(path)
+		if err != nil {
+			t.Fatal(err)
+		}
+		for _, line := range bytes.Split(raw, []byte{'\n'}) {
+			var event iamTrace
+			if json.Unmarshal(line, &event) == nil && want(event) {
+				return event
+			}
+		}
+		time.Sleep(40 * time.Millisecond)
+	}
+	raw, _ := os.ReadFile(path)
+	t.Fatalf("actual IAM observation missing: %s", raw)
+	return iamTrace{}
+}
+
+func postIAMFixture(t *testing.T, st *processStand, cert tls.Certificate, path, session string, body any) (int, []byte, error) {
+	t.Helper()
+	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
+		DisableKeepAlives: true,
+		TLSClientConfig: &tls.Config{RootCAs: st.pki.Root, Certificates: []tls.Certificate{cert},
+			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13},
+	}}
+	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://127.0.0.2:8443"+path, bytes.NewReader(marshalProcess(t, body)))
+	if err != nil {
+		t.Fatal(err)
+	}
+	id, _ := newID()
+	req.Header.Set("X-Request-ID", id)
+	req.Header.Set("Content-Type", "application/json")
+	if session != "" {
+		req.Header.Set("X-HitPipe-Session", session)
+	}
+	resp, err := client.Do(req)
+	if err != nil {
+		return 0, nil, err
+	}
+	defer resp.Body.Close()
+	raw, err := io.ReadAll(resp.Body)
+	return resp.StatusCode, raw, err
+}
+
+func seedIAMWorkspace(t *testing.T, st *processStand, ws string) {
+	t.Helper()
+	db, err := pgx.Connect(t.Context(), "host=/var/run/postgresql user=hp_hit146_iam_app dbname=hit146_iam sslmode=disable")
+	if err != nil {
+		t.Fatal("open disposable IAM fixture database")
+	}
+	defer db.Close(ctx)
+	tx, err := db.Begin(t.Context())
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer tx.Rollback(ctx)
+	session, csrf := sha256.Sum256([]byte(st.token)), sha256.Sum256([]byte(st.csrf))
+	for _, s := range []struct {
+		sql  string
+		args []any
+	}{
+		{"SELECT set_config('hp.workspace_id',$1,true)", []any{ws}},
+		{"INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://test.invalid','synthetic',now())", []any{owner}},
+		{"INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch) VALUES($1,$2,$3,$4,now(),now()+interval '1 hour',now()+interval '30 minutes',7)", []any{st.session, session[:], owner, csrf[:]}},
+		{"INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at) VALUES($1,$2,1,7,'ACTIVE',now())", []any{ws, config}},
+		{"INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at) VALUES($1,$2,'OWNER','ACTIVE',1,now())", []any{ws, owner}},
+	} {
+		if _, err := tx.Exec(t.Context(), s.sql, s.args...); err != nil {
+			t.Fatal("seed disposable IAM owner, workspace and session")
+		}
+	}
+	if err := tx.Commit(t.Context()); err != nil {
+		t.Fatal("commit IAM fixture")
+	}
+}
+
+func TestRealIAMRegistrationBoundary(t *testing.T) {
+	processing, iam := os.Getenv("HIT111_PROCESSING_BIN"), os.Getenv("HIT146_IAM_BIN")
+	if os.Getenv("HIT78_ISOLATED_PG") != "1" || processing == "" || iam == "" {
+		t.Skip("requires the isolated IAM and Processing runner")
+	}
+	f := prepared(t, []string{})
+	result, err := f.service.Prepare(ctx, f.request)
+	if err != nil || result.Disposition != "PENDING_OUTBOX" {
+		t.Fatal("seed one pending synthetic delivery")
+	}
+	ws := f.request.WorkspaceID
+	st := createProcessStandWithIAM(t, processing, f.path, map[string]string{ws: "acct_synthetic"}, false)
+	seedIAMWorkspace(t, st, ws)
+
+	// The fixed binding belongs to TEST provisioning, never to the incoming
+	// registration. The standalone process selects its own boot on each launch.
+	boot, _ := newID()
+	iamRuntime, _ := newID()
+	iamBoot, _ := newID()
+	revokeID, _ := newID()
+	leaf := st.pki.Issue(t, "iam.test", net.ParseIP("127.0.0.2"), true, false)
+	cert, key := st.keypair("real-iam", leaf)
+	iamClient := st.pki.Leaf(t, "iam.test", false, false)
+	clientCert, clientKey := st.keypair("real-iam-client", iamClient)
+	iamFingerprint := sha256.Sum256(iamClient.Certificate[0])
+	procCert, err := tls.LoadX509KeyPair(filepath.Join(st.dir, "processing-client.pem"), filepath.Join(st.dir, "processing-client.key"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	registration := drafts.RuntimeRegistration{RuntimeID: st.identity, Service: "PROCESSING", Generation: 2, BootID: boot, CertificateFingerprint: st.cert}
+	raw, err := os.ReadFile(st.config)
+	var options map[string]any
+	if err != nil || json.Unmarshal(raw, &options) != nil {
+		t.Fatal("read synthetic Processing configuration")
+	}
+	options["generation"] = registration.Generation
+	st.config = st.json("real-processing.json", options)
+	tracePath := filepath.Join(st.dir, "iam-trace.jsonl")
+	trace, err := os.Create(tracePath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer trace.Close()
+	var binding map[string]any
+	if err = json.Unmarshal(marshalProcess(t, registration), &binding); err != nil {
+		t.Fatal(err)
+	}
+	binding["dns_name"] = "processing-client.test"
+	cfg := st.json("iam-process.json", map[string]any{
+		"IAM": map[string]any{"runtime_id": iamRuntime, "boot_id": iamBoot, "service": "IAM", "generation": 2,
+			"dns_name": "iam.test", "certificate_fingerprint": "sha256:" + hex.EncodeToString(iamFingerprint[:])},
+		"Processing": binding, "CA": filepath.Join(st.dir, "ca.pem"), "Certificate": cert, "Key": key,
+		"ClientCertificate": clientCert, "ClientKey": clientKey,
+		"Workspace": ws, "Revocation": revokeID, "Dir": st.dir,
+	})
+	command := exec.Command(iam, cfg)
+	command.Stdout, command.Stderr = trace, trace
+	if err = command.Start(); err != nil {
+		t.Fatal(err)
+	}
+	done := make(chan error, 1)
+	go func() { done <- command.Wait(); close(done) }()
+	t.Cleanup(func() {
+		_ = command.Process.Kill()
+		<-done
+		log, _ := os.ReadFile(tracePath)
+		t.Logf("actual IAM process trace:\n%s", log)
+	})
+	awaitIAMTrace(t, tracePath, func(e iamTrace) bool { return e.Message == "iam-listening" })
+
+	const register = "/internal/v1/runtime/register"
+	code, payload, err := postIAMFixture(t, st, procCert, register, "", registration)
+	var current drafts.RuntimeSnapshot
+	if err != nil || code != 200 || draftjson.Validate("RuntimeSnapshot", payload) != nil ||
+		json.Unmarshal(payload, &current) != nil || current.Gate != "SYNCING" || current.BootID != boot ||
+		current.Generation != 2 || current.RuntimeID != st.identity {
+		t.Fatalf("expected a real matching SYNCING snapshot, code=%d: %v", code, err)
+	}
+	awaitIAMTrace(t, tracePath, func(e iamTrace) bool { return e.Message == "iam-gate-open" })
+	code, again, err := postIAMFixture(t, st, procCert, register, "", registration)
+	var repeated drafts.RuntimeSnapshot
+	if err != nil || code != 200 || json.Unmarshal(again, &repeated) != nil || repeated.Gate != "SYNCING" || repeated.SnapshotRevision <= current.SnapshotRevision {
+		t.Fatal("fresh matching remote registration after IAM opened did not stay SYNCING", code)
+	}
+	t.Logf("exact provisioning, actual IAM HTTP 200 before and after local IAM OPEN: %s / %s", payload, again)
+
+	gate, err := startgate.New(startgate.Identity{RuntimeID: current.RuntimeID, BootID: current.BootID, Generation: 3})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err = gate.ApplySnapshot(ctx, startgate.RuntimeSnapshot{RuntimeID: current.RuntimeID, BootID: current.BootID,
+		Generation: uint64(current.Generation), SnapshotRevision: uint64(current.SnapshotRevision), Gate: startgate.State(current.Gate)}); !errors.Is(err, startgate.ErrIdentity) {
+		t.Fatal("current generation accepted the genuine old IAM snapshot", err)
+	}
+	t.Log("actual generation=2 snapshot rejected by generation=3 startgate")
+	old := registration
+	old.Generation--
+	code, _, err = postIAMFixture(t, st, procCert, register, "", old)
+	if err != nil || code != 403 {
+		t.Fatal("old generation registration did not receive 403", code, err)
+	}
+	foreign := st.pki.Leaf(t, "bff.test", false, false)
+	if code, _, err = postIAMFixture(t, st, foreign, register, "", registration); err == nil || code != 0 {
+		t.Fatal("wrong service principal received an HTTP response", code)
+	}
+	t.Log("old generation registration HTTP 403, wrong mTLS principal rejected at TLS")
+	code, _, err = postIAMFixture(t, st, procCert, "/internal/v1/authorize", st.token,
+		drafts.AuthorizeRequest{WorkspaceID: ws, RuntimeID: st.identity, Action: "CONFIG_READ"})
+	if err != nil || code != 403 {
+		t.Fatal("authorized-use probe did not reject the remote SYNCING row", code, err)
+	}
+	t.Log("actual IAM authorize for valid TEST session and registered remote SYNCING runtime HTTP 403")
+
+	check := func(name, previousBoot string) (*processingChild, string) {
+		p := st.boot(name)
+		st.pollHealth(p, "CLOSED")
+		trace := awaitIAMTrace(t, tracePath, func(e iamTrace) bool {
+			return e.Message == "iam-register" && e.Status == 403 && e.RuntimeID == st.identity &&
+				e.Generation == 2 && e.BootID != "" && e.BootID != boot && e.BootID != previousBoot
+		})
+		health := st.must("ops.test", "9444", "GET", "/internal/v1/health/processing", nil, nil, 200)
+		st.must("bff.test", "8443", "GET", "/api/v1/workspaces/"+ws+"/operations", nil,
+			map[string]string{"X-HitPipe-Session": st.token}, 503)
+		t.Logf("%s boot=%s, actual IAM registration HTTP 403, protected call HTTP 503, OPS HTTP 200 %s", name, trace.BootID, health)
+		return p, trace.BootID
+	}
+	p, first := check("first-start", "")
+	if err = p.cmd.Process.Kill(); err != nil {
+		t.Fatal(err)
+	}
+	<-p.done
+	check("restart", first)
+	control, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", "127.0.0.1:9443",
+		&tls.Config{RootCAs: st.pki.Root, Certificates: []tls.Certificate{iamClient},
+			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13})
+	if err != nil {
+		t.Fatal("could not inspect the actual Processing control certificate", err)
+	}
+	serverHash := sha256.Sum256(control.ConnectionState().PeerCertificates[0].Raw)
+	_ = control.Close()
+	serverPin := "sha256:" + hex.EncodeToString(serverHash[:])
+	if serverPin == st.cert {
+		t.Fatal("the stand must use distinct outbound and control certificates")
+	}
+	t.Logf("control TLS leaf=%s; registered outbound leaf=%s", serverPin, st.cert)
+	if err := os.WriteFile(filepath.Join(st.dir, "revoke"), nil, 0600); err != nil {
+		t.Fatal(err)
+	}
+	select {
+	case err := <-done:
+		if err != nil {
+			t.Fatal("IAM coordinator did not complete the no-ACK observation", err)
+		}
+	case <-time.After(10 * time.Second):
+		t.Fatal("IAM coordinator no-ACK observation timed out")
+	}
+	if _, err := os.Stat(filepath.Join(st.dir, "incident-observed")); err != nil {
+		t.Fatal("missing durable incident observation", err)
+	}
+	if st.deliveryCount.Load() != 0 {
+		t.Fatal("a closed Processing dispatched the seeded outbox")
+	}
+	t.Log("BLOCKED: no protected start, no create, no accepted remote OPEN and no EFFECTIVE revocation")
+}
diff --git a/services/processing/internal/adapters/postgres/runtime_process_test.go b/services/processing/internal/adapters/postgres/runtime_process_test.go
index 7740c0702cf8f89c8d91b82c4afdcfb410251f00..03793e8620936568a576622534f18dcfab6a348e 100644
--- a/services/processing/internal/adapters/postgres/runtime_process_test.go
+++ b/services/processing/internal/adapters/postgres/runtime_process_test.go
@@ -233,0 +234,5 @@ func createProcessStand(t *testing.T, binary, files string, workspaces map[strin
+	t.Helper()
+	return createProcessStandWithIAM(t, binary, files, workspaces, true)
+}
+
+func createProcessStandWithIAM(t *testing.T, binary, files string, workspaces map[string]string, useFixtureIAM bool) *processStand {
@@ -271 +276,3 @@ func createProcessStand(t *testing.T, binary, files string, workspaces map[strin
-	s.peer("127.0.0.2:8443", "iam.test", s.iam)
+	if useFixtureIAM {
+		s.peer("127.0.0.2:8443", "iam.test", s.iam)
+	}
diff --git a/services/processing/scripts/postgres/verify_real_iam.py b/services/processing/scripts/postgres/verify_real_iam.py
new file mode 100644
index 0000000000000000000000000000000000000000..b36c7ee268add0006e0f1a7d88faa6207a6e6911
--- /dev/null
+++ b/services/processing/scripts/postgres/verify_real_iam.py
@@ -0,0 +1,113 @@
+#!/usr/bin/env python3
+"""Reproduce the Processing/IAM startup boundary on the owned local stand."""
+import argparse
+import hashlib
+import json
+import pathlib
+import tempfile
+import uuid
+
+import verify
+
+IAM_HEAD = "ae5fd0f35ec7d5097cace75b3b15ecb14042c654"
+IAM_SCHEMA = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"
+
+
+class Run(verify.Run):
+    def __init__(self, output, go, iam, scratch):
+        super().__init__(output)
+        self.name = "hit146-" + uuid.uuid4().hex
+        self.db = "hit78"
+        self.go, self.iam, self.scratch = str(go), iam, scratch
+        self.iam_module = self.iam / "services/iam"
+        self.migrations = self.iam_module / "internal/adapters/postgres/migrations"
+        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
+
+    def command(self, label, args, *rest, **kwargs):
+        if label == "container-create":
+            args = list(args)
+            args[2:2] = [
+                "--mount", f"type=bind,source={self.scratch},target=/test,readonly",
+                "--mount", f"type=bind,source={self.migrations},target=/iam-migration,readonly",
+            ]
+            args[args.index("hitpipe.task=HIT-76")] = "hitpipe.task=HIT-146"
+        return super().command(label, args, *rest, **kwargs)
+
+    def build(self, architecture):
+        if self.command("iam-head", ["git", "-C", str(self.iam), "rev-parse", "HEAD"]).stdout.decode().strip() != IAM_HEAD:
+            raise RuntimeError("use the pinned IAM checkout")
+        if self.command("iam-clean", ["git", "-C", str(self.iam), "status", "--porcelain"]).stdout:
+            raise RuntimeError("use a clean IAM checkout")
+        if hashlib.sha256((self.migrations / "schema/001_iam.sql").read_bytes()).hexdigest() != IAM_SCHEMA:
+            raise RuntimeError("IAM migration differs from the pinned formal SQL")
+        go = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go]
+        if self.command("go-version", [*go, "version"]).stdout.split()[2] != b"go1.27.2":
+            raise RuntimeError("Go 1.27.2 required")
+        processing = [*go, "-C", str(verify.MODULE)]
+        self.command("go-race", [*processing, "test", "-race", "-count=1", "./..."])
+        self.command("go-vet", [*processing, "vet", "./..."])
+        linux = ["env", "GOOS=linux", "GOARCH=" + architecture, "CGO_ENABLED=0", *go]
+        binaries = self.scratch
+        self.command("build-processing", [*linux, "-C", str(verify.MODULE), "build", "-trimpath",
+            "-ldflags", "-X main.revision=" + self.source, "-o", str(binaries / "processing"), "./cmd/processing"])
+        self.command("build-processing-tests", [*linux, "-C", str(verify.MODULE), "test", "-c",
+            "-o", str(binaries / "storage.test"), "./internal/adapters/postgres"])
+        # Only the fixture main is overlaid. The IAM checkout and all imported
+        # IAM implementation files remain the pinned originals.
+        probe = verify.REPO / "tools/hit146/iam-process.go"
+        overlay = binaries / "iam.probe.overlay.json"
+        overlay.write_text(json.dumps({"Replace": {
+            str(self.iam_module / "cmd/iam-contractcheck/main.go"): str(probe),
+        }}) + "\n")
+        self.command("build-iam-probe", [*linux, "-C", str(self.iam_module), "build", "-trimpath", "-tags", "hit146_iam_fixture",
+            "-overlay", str(overlay), "-o", str(binaries / "iam-probe"), "./cmd/iam-contractcheck"])
+        self.command("build-iam-migrate", [*linux, "-C", str(self.iam_module), "build", "-trimpath",
+            "-o", str(binaries / "iam-migrate"), "./cmd/iam-migrate"])
+        (self.output / "binaries.json").write_text(json.dumps({
+            "processing_sha": self.source, "iam_sha": IAM_HEAD, "platform": "linux/" + architecture,
+            "fixture_main_sha256": hashlib.sha256(probe.read_bytes()).hexdigest(),
+            "binaries": {file.name: hashlib.sha256(file.read_bytes()).hexdigest()
+                for file in binaries.iterdir() if file != overlay},
+        }, indent=2) + "\n")
+
+    def checks(self):
+        arch = self.command("container-arch", ["docker", "exec", self.container, "uname", "-m"]).stdout.decode().strip()
+        self.build({"aarch64": "arm64", "x86_64": "amd64"}[arch])
+        self.sql("iam-database", "CREATE DATABASE hit146_iam; REVOKE ALL ON DATABASE hit146_iam FROM PUBLIC;",
+                 role="postgres", database="postgres")
+        self.sql("iam-roles", "\\i /iam-migration/bootstrap_roles.sql\n", role="postgres", database="hit146_iam")
+        self.sql("iam-logins",
+                 "CREATE ROLE hp_hit146_iam_app LOGIN INHERIT; CREATE ROLE hp_hit146_iam_migrate LOGIN INHERIT;"
+                 "GRANT hp_iam_app TO hp_hit146_iam_app; GRANT hp_iam_migrator TO hp_hit146_iam_migrate;"
+                 "GRANT CONNECT ON DATABASE hit146_iam TO hp_hit146_iam_app,hp_hit146_iam_migrate;",
+                 role="postgres", database="hit146_iam")
+        self.command("iam-migration", ["docker", "exec", "-e",
+            "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql user=hp_hit146_iam_migrate"
+            " dbname=hit146_iam sslmode=disable options='-c role=hp_iam_migrator'",
+            self.container, "/test/iam-migrate"])
+        self.command("real-iam-boundary", ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1",
+            "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT146_IAM_BIN=/test/iam-probe",
+            self.container, "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s",
+            "-test.run=^TestRealIAMRegistrationBoundary$"])
+        self.command("processing-predecessor", ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1",
+            "-e", "HIT111_PROCESSING_BIN=/test/processing", self.container, "/test/storage.test",
+            "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestProcessingExecutableOwnedStand$"])
+
+    def main(self):
+        if self.tree_status:
+            raise RuntimeError("use a clean Processing checkout")
+        super().main()
+        path = self.output / "summary.json"
+        result = json.loads(path.read_text())
+        result.update(status="BLOCKED_REPRODUCED", iam_source_sha=IAM_HEAD,
+                      scope="Negative real-IAM startup and missing-ACK deadline; independent controlled HIT-111 regression.")
+        path.write_text(json.dumps(result, indent=2) + "\n")
+
+
+if __name__ == "__main__":
+    parser = argparse.ArgumentParser(description=__doc__)
+    for argument in ("go", "iam", "output"):
+        parser.add_argument("--" + argument, type=pathlib.Path, required=True)
+    args = parser.parse_args()
+    with tempfile.TemporaryDirectory(prefix="hit146-build-") as scratch:
+        Run(args.output.resolve(), args.go.resolve(), args.iam.resolve(), pathlib.Path(scratch)).main()
diff --git a/tools/hit146/iam-process.go b/tools/hit146/iam-process.go
new file mode 100644
index 0000000000000000000000000000000000000000..1533e6dd75fa5e7e3022f52268d4620eb1c9b56e
--- /dev/null
+++ b/tools/hit146/iam-process.go
@@ -0,0 +1,227 @@
+//go:build hit146_iam_fixture
+
+// Built by the disposable Processing qualification runner as an overlay on the
+// accepted IAM checkout. This is only an assembly for existing IAM components.
+package main
+
+import (
+	"bytes"
+	"context"
+	"crypto/tls"
+	"crypto/x509"
+	"encoding/json"
+	"errors"
+	"io"
+	"log/slog"
+	"net"
+	"net/http"
+	"os"
+	"path/filepath"
+	"time"
+
+	composition "github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam"
+	privatehttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
+	barrierhttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/revocation"
+	revocationdb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/revocation"
+	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
+	"github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
+	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
+	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
+	"github.com/hitpipe/hitpipe/services/platform/startgate"
+	"github.com/jackc/pgx/v5/pgxpool"
+)
+
+type fixture struct {
+	IAM, Processing              runtimeaccess.Binding
+	CA, Certificate, Key         string
+	ClientCertificate, ClientKey string
+	Workspace, Revocation, Dir   string
+}
+
+type clock struct{}
+
+func (clock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
+	if err := ctx.Err(); err != nil {
+		return sessionpolicy.Observation{}, err
+	}
+	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: true}, nil
+}
+
+type closedOwner struct{}
+
+func (closedOwner) Prove(context.Context, authorizeaction.Actor, authorizeaction.Request) (authorizeaction.Proof, error) {
+	return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
+}
+
+type statusWriter struct {
+	http.ResponseWriter
+	status int
+}
+
+func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
+
+func (w *statusWriter) WriteHeader(status int) {
+	w.status = status
+	w.ResponseWriter.WriteHeader(status)
+}
+
+func trace(h http.Handler, log *slog.Logger) http.Handler {
+	return http.HandlerFunc(func(out http.ResponseWriter, req *http.Request) {
+		w := &statusWriter{ResponseWriter: out, status: 200}
+		var registration bytes.Buffer
+		if req.URL.Path == privatehttp.RegisterPath && req.Body != nil {
+			req.Body = struct {
+				io.Reader
+				io.Closer
+			}{io.TeeReader(req.Body, &registration), req.Body}
+		}
+		h.ServeHTTP(w, req)
+		var r runtimeaccess.Registration
+		if registration.Len() != 0 && json.Unmarshal(registration.Bytes(), &r) == nil {
+			log.Info("iam-register", "status", w.status, "runtime_id", r.RuntimeID, "boot_id", r.BootID, "generation", r.Generation)
+		} else {
+			log.Info("iam-http", "status", w.status, "path", req.URL.Path)
+		}
+	})
+}
+
+type tracedSender struct {
+	client *barrierhttp.Client
+	log    *slog.Logger
+}
+
+func (s tracedSender) Send(ctx context.Context, target revocation.Participant, command startgate.BarrierCommand) (startgate.BarrierAck, error) {
+	ack, err := s.client.Send(ctx, target, command)
+	s.log.Info("coordinator-send", "runtime_id", target.Identity.RuntimeID, "generation", target.Identity.Generation, "ack_received", err == nil)
+	return ack, err
+}
+
+func observeNoACK(ctx context.Context, pool *pgxpool.Pool, f fixture, leaf tls.Certificate, ca *x509.CertPool, log *slog.Logger) error {
+	store, err := revocationdb.New(pool)
+	if err != nil {
+		return err
+	}
+	client, err := barrierhttp.NewClient(leaf, ca, map[string]string{
+		f.IAM.RuntimeID: "https://127.0.0.2:9443", f.Processing.RuntimeID: "https://127.0.0.1:9443",
+	})
+	if err != nil {
+		return err
+	}
+	rt, err := revocation.New(store, tracedSender{client, log}, time.Now)
+	if err != nil {
+		return err
+	}
+	accepted, err := rt.Begin(ctx, revocation.Intent{ID: f.Revocation, ReasonCode: "TEST_REVOCATION",
+		Subject: startgate.Subject{Scope: startgate.Workspace, WorkspaceID: &f.Workspace, SubjectID: f.Workspace}})
+	if err != nil {
+		return err
+	}
+	pending, err := rt.Drive(ctx, accepted.ID)
+	if err != nil {
+		return err
+	}
+	log.Info("coordinator-pending", "status", pending.Status, "accepted_at", pending.AcceptedAt,
+		"deadline_at", pending.DeadlineAt, "effective", pending.EffectiveAt != nil, "missing", len(pending.MissingRuntimeIDs()))
+	if pending.Status != revocation.Pending || pending.EffectiveAt != nil || len(pending.MissingRuntimeIDs()) != 2 {
+		return errors.New("no-ACK fixture did not retain its mandatory participants as pending")
+	}
+	wait := time.NewTimer(time.Until(accepted.DeadlineAt) + 80*time.Millisecond)
+	defer wait.Stop()
+	select {
+	case <-ctx.Done():
+		return ctx.Err()
+	case <-wait.C:
+	}
+	final, err := rt.Drive(ctx, accepted.ID)
+	if err != nil {
+		return err
+	}
+	log.Info("coordinator-deadline", "status", final.Status, "effective", final.EffectiveAt != nil,
+		"deadline_breached", final.DeadlineBreached(), "missing", len(final.MissingRuntimeIDs()),
+		"observed_at", time.Now().UTC(), "elapsed_ms", time.Since(accepted.AcceptedAt).Milliseconds())
+	if final.Status != revocation.Incident || final.EffectiveAt != nil || !final.DeadlineBreached() || len(final.MissingRuntimeIDs()) != 2 {
+		return errors.New("missing ACK did not produce a deadline incident")
+	}
+	return os.WriteFile(filepath.Join(f.Dir, "incident-observed"), []byte("INCIDENT\n"), 0600)
+}
+
+func run(ctx context.Context, path string, log *slog.Logger) error {
+	raw, err := os.ReadFile(path)
+	var f fixture
+	if err != nil || json.Unmarshal(raw, &f) != nil {
+		return errors.New("read disposable fixture")
+	}
+	cert, err := tls.LoadX509KeyPair(f.Certificate, f.Key)
+	if err != nil {
+		return errors.New("load disposable IAM certificate")
+	}
+	clientCert, err := tls.LoadX509KeyPair(f.ClientCertificate, f.ClientKey)
+	if err != nil {
+		return errors.New("load disposable IAM client certificate")
+	}
+	ca := x509.NewCertPool()
+	if raw, err = os.ReadFile(f.CA); err != nil || !ca.AppendCertsFromPEM(raw) {
+		return errors.New("load disposable root")
+	}
+	pool, err := pgxpool.New(ctx, "host=/var/run/postgresql user=hp_hit146_iam_app dbname=hit146_iam sslmode=disable pool_max_conns=4")
+	if err != nil {
+		return err
+	}
+	defer pool.Close()
+	iam, err := composition.New(pool, clock{}, closedOwner{}, f.IAM.Registration, []runtimeaccess.Binding{f.IAM, f.Processing})
+	if err != nil {
+		return err
+	}
+	h, err := privatehttp.New(iam)
+	if err != nil {
+		return err
+	}
+	tlsConfig, err := privatehttp.TLSConfig(cert, ca, []runtimeaccess.Peer{
+		{DNSName: f.IAM.DNSName, CertificateFingerprint: f.IAM.CertificateFingerprint},
+		{DNSName: f.Processing.DNSName, CertificateFingerprint: f.Processing.CertificateFingerprint},
+	})
+	if err != nil {
+		return err
+	}
+	listener, err := net.Listen("tcp", "127.0.0.2:8443")
+	if err != nil {
+		return err
+	}
+	defer listener.Close()
+	server := &http.Server{Handler: trace(h, log), TLSConfig: tlsConfig, ReadHeaderTimeout: time.Second}
+	defer server.Close()
+	go server.ServeTLS(listener, "", "")
+	log.Info("iam-listening")
+	for {
+		if err = iam.Start(ctx); err == nil {
+			break
+		}
+		if ctx.Err() != nil {
+			return errors.New("local IAM failed to open after roster registration")
+		}
+		time.Sleep(40 * time.Millisecond)
+	}
+	log.Info("iam-gate-open")
+	for {
+		if _, err = os.Stat(filepath.Join(f.Dir, "revoke")); err == nil {
+			return observeNoACK(ctx, pool, f, clientCert, ca, log)
+		}
+		if ctx.Err() != nil {
+			return ctx.Err()
+		}
+		time.Sleep(40 * time.Millisecond)
+	}
+}
+
+func main() {
+	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
+	if len(os.Args) != 2 {
+		os.Exit(2)
+	}
+	ctx, cancel := context.WithTimeout(context.Background(), 36*time.Second)
+	defer cancel()
+	if err := run(ctx, os.Args[1], log); err != nil {
+		log.Error("iam-fixture", "error", err.Error())
+		os.Exit(1)
+	}
+}
```
