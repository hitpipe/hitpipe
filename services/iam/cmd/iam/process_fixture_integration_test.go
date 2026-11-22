//go:build integration && linux

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	businessAddr = "127.0.0.138:8443"
	opsAddr      = "127.0.0.138:9444"
)

type processCA struct {
	cert  *x509.Certificate
	key   *ecdsa.PrivateKey
	roots *x509.CertPool
}

type processPeer struct {
	dns string
	tls tls.Certificate
}

func makeProcessCA(t *testing.T) processCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("generate fixture CA key")
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal("create fixture CA")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal("parse fixture CA")
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return processCA{cert: cert, key: key, roots: roots}
}

func (ca processCA) leaf(t *testing.T, dns string, server bool) processPeer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("generate fixture leaf key")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal("generate fixture leaf serial")
	}
	now := time.Now()
	leaf := &x509.Certificate{SerialNumber: serial, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		DNSNames: []string{dns}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.138"), net.ParseIP("127.0.0.1")}
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal("create fixture leaf")
	}
	return processPeer{dns: dns, tls: tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key}}
}

func (p processPeer) binding() map[string]any {
	return map[string]any{"dns_name": p.dns, "certificate_fingerprint": p.fingerprint()}
}

func (p processPeer) fingerprint() string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(p.tls.Certificate[0]))
}

func (p processPeer) runtime(service string) runtimeaccess.Binding {
	return runtimeaccess.Binding{DNSName: p.dns, Registration: runtimeaccess.Registration{
		Service: service, RuntimeID: processID(), BootID: processID(), Generation: 1, CertificateFingerprint: p.fingerprint(),
	}}
}

func processID() string {
	var v [16]byte
	_, _ = rand.Read(v[:])
	v[6], v[8] = (v[6]&15)|64, (v[8]&63)|128
	return fmt.Sprintf("%x-%x-%x-%x-%x", v[:4], v[4:6], v[6:8], v[8:10], v[10:])
}

func processSecret() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func fixtureFile(t *testing.T, dir, filename string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal("write owner-only fixture file")
	}
	return path
}

func fixtureJSON(t *testing.T, dir, filename string, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal("marshal private fixture")
	}
	return fixtureFile(t, dir, filename, body)
}

func openProcessDB(t *testing.T) (*pgxpool.Pool, string, string) {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" && appDSN == "" {
		t.Skip("run the isolated PostgreSQL 18.7 IAM integration driver")
	}
	adminCfg, adErr := pgxpool.ParseConfig(adminDSN)
	appCfg, apErr := pgxpool.ParseConfig(appDSN)
	if adErr != nil || apErr != nil || adminDSN == "" || appDSN == "" ||
		adminCfg.ConnConfig.Host != "127.0.0.1" || appCfg.ConnConfig.Host != "127.0.0.1" ||
		adminCfg.ConnConfig.User != "postgres" || appCfg.ConnConfig.User != "hp_hit81_app" ||
		!strings.HasPrefix(appCfg.ConnConfig.Database, "hit68_test_") ||
		adminCfg.ConnConfig.Database != appCfg.ConnConfig.Database || adminCfg.ConnConfig.Port != appCfg.ConnConfig.Port {
		t.Fatal("HIT-138 requires two distinct roles on the same disposable loopback PostgreSQL database")
	}
	adminCfg.MaxConns = 2
	admin, err := pgxpool.NewWithConfig(t.Context(), adminCfg)
	if err != nil {
		t.Fatal("open PostgreSQL fixture setup pool")
	}
	t.Cleanup(admin.Close)
	var version string
	if err := admin.QueryRow(t.Context(), "SHOW server_version_num").Scan(&version); err != nil || version != "180007" {
		t.Fatal("actual fixture database is not PostgreSQL 18.7")
	}
	return admin, appDSN, appCfg.ConnConfig.Database
}

type processFixture struct {
	ca                    processCA
	server, edge, ops     processPeer
	iam, bff, ai, unknown processPeer
	foreign               processPeer
	local, caller, agent  runtimeaccess.Binding
	dir                   string
	idp                   *processIDP
	profile               string
	principal, session    string
	workspace, resource   string
	opaque                string
}

func newProcessFixture(t *testing.T, db *pgxpool.Pool) *processFixture {
	t.Helper()
	ca := makeProcessCA(t)
	f := &processFixture{
		ca: ca, server: ca.leaf(t, "server.hit138.test", true), edge: ca.leaf(t, "edge.hit138.test", false),
		ops: ca.leaf(t, "ops.hit138.test", false), iam: ca.leaf(t, "iam.hit138.test", false),
		bff: ca.leaf(t, "bff.hit138.test", false), ai: ca.leaf(t, "ai.hit138.test", false),
		unknown: ca.leaf(t, "bff.hit138.test", false),
		foreign: makeProcessCA(t).leaf(t, "ops.hit138.test", false), dir: t.TempDir(),
		principal: processID(), session: processID(), workspace: processID(), resource: processID(), opaque: processSecret(),
	}
	f.local, f.caller, f.agent = f.iam.runtime("IAM"), f.bff.runtime("BFF"), f.ai.runtime("AI")
	f.idp = newProcessIDP(t, f.server.tls, f.principal)
	f.profile = filepath.Join(f.dir, "profiles")
	if err := os.Mkdir(f.profile, 0700); err != nil {
		t.Fatal("create private fixture profile directory")
	}
	nameRef := processID()
	profile := []byte(fmt.Sprintf(`{"user_id":%q,"display_name":"IAM process test","timezone":"Europe/Moscow","revision":4}`, f.principal))
	fixtureFile(t, f.profile, nameRef+".json", profile)
	now := time.Now().UTC().Truncate(time.Microsecond)
	digest := sha256.Sum256([]byte(f.opaque))
	csrf := sha256.Sum256([]byte(processSecret()))
	t.Cleanup(func() {
		processSQL(t, db, "DELETE FROM hp_iam.revocations WHERE subject_id=$1", f.session)
		processSQL(t, db, "DELETE FROM hp_iam.memberships WHERE principal_id=$1", f.principal)
		processSQL(t, db, "DELETE FROM hp_iam.workspaces WHERE workspace_id=$1", f.workspace)
		processSQL(t, db, "DELETE FROM hp_iam.sessions WHERE principal_id=$1", f.principal)
		processSQL(t, db, "DELETE FROM hp_iam.principals WHERE principal_id=$1", f.principal)
		processSQL(t, db, "DELETE FROM hp_iam.runtime_registry WHERE runtime_id IN($1,$2,$3)", f.local.RuntimeID, f.caller.RuntimeID, f.agent.RuntimeID)
	})
	processSQL(t, db, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,profile_content_ref,profile_digest,created_at)
 VALUES($1,$2,$3,$4,$5,$6)`, f.principal, f.idp.issuer, f.principal, nameRef, fmt.Sprintf("sha256:%x", sha256.Sum256(profile)), now.Add(-time.Minute))
	processSQL(t, db, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
 VALUES($1,$2,$3,$4,$5,$6,$7,3)`, f.session, digest[:], f.principal, csrf[:], now.Add(-time.Minute), now.Add(time.Hour), now.Add(30*time.Minute))
	processSQL(t, db, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
 VALUES($1,$2,1,7,'ACTIVE',$3,$3)`, f.workspace, processID(), now)
	processSQL(t, db, `INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at)
 VALUES($1,$2,'OWNER','ACTIVE',1,$3)`, f.workspace, f.principal, now)
	return f
}

func processSQL(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := db.Exec(ctx, query, args...); err != nil {
		t.Fatalf("fixture SQL failed for statement sha256:%x", sha256.Sum256([]byte(query)))
	}
}

func (f *processFixture) config(t *testing.T, filename, dsn, revision string, invalidTLS bool) string {
	t.Helper()
	certFile := fixtureFile(t, f.dir, "server.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.tls.Certificate[0]}))
	key, err := x509.MarshalPKCS8PrivateKey(f.server.tls.PrivateKey)
	if err != nil {
		t.Fatal("encode synthetic TLS private key")
	}
	keyFile := fixtureFile(t, f.dir, "server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	caFile := fixtureFile(t, f.dir, "fixture-ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.ca.cert.Raw}))
	if invalidTLS {
		certFile = fixtureFile(t, f.dir, "invalid.crt", []byte("malformed fixture certificate"))
	}
	listener := func(address string) map[string]any {
		return map[string]any{"address": address, "certificate_file": certFile, "key_file": keyFile, "client_ca_file": caFile}
	}
	business, ops := listener(businessAddr), listener(opsAddr)
	business["edge_peers"], ops["peers"] = []map[string]any{f.edge.binding()}, []map[string]any{f.ops.binding()}
	return fixtureJSON(t, f.dir, filename, map[string]any{
		"business": business, "ops": ops, "build_revision": revision, "profile_root": f.profile,
		"postgres": map[string]any{"dsn_file": fixtureFile(t, f.dir, filename+".dsn", []byte(dsn))},
		"runtime":  map[string]any{"local": f.local.Registration, "peers": []runtimeaccess.Binding{f.local, f.caller, f.agent}},
		"oidc": map[string]any{
			"issuer": f.idp.issuer, "authorization_endpoint": f.idp.issuer + "/authorize", "token_endpoint": f.idp.issuer + "/token",
			"jwks_uri": f.idp.issuer + "/keys", "redirect_uri": processCallback,
			"client_id": processClientID, "client_secret_file": fixtureFile(t, f.dir, "idp.secret", []byte(f.idp.secret)),
			"ca_file": caFile, "success_path": "/", "session_absolute_lifetime": "1h", "session_idle_lifetime": "15m",
		},
	})
}
