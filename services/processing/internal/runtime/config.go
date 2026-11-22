// Package runtime composes the existing Processing owners in a single process.
package runtime

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

var (
	ErrConfig      = errors.New("processing: invalid runtime configuration")
	ErrUnavailable = errors.New("processing: required dependency unavailable")
	commit         = regexp.MustCompile(`^[0-9a-f]{40}$`)
	dns            = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,252}$`)
	publicID       = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,255}$`)
)

type Config struct {
	Business    string   `json:"business_listen"`
	Control     string   `json:"control_listen"`
	Health      string   `json:"health_listen"`
	Identity    string   `json:"runtime_id"`
	Generation  uint64   `json:"generation"`
	OwnerDB     string   `json:"database_name"`
	AppDSN      string   `json:"application_dsn_file"`
	WorkerDSN   string   `json:"worker_dsn_file"`
	CursorKey   string   `json:"cursor_key_file"`
	Content     string   `json:"content_directory"`
	Connections int32    `json:"pool_connections"`
	TLS         TLSFiles `json:"tls"`
	IAM         Peer     `json:"iam"`
	Intake      Peer     `json:"intake"`
	Delivery    Peer     `json:"delivery"`
	BFFName     string   `json:"bff_name"`
	OPSName     string   `json:"ops_name"`

	RestoreCaseDeletion bool `json:"restore_case_deletion,omitempty"`
}

type TLSFiles struct {
	ServerCertificate string `json:"server_certificate"`
	ServerKey         string `json:"server_key"`
	ClientCertificate string `json:"client_certificate"`
	ClientKey         string `json:"client_key"`
	InboundCA         string `json:"inbound_ca"`
	OutboundCA        string `json:"outbound_ca"`
}

type Peer struct {
	Origin string `json:"origin"`
	Name   string `json:"name"`
}

type StandConfig struct {
	Workspaces []StandWorkspace `json:"workspaces"`
}

type StandWorkspace struct {
	ID                   string `json:"workspace_id"`
	Owner                string `json:"owner_user_id"`
	SessionID            string `json:"session_id"`
	SessionFile          string `json:"session_file"`
	CSRFFile             string `json:"csrf_file"`
	Source               string `json:"source_connection_id"`
	Destination          string `json:"destination_connection_id"`
	Account              string `json:"source_account_id"`
	DestinationWorkspace string `json:"destination_workspace_id"`
	Mode                 string `json:"mode"`
	PriceAlias           string `json:"price_alias"`
	ListAlias            string `json:"list_alias"`
	AssigneeAlias        string `json:"assignee_alias"`
	Price                string `json:"price_id"`
	List                 string `json:"list_id"`
	Assignee             string `json:"assignee_id"`
	Label                string `json:"service_label"`
	QueueSnapshot        string `json:"queue_snapshot_file"`
	QueueLock            string `json:"queue_lock_file"`
}

func readJSON(path string, dst any) error {
	if !filepath.IsAbs(path) {
		return ErrConfig
	}
	f, err := os.Open(path)
	if err != nil {
		return ErrConfig
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 256<<10+1))
	if err != nil || len(b) > 256<<10 {
		return ErrConfig
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil || d.Decode(new(any)) != io.EOF {
		return ErrConfig
	}
	return nil
}

func LoadConfig(path string) (Config, error) {
	var c Config
	if err := readJSON(path, &c); err != nil {
		return c, err
	}
	return c, c.validate()
}

func (c Config) validate() error {
	if !drafts.UUID(c.Identity) || c.Generation < 1 || c.Generation > 9007199254740991 ||
		c.Connections < 2 || c.Connections > 10 || !dns.MatchString(c.OwnerDB) || c.OwnerDB == "postgres" ||
		c.AppDSN == c.WorkerDSN {
		return ErrConfig
	}
	for _, pair := range [][2]string{{c.Business, "8443"}, {c.Control, "9443"}, {c.Health, "9444"}} {
		host, port, err := net.SplitHostPort(pair[0])
		ip := net.ParseIP(host)
		if err != nil || port != pair[1] || ip == nil || !ip.IsPrivate() && !ip.IsLoopback() {
			return ErrConfig
		}
	}
	names := map[string]bool{}
	for _, name := range []string{c.IAM.Name, c.Delivery.Name, c.Intake.Name, c.BFFName, c.OPSName} {
		if !dns.MatchString(name) || names[name] {
			return ErrConfig
		}
		names[name] = true
	}
	origins := map[string]bool{}
	for _, p := range []Peer{c.IAM, c.Delivery, c.Intake} {
		u, err := url.Parse(p.Origin)
		if err != nil || u.Scheme != "https" || u.Port() != "8443" || u.Hostname() == "" || u.User != nil || u.Path != "" ||
			u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || origins[u.Host] {
			return ErrConfig
		}
		origins[u.Host] = true
	}
	for _, p := range []string{c.AppDSN, c.WorkerDSN, c.Content, c.CursorKey, c.TLS.ServerCertificate, c.TLS.ServerKey,
		c.TLS.ClientCertificate, c.TLS.ClientKey, c.TLS.InboundCA, c.TLS.OutboundCA} {
		if !filepath.IsAbs(p) {
			return ErrConfig
		}
	}
	if c.TLS.ServerKey == c.TLS.ClientKey || c.TLS.ServerCertificate == c.TLS.ClientCertificate {
		return ErrConfig
	}
	return nil
}

// readSecret refuses shared permissions and symlinks. Errors never carry the
// path or the contents into process logs.
func readSecret(path string, max int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !filepath.IsAbs(path) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > int64(max) || info.Size() == 0 {
		return nil, ErrConfig
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrConfig
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, ErrConfig
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil || len(b) > max {
		return nil, ErrConfig
	}
	return b, nil
}

func caFile(path string) (*x509.CertPool, error) {
	b, err := os.ReadFile(path)
	ca := x509.NewCertPool()
	if err != nil || len(b) > 1<<20 || !ca.AppendCertsFromPEM(b) {
		return nil, ErrConfig
	}
	return ca, nil
}

func keyPair(certPath, keyPath string) (tls.Certificate, error) {
	var zero tls.Certificate
	key, err := readSecret(keyPath, 64<<10)
	if err != nil {
		return zero, err
	}
	cert, err := os.ReadFile(certPath)
	if err != nil || len(cert) > 1<<20 {
		return zero, ErrConfig
	}
	p, err := tls.X509KeyPair(cert, key)
	if err != nil || p.Leaf == nil || time.Now().Before(p.Leaf.NotBefore) || !time.Now().Before(p.Leaf.NotAfter) {
		return zero, ErrConfig
	}
	return p, nil
}

type installed struct {
	config   StandWorkspace
	token    string
	csrfHash [sha256.Size]byte
}

func loadStand(path string, c Config) (map[string]installed, error) {
	out := map[string]installed{}
	if path == "" {
		return out, nil
	}
	for _, addr := range []string{c.Business, c.Control, c.Health, c.IAM.Origin, c.Intake.Origin, c.Delivery.Origin} {
		host, _, _ := net.SplitHostPort(strings.TrimPrefix(addr, "https://"))
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return nil, ErrConfig
		}
	}
	var s StandConfig
	if err := readJSON(path, &s); err != nil || len(s.Workspaces) < 1 || len(s.Workspaces) > 5 {
		return nil, ErrConfig
	}
	for _, w := range s.Workspaces {
		if _, ok := out[w.ID]; ok || !drafts.UUID(w.ID) || !drafts.UUID(w.Owner) || !drafts.UUID(w.SessionID) ||
			!drafts.UUID(w.Source) || !drafts.UUID(w.Destination) || w.Source == w.Destination || w.Mode != "TEST" ||
			!publicID.MatchString(w.Account) || !publicID.MatchString(w.DestinationWorkspace) ||
			!publicID.MatchString(w.PriceAlias) || !publicID.MatchString(w.Price) ||
			!publicID.MatchString(w.ListAlias) || !publicID.MatchString(w.List) ||
			!publicID.MatchString(w.AssigneeAlias) || !publicID.MatchString(w.Assignee) ||
			len([]rune(w.Label)) < 1 || len([]rune(w.Label)) > 120 ||
			!filepath.IsAbs(w.QueueSnapshot) || !filepath.IsAbs(w.QueueLock) || w.QueueSnapshot == w.QueueLock {
			return nil, ErrConfig
		}
		token, err := readSecret(w.SessionFile, 256)
		if err != nil {
			return nil, err
		}
		csrf, err := readSecret(w.CSRFFile, 256)
		t, k := strings.TrimSpace(string(token)), strings.TrimSpace(string(csrf))
		if err != nil || len(t) < 43 || len(t) > 128 || len(k) < 32 || len(k) > 128 || strings.ContainsAny(t, "\r\n\t ") || strings.ContainsAny(k, "\r\n\t ") {
			return nil, ErrConfig
		}
		out[w.ID] = installed{config: w, token: t, csrfHash: sha256.Sum256([]byte(k))}
	}
	return out, nil
}
