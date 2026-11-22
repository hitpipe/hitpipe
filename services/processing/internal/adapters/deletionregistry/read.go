// Package deletionregistry reads the pinned IAM case-content deletion inventory.
// Freshness at serving requires IAM's separate local WithCurrent barrier.
package deletionregistry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

const (
	snapshotPath    = "/internal/maintenance/v1/case-deletion/snapshot"
	signatureDomain = "HITPIPE-CASE-DELETION/snapshot-v1\x00"
	maxResponse     = 4 << 20
	maxInteger      = 9007199254740991
)

var (
	ErrUnavailable = errors.New("trusted case deletion inventory unavailable")
	digestPattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Target struct {
	Class     string `json:"class"`
	VersionID string `json:"version_id"`
}

type Scope struct {
	WorkspaceID string   `json:"workspace_id"`
	CaseID      string   `json:"case_id"`
	Targets     []Target `json:"targets"`
}

type Receipt struct {
	Environment    string    `json:"environment"`
	WorkspaceID    string    `json:"workspace_id"`
	DeletionID     string    `json:"deletion_id"`
	ScopeDigest    string    `json:"scope_digest"`
	Generation     uint64    `json:"generation"`
	Revision       uint64    `json:"revision"`
	AppliedAt      time.Time `json:"applied_at"`
	ApplicationRef string    `json:"application_ref"`
}

type Decision struct {
	DeletionID      string     `json:"deletion_id"`
	Action          string     `json:"action"`
	PublisherID     string     `json:"publisher_id"`
	Scope           Scope      `json:"scope"`
	ScopeDigest     string     `json:"scope_digest"`
	ConfirmedAt     time.Time  `json:"confirmed_at"`
	ConfirmationRef string     `json:"confirmation_ref"`
	Status          string     `json:"status"`
	ApplyGeneration uint64     `json:"apply_generation"`
	ApplyRevision   uint64     `json:"apply_revision"`
	ApplyingAt      *time.Time `json:"applying_at"`
	Receipts        []Receipt  `json:"receipts"`
}

type Snapshot struct {
	SchemaVersion int        `json:"schema_version"`
	Environment   string     `json:"environment"`
	PublisherID   string     `json:"publisher_id"`
	WorkspaceID   string     `json:"workspace_id"`
	Generation    uint64     `json:"generation"`
	Revision      uint64     `json:"revision"`
	Full          bool       `json:"full"`
	Decisions     []Decision `json:"decisions"`
}

// Observation is the validated identity and revision from a live pinned
// request. It authorizes neither serving nor changes to Processing storage.
type Observation struct {
	WorkspaceID string `json:"workspace_id"`
	Generation  uint64 `json:"generation"`
	Revision    uint64 `json:"revision"`
	Decisions   int    `json:"decision_count"`
}

type Provision struct {
	Origin      string
	ServerPin   string
	PublisherID string
	WorkspaceID string
	Generation  uint64
}

type Client struct {
	endpoint  url.URL
	provision Provision
	key       ed25519.PublicKey
	http      *http.Client
}

func New(p Provision, roots *x509.CertPool, leaf tls.Certificate, key ed25519.PublicKey) (*Client, error) {
	u, err := url.Parse(p.Origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Opaque != "" || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || !digestPattern.MatchString(p.ServerPin) || !id(p.PublisherID) ||
		!id(p.WorkspaceID) || !version(p.Generation) || roots == nil || len(leaf.Certificate) == 0 ||
		leaf.PrivateKey == nil || len(key) != ed25519.PublicKeySize {
		return nil, ErrUnavailable
	}
	port, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil || port == 0 {
		return nil, ErrUnavailable
	}
	u.Path = snapshotPath
	u.RawQuery = url.Values{"workspace_id": {p.WorkspaceID}, "generation": {strconv.FormatUint(p.Generation, 10)}}.Encode()
	dial := net.Dialer{Timeout: 2 * time.Second}
	t := &http.Transport{
		DialContext: dial.DialContext, DisableKeepAlives: true, DisableCompression: true,
		TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
		MaxResponseHeaderBytes: 16 << 10,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
			RootCAs: roots.Clone(), Certificates: []tls.Certificate{leaf},
			VerifyConnection: func(s tls.ConnectionState) error {
				if len(s.VerifiedChains) == 0 || len(s.PeerCertificates) == 0 ||
					digest(s.PeerCertificates[0].Raw) != p.ServerPin {
					return ErrUnavailable
				}
				return nil
			},
		},
	}
	return &Client{endpoint: *u, provision: p, key: bytes.Clone(key), http: &http.Client{
		Transport: t, Timeout: 4 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) Observe(ctx context.Context) (Observation, error) {
	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		return Observation{}, err
	}
	return Observation{snapshot.WorkspaceID, snapshot.Generation, snapshot.Revision, len(snapshot.Decisions)}, nil
}

func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return Snapshot{}, ErrUnavailable
	}
	q, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint.String(), nil)
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	q.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(q)
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	defer resp.Body.Close()
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || resp.StatusCode != http.StatusOK ||
		resp.Header.Get("Content-Encoding") != "" || resp.ContentLength > maxResponse {
		return Snapshot{}, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse || ctx.Err() != nil {
		return Snapshot{}, ErrUnavailable
	}
	return c.verify(raw)
}

func (c *Client) verify(raw []byte) (Snapshot, error) {
	var envelope struct {
		Payload   json.RawMessage `json:"payload"`
		Signature []byte          `json:"signature"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Signature) != ed25519.SignatureSize {
		return Snapshot{}, ErrUnavailable
	}
	// The publisher signs its canonical Go JSON bytes. Exact re-encoding
	// rejects repeat keys, folded names, null substitutions and extra fields.
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil || !bytes.Equal(bytes.TrimSuffix(raw, []byte("\n")), envelopeJSON) ||
		!ed25519.Verify(c.key, append([]byte(signatureDomain), envelope.Payload...), envelope.Signature) {
		return Snapshot{}, ErrUnavailable
	}
	var snapshot Snapshot
	if err := json.Unmarshal(envelope.Payload, &snapshot); err != nil {
		return Snapshot{}, ErrUnavailable
	}
	canonical, err := json.Marshal(snapshot)
	p := c.provision
	if err != nil || !bytes.Equal(envelope.Payload, canonical) ||
		snapshot.SchemaVersion != 1 || snapshot.Environment != "test" ||
		snapshot.PublisherID != p.PublisherID || snapshot.WorkspaceID != p.WorkspaceID ||
		snapshot.Generation != p.Generation || !version(snapshot.Revision) ||
		!snapshot.Full || snapshot.Decisions == nil || len(snapshot.Decisions) > 4096 {
		return Snapshot{}, ErrUnavailable
	}
	var previous string
	for _, d := range snapshot.Decisions {
		if d.DeletionID <= previous || !d.valid(snapshot) {
			return Snapshot{}, ErrUnavailable
		}
		previous = d.DeletionID
	}
	return snapshot, nil
}

func (d Decision) valid(p Snapshot) bool {
	if !id(d.DeletionID) || d.Action != "CASE_CONTENT_DELETE" || d.PublisherID != p.PublisherID ||
		d.Scope.WorkspaceID != p.WorkspaceID || !id(d.Scope.CaseID) ||
		len(d.Scope.Targets) == 0 || len(d.Scope.Targets) > 128 ||
		!digestPattern.MatchString(d.ConfirmationRef) || d.ConfirmedAt.IsZero() || d.Receipts == nil {
		return false
	}
	var previous string
	for _, t := range d.Scope.Targets {
		key := t.Class + "/" + t.VersionID
		if key <= previous || !id(t.VersionID) || (t.Class != "CASE_CONTEXT" && t.Class != "HISTORY") {
			return false
		}
		previous = key
	}
	fields, err := json.Marshal(struct {
		Action string `json:"action"`
		Scope  Scope  `json:"scope"`
	}{d.Action, d.Scope})
	if err != nil || digest(fields) != d.ScopeDigest {
		return false
	}
	switch d.Status {
	case "CONFIRMED", "CANCELLED":
		return d.ApplyGeneration == 0 && d.ApplyRevision == 0 && d.ApplyingAt == nil && len(d.Receipts) == 0
	case "APPLYING", "APPLIED":
		if d.ApplyGeneration != p.Generation || !version(d.ApplyRevision) || d.ApplyRevision > p.Revision ||
			d.ApplyingAt == nil || d.ApplyingAt.Before(d.ConfirmedAt) {
			return false
		}
		if d.Status == "APPLYING" {
			return len(d.Receipts) == 0
		}
		if len(d.Receipts) != 1 {
			return false
		}
		r := d.Receipts[0]
		return r.Environment == p.Environment && r.WorkspaceID == p.WorkspaceID &&
			r.DeletionID == d.DeletionID && r.ScopeDigest == d.ScopeDigest &&
			r.Generation == p.Generation && r.Revision == d.ApplyRevision &&
			!r.AppliedAt.Before(*d.ApplyingAt) && digestPattern.MatchString(r.ApplicationRef)
	default:
		return false
	}
}

func digest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
func version(n uint64) bool     { return n > 0 && n <= maxInteger }
func id(s string) bool          { return readcases.UUID(s) && s != "00000000-0000-0000-0000-000000000000" }
