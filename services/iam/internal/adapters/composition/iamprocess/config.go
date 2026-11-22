package iamprocess

import (
	"crypto/tls"
	"crypto/x509"
	json "encoding/json/v2"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	transport "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/oidc"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errConfig     = errors.New("IAM_CONFIG_INVALID")
	errTLS        = errors.New("IAM_TLS_CONFIG_INVALID")
	errOIDCConfig = errors.New("IAM_OIDC_CONFIG_INVALID")
	buildSHA      = regexp.MustCompile("^[0-9a-f]{40}$")
)

const maxConfigFile = 256 << 10
const maxMaterialFile = 256 << 10
const maxSecretFile = 16 << 10

// Config holds the loaded provisioning settings for one IAM process.
// The command never logs this value or any of its fields.
type Config struct {
	BusinessAddress, OPSAddress, BuildRevision string
	BusinessTLS, OPSTLS                        *tls.Config
	Local                                      runtimeaccess.Registration
	RuntimeBindings                            []runtimeaccess.Binding
	EdgePeers, BFFPeers, OPSPeers              []runtimeaccess.Peer
	OIDC                                       oidc.Config
	SessionPolicy                              issuesession.Policy
	SuccessPath, ProfileRoot                   string
	pool                                       *pgxpool.Config
}

type peerInput struct {
	DNSName                string `json:"dns_name"`
	CertificateFingerprint string `json:"certificate_fingerprint"`
}

type tlsInput struct {
	Address         string `json:"address"`
	CertificateFile string `json:"certificate_file"`
	KeyFile         string `json:"key_file"`
	ClientCAFile    string `json:"client_ca_file"`
}

type oidcInput struct {
	Issuer                  string `json:"issuer"`
	AuthorizationEndpoint   string `json:"authorization_endpoint"`
	TokenEndpoint           string `json:"token_endpoint"`
	JWKSURI                 string `json:"jwks_uri"`
	RedirectURI             string `json:"redirect_uri"`
	ClientID                string `json:"client_id"`
	ClientSecretFile        string `json:"client_secret_file"`
	CAFile                  string `json:"ca_file"`
	SuccessPath             string `json:"success_path"`
	SessionAbsoluteLifetime string `json:"session_absolute_lifetime"`
	SessionIdleLifetime     string `json:"session_idle_lifetime"`
}

type configInput struct {
	Business struct {
		tlsInput
		EdgePeers []peerInput `json:"edge_peers"`
	} `json:"business"`
	OPS struct {
		tlsInput
		Peers []peerInput `json:"peers"`
	} `json:"ops"`
	Postgres databaseInput `json:"postgres"`
	OIDC     oidcInput     `json:"oidc"`
	Runtime  struct {
		Local runtimeaccess.Registration `json:"local"`
		Peers []runtimeaccess.Binding    `json:"peers"`
	} `json:"runtime"`
	ProfileRoot   string `json:"profile_root"`
	BuildRevision string `json:"build_revision"`
}

// LoadConfig rejects ambiguous JSON and missing provisioning before any
// listener or PostgreSQL connection is started.
func LoadConfig(path string) (Config, error) {
	data, err := readFile(path, maxConfigFile, false)
	if err != nil {
		return Config{}, errConfig
	}
	var raw configInput
	if err := json.Unmarshal(data, &raw, json.RejectUnknownMembers(true)); err != nil {
		return Config{}, errConfig
	}
	if !privateAddress(raw.Business.Address, "8443") || !privateAddress(raw.OPS.Address, "9444") ||
		!validFilePath(raw.ProfileRoot) || !buildSHA.MatchString(raw.BuildRevision) {
		return Config{}, errConfig
	}
	cfg := Config{
		BusinessAddress: raw.Business.Address, OPSAddress: raw.OPS.Address, BuildRevision: raw.BuildRevision,
		Local: raw.Runtime.Local.Normalized(), RuntimeBindings: raw.Runtime.Peers,
		ProfileRoot: raw.ProfileRoot, SuccessPath: raw.OIDC.SuccessPath,
	}
	businessPeers, err := cfg.assignPeers(raw.Business.EdgePeers, raw.OPS.Peers)
	if err != nil {
		return Config{}, err
	}
	cfg.BusinessTLS, err = raw.Business.loadTLS(businessPeers)
	if err != nil {
		return Config{}, err
	}
	cfg.OPSTLS, err = raw.OPS.loadTLS(cfg.OPSPeers)
	if err != nil {
		return Config{}, err
	}
	cfg.OIDC, cfg.SessionPolicy, err = raw.OIDC.load()
	if err != nil {
		return Config{}, err
	}
	cfg.pool, err = raw.Postgres.load()
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func privateAddress(address, expectedPort string) bool {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	return err == nil && port == expectedPort && ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func validFilePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && path != "/"
}

func readFile(path string, limit int64, secret bool) ([]byte, error) {
	if !validFilePath(path) {
		return nil, errConfig
	}
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errConfig
	}
	defer dir.Close()
	file, err := dir.OpenFile(filepath.Base(path), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errConfig
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1 || stat.Size() > limit ||
		(secret && stat.Mode().Perm()&0o077 != 0) {
		return nil, errConfig
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		return nil, errConfig
	}
	return data, nil
}

func (cfg *Config) assignPeers(edge, ops []peerInput) ([]runtimeaccess.Peer, error) {
	if !cfg.Local.Valid() || cfg.Local.Service != "IAM" || len(edge) == 0 || len(ops) == 0 || len(cfg.RuntimeBindings) == 0 {
		return nil, errTLS
	}
	fingerprints := make(map[string]struct{})
	add := func(name, fingerprint string) (runtimeaccess.Peer, bool) {
		if !runtimeaccess.ValidDNSName(name) || net.ParseIP(name) != nil || !runtimeaccess.ValidFingerprint(fingerprint) {
			return runtimeaccess.Peer{}, false
		}
		if _, used := fingerprints[fingerprint]; used {
			return runtimeaccess.Peer{}, false
		}
		fingerprints[fingerprint] = struct{}{}
		return runtimeaccess.Peer{DNSName: strings.ToLower(name), CertificateFingerprint: fingerprint}, true
	}
	all := make([]runtimeaccess.Peer, 0, len(cfg.RuntimeBindings)+len(edge))
	var containsLocal bool
	runtimeIDs := make(map[string]struct{})
	bootIDs := make(map[string]struct{})
	for i, binding := range cfg.RuntimeBindings {
		reg := binding.Normalized()
		peer, ok := add(binding.DNSName, reg.CertificateFingerprint)
		_, runtimeSeen := runtimeIDs[reg.RuntimeID]
		_, bootSeen := bootIDs[reg.BootID]
		if !ok || !reg.Valid() || runtimeSeen || bootSeen {
			return nil, errTLS
		}
		runtimeIDs[reg.RuntimeID], bootIDs[reg.BootID] = struct{}{}, struct{}{}
		cfg.RuntimeBindings[i] = runtimeaccess.Binding{Registration: reg, DNSName: peer.DNSName}
		all = append(all, peer)
		containsLocal = containsLocal || reg == cfg.Local
		if reg.Service == "BFF" {
			cfg.BFFPeers = append(cfg.BFFPeers, peer)
		}
	}
	if !containsLocal || len(cfg.BFFPeers) == 0 {
		return nil, errTLS
	}
	for _, input := range edge {
		peer, ok := add(input.DNSName, input.CertificateFingerprint)
		if !ok {
			return nil, errTLS
		}
		all = append(all, peer)
		cfg.EdgePeers = append(cfg.EdgePeers, peer)
	}
	for _, input := range ops {
		peer, ok := add(input.DNSName, input.CertificateFingerprint)
		if !ok {
			return nil, errTLS
		}
		cfg.OPSPeers = append(cfg.OPSPeers, peer)
	}
	return all, nil
}

func (input tlsInput) loadTLS(peers []runtimeaccess.Peer) (*tls.Config, error) {
	certPEM, err := readFile(input.CertificateFile, maxMaterialFile, false)
	if err != nil {
		return nil, errTLS
	}
	keyPEM, err := readFile(input.KeyFile, maxMaterialFile, true)
	if err != nil {
		return nil, errTLS
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || cert.Leaf == nil || cert.Leaf.IsCA || !hasServerUsage(cert.Leaf) {
		return nil, errTLS
	}
	now := time.Now()
	for _, raw := range cert.Certificate {
		entry, err := x509.ParseCertificate(raw)
		if err != nil || now.Before(entry.NotBefore) || !now.Before(entry.NotAfter) {
			return nil, errTLS
		}
	}
	roots, err := loadCA(input.ClientCAFile)
	if err != nil {
		return nil, errTLS
	}
	config, err := transport.TLSConfig(cert, roots, peers)
	if err != nil {
		return nil, errTLS
	}
	return config, nil
}

func hasServerUsage(cert *x509.Certificate) bool {
	if len(cert.ExtKeyUsage) == 0 && len(cert.UnknownExtKeyUsage) == 0 {
		return true
	}
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}

func loadCA(path string) (*x509.CertPool, error) {
	data, err := readFile(path, maxMaterialFile, false)
	if err != nil {
		return nil, errTLS
	}
	now := time.Now()
	roots := x509.NewCertPool()
	for strings.TrimSpace(string(data)) != "" {
		block, remainder := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errTLS
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !certificate.IsCA || now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			return nil, errTLS
		}
		roots.AddCert(certificate)
		data = remainder
	}
	if len(roots.Subjects()) == 0 {
		return nil, errTLS
	}
	return roots, nil
}

func (raw oidcInput) load() (oidc.Config, issuesession.Policy, error) {
	secret, err := readFile(raw.ClientSecretFile, maxSecretFile, true)
	if err != nil {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	clientSecret := strings.TrimSuffix(strings.TrimSuffix(string(secret), "\n"), "\r")
	if clientSecret == "" || strings.ContainsAny(clientSecret, "\x00\r\n") {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	absolute, err := time.ParseDuration(raw.SessionAbsoluteLifetime)
	if err != nil {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	idle, err := time.ParseDuration(raw.SessionIdleLifetime)
	if err != nil || idle <= 0 || idle > absolute || absolute > sessionpolicy.MaxAbsoluteLifetime {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	roots, err := loadCA(raw.CAFile)
	if err != nil {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	callback, err := url.Parse(raw.RedirectURI)
	if err != nil || callback.EscapedPath() != "/auth/callback" {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	config := oidc.Config{
		Issuer: raw.Issuer, AuthorizationEndpoint: raw.AuthorizationEndpoint, TokenEndpoint: raw.TokenEndpoint,
		JWKSURI: raw.JWKSURI, RedirectURI: raw.RedirectURI, ClientID: raw.ClientID, ClientSecret: clientSecret, RootCAs: roots,
	}
	if _, err := oidc.New(config); err != nil {
		return oidc.Config{}, issuesession.Policy{}, errOIDCConfig
	}
	return config, issuesession.Policy{AbsoluteLifetime: absolute, IdleLifetime: idle}, nil
}
