//go:build integration && linux

package casedeletion_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	httpreg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/casedeletion"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	labEnv         = "test"
	processingPeer = "processing-lab"
	syntheticText  = "HIT154_SYNTHETIC_SUBORDINATE_CONTENT"
)

type fixtureDB struct {
	admin, iam                   *pgxpool.Pool
	owners, workspaces, sessions [2]string
	session                      [2]string
}

func fixtureUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func openFixtureDB(t *testing.T, now time.Time) fixtureDB {
	t.Helper()
	adminDSN, iamDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || iamDSN == "" {
		t.Skip("run the isolated PostgreSQL driver for the real HIT-154 qualification")
	}
	adminCfg, err1 := pgxpool.ParseConfig(adminDSN)
	iamCfg, err2 := pgxpool.ParseConfig(iamDSN)
	if err1 != nil || err2 != nil {
		t.Fatal("synthetic database configuration rejected")
	}
	for _, cfg := range []*pgxpool.Config{adminCfg, iamCfg} {
		c := cfg.ConnConfig
		if c.Host != "127.0.0.1" || !strings.HasPrefix(c.Database, "hit68_test_") ||
			c.Password != "" || c.TLSConfig != nil || len(c.Fallbacks) != 0 {
			t.Fatal("refusing any database outside the existing isolated fixture")
		}
		cfg.MinConns, cfg.MaxConns = 0, 3
	}
	a, b := adminCfg.ConnConfig, iamCfg.ConnConfig
	if a.Port != b.Port || a.Database != b.Database || a.User != "postgres" || b.User != "hp_hit81_app" {
		t.Fatal("synthetic roles and database identity do not match")
	}
	f := fixtureDB{}
	f.admin, err1 = pgxpool.NewWithConfig(t.Context(), adminCfg)
	f.iam, err2 = pgxpool.NewWithConfig(t.Context(), iamCfg)
	if err1 != nil || err2 != nil {
		t.Fatal("cannot connect to isolated role pools")
	}
	t.Cleanup(f.admin.Close)
	t.Cleanup(f.iam.Close)
	var version string
	if err := f.admin.QueryRow(t.Context(), "SHOW server_version").Scan(&version); err != nil || version != "18.7" {
		t.Fatal("pinned PostgreSQL 18.7 is required")
	}
	for i := range f.workspaces {
		f.owners[i], f.workspaces[i], f.sessions[i] = fixtureUUID(), fixtureUUID(), fixtureUUID()
		f.session[i] = "HIT154-LAB-SESSION-" + fixtureUUID()
	}
	t.Cleanup(func() {
		for i := range f.workspaces {
			f.exec(t, "DELETE FROM hp_iam.memberships WHERE workspace_id=$1 AND principal_id=$2", f.workspaces[i], f.owners[i])
			f.exec(t, "DELETE FROM hp_iam.sessions WHERE session_id=$1", f.sessions[i])
			f.exec(t, "DELETE FROM hp_iam.workspaces WHERE workspace_id=$1", f.workspaces[i])
			f.exec(t, "DELETE FROM hp_iam.principals WHERE principal_id=$1", f.owners[i])
		}
	})
	for i, id := range f.workspaces {
		owner := f.owners[i]
		f.exec(t, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
			VALUES($1::uuid,'https://fixture.invalid',$1::text,$2)`, owner, now.Add(-time.Hour))
		sessionHash, csrfHash := sha256.Sum256([]byte(f.session[i])), sha256.Sum256([]byte(fixtureUUID()))
		f.exec(t, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
			VALUES($1,$2,$3,$4,$5,$6,$7,3)`, f.sessions[i], sessionHash[:], owner, csrfHash[:], now.Add(-time.Minute), now.Add(time.Hour), now.Add(30*time.Minute))
		f.exec(t, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
			VALUES($1,$2,2,7,'ACTIVE',$3,$3)`, id, fixtureUUID(), now)
		f.exec(t, `INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at)
			VALUES($1,$2,'OWNER','ACTIVE',5,$3)`, id, owner, now)
	}
	t.Logf("postgres=%s role=hp_hit81_app isolated_workspaces=%d", version, len(f.workspaces))
	return f
}

func (f fixtureDB) exec(t *testing.T, query string, params ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.admin.Exec(ctx, query, params...); err != nil {
		var pg *pgconn.PgError
		state := "unavailable"
		if errors.As(err, &pg) {
			state = pg.Code
		}
		t.Fatalf("synthetic SQL fixture failed (SQLSTATE=%s query_sha256=%x)", state, sha256.Sum256([]byte(query)))
	}
}

func (f fixtureDB) role(t *testing.T, index int, role string) {
	t.Helper()
	f.exec(t, `UPDATE hp_iam.memberships SET role=$3,revision=revision+1
		WHERE workspace_id=$1 AND principal_id=$2`, f.workspaces[index], f.owners[index], role)
}

type fixtureClock struct{ t time.Time }

func (c fixtureClock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: c.t, Trusted: true}, ctx.Err()
}

type caseItem struct {
	WorkspaceID string
	CaseID      string
	Target      app.Target
	Detail      string
}

type storedApply struct {
	Receipt app.Receipt
	Scope   app.Scope
}

type copyState struct {
	Generation uint64
	Items      []caseItem
	Applied    []storedApply
}

// subordinateCopy is a synthetic proof adapter, not a Processing implementation.
// The test alone owns original details; none of its file bytes go to the publisher.
type subordinateCopy struct {
	mu  sync.Mutex
	dir string
}

func (s *subordinateCopy) path() string { return filepath.Join(s.dir, "synthetic-content.json") }

func (s *subordinateCopy) save(state copyState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.write(payload)
}

func (s *subordinateCopy) write(payload []byte) error {
	f, err := os.CreateTemp(s.dir, ".test-copy-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	n, writeErr := f.Write(payload)
	syncErr := f.Sync()
	closeErr := f.Close()
	if n != len(payload) || writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("could not persist synthetic content copy")
	}
	if err := os.Rename(f.Name(), s.path()); err != nil {
		return err
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *subordinateCopy) read() (copyState, error) {
	var state copyState
	raw, err := os.ReadFile(s.path())
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	return state, nil
}

func selected(item caseItem, scope app.Scope) bool {
	return item.WorkspaceID == scope.WorkspaceID && item.CaseID == scope.CaseID && slices.Contains(scope.Targets, item.Target)
}

func (s *subordinateCopy) WithExactTargets(ctx context.Context, scope app.Scope, commit func(context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read()
	if err != nil {
		return app.ErrUnavailable
	}
	for _, target := range scope.Targets {
		if !slices.ContainsFunc(state.Items, func(item caseItem) bool {
			return item.WorkspaceID == scope.WorkspaceID && item.CaseID == scope.CaseID && item.Target == target
		}) {
			return app.ErrUnauthorized
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return commit(ctx)
}

func (s *subordinateCopy) WithApplied(ctx context.Context, receipt app.Receipt, commit func(context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read()
	if err != nil {
		return app.ErrUnavailable
	}
	for _, proof := range state.Applied {
		if state.Generation == receipt.Generation && proof.Receipt == receipt &&
			!slices.ContainsFunc(state.Items, func(item caseItem) bool { return selected(item, proof.Scope) }) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return commit(ctx)
		}
	}
	return app.ErrUnauthorized
}

func fixtureReceipt(decision app.Decision, now time.Time) (app.Receipt, error) {
	proof, err := app.Digest(struct {
		ID         string
		Generation uint64
		Revision   uint64
	}{decision.DeletionID, decision.ApplyGeneration, decision.ApplyRevision})
	if err != nil {
		return app.Receipt{}, err
	}
	return app.Receipt{
		Environment: labEnv, WorkspaceID: decision.Scope.WorkspaceID, DeletionID: decision.DeletionID,
		ScopeDigest: decision.ScopeDigest, Generation: decision.ApplyGeneration, Revision: decision.ApplyRevision,
		AppliedAt: now, ApplicationRef: proof,
	}, nil
}

func (s *subordinateCopy) apply(decision app.Decision, now time.Time) (app.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read()
	if err != nil {
		return app.Receipt{}, err
	}
	if decision.Status != app.Applying || state.Generation != decision.ApplyGeneration || decision.ApplyingAt == nil {
		return app.Receipt{}, app.ErrConflict
	}
	receipt, err := fixtureReceipt(decision, now)
	if err != nil {
		return app.Receipt{}, err
	}
	for _, old := range state.Applied {
		if old.Receipt.DeletionID == decision.DeletionID && old.Receipt.Generation == decision.ApplyGeneration {
			return old.Receipt, nil
		}
	}
	state.Items = slices.DeleteFunc(state.Items, func(item caseItem) bool { return selected(item, decision.Scope) })
	state.Applied = append(state.Applied, storedApply{Receipt: receipt, Scope: decision.Scope})
	return receipt, s.save(state)
}

type labTLS struct {
	ca      *x509.CertPool
	publish tls.Certificate
	read    tls.Certificate
	other   tls.Certificate
}

func newLabTLS(t *testing.T) labTLS {
	t.Helper()
	now := time.Now()
	issuerPublic, issuerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal("issuer generation failed")
	}
	issuer := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, issuer, issuer, issuerPublic, issuerPrivate)
	if err != nil {
		t.Fatal("fixture issuer certificate generation failed")
	}
	trust, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal("fixture issuer certificate invalid")
	}
	ca := x509.NewCertPool()
	ca.AddCert(trust)
	issue := func(serial int64, dns string, usage x509.ExtKeyUsage) tls.Certificate {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal("fixture leaf key generation failed")
		}
		cert := &x509.Certificate{
			SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		if usage == x509.ExtKeyUsageServerAuth {
			cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		} else {
			cert.DNSNames = []string{dns}
		}
		encoded, err := x509.CreateCertificate(rand.Reader, cert, issuer, public, issuerPrivate)
		if err != nil {
			t.Fatal("fixture leaf certificate generation failed")
		}
		return tls.Certificate{Certificate: [][]byte{encoded}, PrivateKey: private}
	}
	return labTLS{
		ca: ca, publish: issue(2, "", x509.ExtKeyUsageServerAuth),
		read:  issue(3, "processing.test", x509.ExtKeyUsageClientAuth),
		other: issue(4, "processing.test", x509.ExtKeyUsageClientAuth),
	}
}

func pin(cert tls.Certificate) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(cert.Certificate[0]))
}

func (l labTLS) serve(t *testing.T, registry *app.Registry, workspaceIDs []string) (*httpreg.Client, *httpreg.Client, func()) {
	t.Helper()
	conf := []httpreg.Peer{{ID: processingPeer, Workspaces: workspaceIDs,
		Transport: runtimeaccess.Peer{DNSName: "processing.test", CertificateFingerprint: pin(l.read)}}}
	server, err := httpreg.NewServer(registry, l.publish, l.ca, conf)
	if err != nil {
		t.Fatal(err)
	}
	server.ErrorLog = log.New(io.Discard, "", 0)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("cannot bind isolated TLS listener")
	}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	var once sync.Once
	closeServer := func() {
		once.Do(func() {
			if err := server.Close(); err != nil {
				t.Errorf("close fixture listener: %v", err)
			}
			if err := <-done; !errors.Is(err, http.ErrServerClosed) {
				t.Errorf("fixture listener: %v", err)
			}
		})
	}
	t.Cleanup(closeServer)
	origin := "https://" + listener.Addr().String()
	client, err := httpreg.NewClient(origin, l.read, l.ca, pin(l.publish))
	if err != nil {
		t.Fatal(err)
	}
	unauthorized, err := httpreg.NewClient(origin, l.other, l.ca, pin(l.publish))
	if err != nil {
		t.Fatal(err)
	}
	return client, unauthorized, closeServer
}
