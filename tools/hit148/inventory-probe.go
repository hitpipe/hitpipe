//go:build hit148_probe && integration && linux

// Added to the pinned IAM test package using a Go overlay. The Processing
// executable makes its own authenticated request, outside the IAM process.
package casedeletion_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	filereg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/filestore/casedeletion"
	httpreg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/casedeletion"
	pgowner "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/casedeletion"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

func TestProcessingConsumesRealHIT154Inventory(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openFixtureDB(t, now)
	owner, err := pgowner.New(db.iam, fixtureClock{now})
	requireOK(t, err)
	workspace, neighbor := db.workspaces[0], db.workspaces[1]
	publisherDir, subordinateDir := filepath.Join(t.TempDir(), "authority"), filepath.Join(t.TempDir(), "copy")
	requireOK(t, os.Mkdir(publisherDir, 0700))
	requireOK(t, os.Mkdir(subordinateDir, 0700))
	subordinate := &subordinateCopy{dir: subordinateDir}
	// This is HIT-154's synthetic target authority. There is no deletion,
	// application proof or serving transition in the Processing verification.
	target := app.Target{Class: "HISTORY", VersionID: fixtureUUID()}
	scope := app.Scope{WorkspaceID: workspace, CaseID: fixtureUUID(), Targets: []app.Target{target}}
	requireOK(t, subordinate.save(copyState{Generation: 1, Items: []caseItem{{WorkspaceID: workspace, CaseID: scope.CaseID, Target: target, Detail: syntheticText}}}))
	initial, err := app.NewState(labEnv)
	requireOK(t, err)
	store, err := filereg.Initialize(publisherDir, initial)
	requireOK(t, err)
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	requireOK(t, err)
	publisherID := fixtureUUID()
	r, err := app.New(app.Config{Environment: labEnv, PublisherID: publisherID,
		ProcessingPeerID: processingPeer, SigningKey: private, Clock: func() time.Time { return now }}, store, owner, subordinate, subordinate)
	requireOK(t, err)
	for _, ws := range db.workspaces {
		requireOK(t, r.ProvisionWorkspace(t.Context(), ws, 1))
	}

	keys := newLabTLS(t)
	s, err := httpreg.NewServer(r, keys.publish, keys.ca, []httpreg.Peer{{ID: processingPeer, Workspaces: db.workspaces[:],
		Transport: runtimeaccess.Peer{DNSName: "processing.test", CertificateFingerprint: pin(keys.read)}}})
	requireOK(t, err)
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	requireOK(t, err)
	go s.ServeTLS(l, "", "")
	defer s.Close()

	dir := t.TempDir()
	file := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		requireOK(t, os.WriteFile(p, data, 0600))
		return p
	}
	pemFile := func(name, typ string, data []byte) string {
		return file(name, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: data}))
	}
	credential := func(name string, leaf tls.Certificate) (string, string) {
		private, err := x509.MarshalPKCS8PrivateKey(leaf.PrivateKey)
		requireOK(t, err)
		return pemFile(name+".pem", "CERTIFICATE", leaf.Certificate[0]), pemFile(name+".key", "PRIVATE KEY", private)
	}
	goodCert, goodKey := credential("trusted", keys.read)
	foreignCert, foreignKey := credential("untrusted", keys.other)
	publicDER, err := x509.MarshalPKIXPublicKey(pub)
	requireOK(t, err)
	foreignPub, _, err := ed25519.GenerateKey(rand.Reader)
	requireOK(t, err)
	foreignDER, err := x509.MarshalPKIXPublicKey(foreignPub)
	requireOK(t, err)
	ca := pemFile("ca.pem", "CERTIFICATE", keys.caDER)
	pubFile := pemFile("publisher.pem", "PUBLIC KEY", publicDER)
	badPubFile := pemFile("foreign-publisher.pem", "PUBLIC KEY", foreignDER)
	run := func(label, ws string, generation int, serverPin, cert, key, signingKey string, wantExit, wantCount int) {
		t.Helper()
		cfg, err := json.Marshal(map[string]any{
			"origin": "https://" + l.Addr().String(), "server_pin": serverPin, "publisher_id": publisherID,
			"workspace_id": ws, "generation": generation, "ca": ca, "client_certificate": cert,
			"client_key": key, "publisher_public_key": signingKey,
		})
		requireOK(t, err)
		cmd := exec.CommandContext(t.Context(), "/test/processing-purge", "--local-stand", "--verify-deletion-inventory", file(label+".json", cfg))
		out, err := cmd.CombinedOutput()
		exit := 0
		if err != nil {
			exited, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			exit = exited.ExitCode()
		}
		if exit != wantExit {
			t.Fatalf("%s: exit=%d want=%d output=%s", label, exit, wantExit, out)
		}
		if exit == 0 {
			var view struct {
				Workspace  string `json:"workspace_id"`
				Generation int    `json:"generation"`
				Revision   int    `json:"revision"`
				Count      int    `json:"decision_count"`
				Serving    string `json:"serving"`
			}
			if json.Unmarshal(out, &view) != nil || view.Workspace != ws || view.Generation != generation ||
				view.Revision < 1 || view.Count != wantCount || view.Serving != "CLOSED" {
				t.Fatalf("%s: wrong processing result %s", label, out)
			}
		}
		t.Logf("%s: exit=%d %s", label, exit, out)
	}
	serverPin := pin(keys.publish)
	run("signed-empty", workspace, 1, serverPin, goodCert, goodKey, pubFile, 0, 0)
	run("neighbor-empty", neighbor, 1, serverPin, goodCert, goodKey, pubFile, 0, 0)
	run("foreign-workspace", fixtureUUID(), 1, serverPin, goodCert, goodKey, pubFile, 1, 0)
	run("unknown-generation", workspace, 2, serverPin, goodCert, goodKey, pubFile, 1, 0)
	run("wrong-mTLS-client", workspace, 1, serverPin, foreignCert, foreignKey, pubFile, 1, 0)
	run("wrong-server-pin", workspace, 1, pin(keys.other), goodCert, goodKey, pubFile, 1, 0)
	run("wrong-signer", workspace, 1, serverPin, goodCert, goodKey, badPubFile, 1, 0)

	offer, err := r.Prepare(t.Context(), db.session[0], scope)
	requireOK(t, err)
	code, err := offer.ConfirmationCode()
	requireOK(t, err)
	decision, err := r.Confirm(t.Context(), db.session[0], offer, code)
	requireOK(t, err)
	_, err = r.StartApplying(t.Context(), workspace, decision.DeletionID, 1)
	requireOK(t, err)
	run("owner-confirmed-applying", workspace, 1, serverPin, goodCert, goodKey, pubFile, 0, 1)
	run("neighbor-stays-empty", neighbor, 1, serverPin, goodCert, goodKey, pubFile, 0, 0)
	requireOK(t, s.Close())
	run("no-current-source", workspace, 1, serverPin, goodCert, goodKey, pubFile, 1, 0)
}
