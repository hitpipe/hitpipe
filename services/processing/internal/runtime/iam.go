package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

const (
	limitBody     = 256 << 10
	protectedTime = 2 * time.Second
)

type iamClient struct {
	origin   string
	client   *http.Client
	identity startgate.Identity
	certHash string
}

func outbound(peer Peer, ca *x509.CertPool, leaf tls.Certificate) *http.Transport {
	dialer := &net.Dialer{Timeout: protectedTime}
	return &http.Transport{
		DialContext: dialer.DialContext, TLSHandshakeTimeout: protectedTime,
		DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 16 << 10,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
			RootCAs: ca, Certificates: []tls.Certificate{leaf}, NextProtos: []string{"http/1.1"},
			VerifyConnection: func(s tls.ConnectionState) error {
				if len(s.VerifiedChains) == 0 || len(s.PeerCertificates) == 0 || s.PeerCertificates[0].VerifyHostname(peer.Name) != nil {
					return ErrUnavailable
				}
				return nil
			},
		},
	}
}

func newIAM(c Peer, t *http.Transport, id startgate.Identity, cert tls.Certificate) iamClient {
	h := sha256.Sum256(cert.Certificate[0])
	return iamClient{origin: c.Origin, identity: id, certHash: "sha256:" + hex.EncodeToString(h[:]),
		client: &http.Client{Transport: t, Timeout: protectedTime, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (i iamClient) post(ctx context.Context, path, session, schema, expected string, input, result any) error {
	raw, err := json.Marshal(input)
	if err != nil || draftjson.Validate(schema, raw) != nil {
		return ErrConfig
	}
	requestID, err := drafts.NewID()
	if err != nil {
		return ErrUnavailable
	}
	q, err := http.NewRequestWithContext(ctx, http.MethodPost, i.origin+path, bytes.NewReader(raw))
	if err != nil {
		return ErrUnavailable
	}
	q.GetBody = nil
	q.Header.Set("X-Request-ID", requestID)
	q.Header.Set("Content-Type", "application/json")
	q.Header.Set("Accept", "application/json")
	if session != "" {
		q.Header.Set("X-HitPipe-Session", session)
	}
	r, err := i.client.Do(q)
	if err != nil {
		return ErrUnavailable
	}
	defer r.Body.Close()
	if r.StatusCode == http.StatusUnauthorized {
		return drafts.ErrUnauthenticated
	}
	if r.StatusCode == http.StatusForbidden {
		return drafts.ErrDenied
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if r.StatusCode != 200 || err != nil || typ != "application/json" || r.Header.Get("Content-Encoding") != "" {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20+1))
	if err != nil || len(body) > 2<<20 || draftjson.Validate(expected, body) != nil || json.Unmarshal(body, result) != nil {
		return ErrUnavailable
	}
	return nil
}

func (i iamClient) sync(ctx context.Context, g *startgate.Gate) error {
	req := drafts.RuntimeRegistration{RuntimeID: i.identity.RuntimeID, BootID: i.identity.BootID,
		Service: "PROCESSING", Generation: int64(i.identity.Generation), CertificateFingerprint: i.certHash}
	var res drafts.RuntimeSnapshot
	if err := i.post(ctx, "/internal/v1/runtime/register", "", "RuntimeRegistration", "RuntimeSnapshot", req, &res); err != nil {
		return err
	}
	denials := make([]startgate.Denial, 0, len(res.Denials))
	for _, d := range res.Denials {
		denials = append(denials, startgate.Denial{Subject: subject(d.Scope, d.SubjectID, d.WorkspaceID),
			Epoch: uint64(d.Epoch), RevocationID: d.RevocationID, AcceptedAt: d.AcceptedAt, EffectiveAt: d.EffectiveAt})
	}
	return g.ApplySnapshot(ctx, startgate.RuntimeSnapshot{RuntimeID: res.RuntimeID, BootID: res.BootID,
		Generation: uint64(res.Generation), SnapshotRevision: uint64(res.SnapshotRevision), Gate: startgate.State(res.Gate), Denials: denials})
}

func subject(scope, id string, workspace *string) startgate.Subject {
	return startgate.Subject{Scope: startgate.Scope(scope), SubjectID: id, WorkspaceID: workspace}
}

type permit struct {
	workspace, principal, session, role string
	epoch                               int64
	expires                             time.Time
	connections                         []string
}

func (i iamClient) authorize(ctx context.Context, ws, action, token string, resource *string) (permit, error) {
	var p permit
	if !drafts.UUID(ws) || len(token) < 43 || len(token) > 128 {
		return p, drafts.ErrUnauthenticated
	}
	var a drafts.Authorization
	q := drafts.AuthorizeRequest{WorkspaceID: ws, Action: action, RuntimeID: i.identity.RuntimeID, ResourceID: resource}
	err := i.post(ctx, "/internal/v1/authorize", token, "AuthorizeRequest", "Authorization", q, &a)
	if err != nil {
		return p, err
	}
	if !a.Allowed || a.RevocationPending {
		return p, drafts.ErrDenied
	}
	now := time.Now()
	if a.PrincipalID == nil || a.SessionID == nil || a.WorkspaceID == nil || *a.WorkspaceID != ws || a.Role == nil ||
		a.CheckedAt.After(now) || !now.Before(a.CheckedAt.Add(protectedTime)) {
		return p, drafts.ErrDenied
	}
	return permit{workspace: ws, principal: *a.PrincipalID, session: *a.SessionID, role: *a.Role,
		epoch: a.Epoch, expires: a.CheckedAt.Add(protectedTime)}, nil
}

type permitKey struct{}
type requestPermit struct{ permit permit }

func withPermit(ctx context.Context) context.Context {
	return context.WithValue(ctx, permitKey{}, new(requestPermit))
}

func scopeOf(ctx context.Context) *requestPermit {
	p, _ := ctx.Value(permitKey{}).(*requestPermit)
	return p
}

func (p permit) matches(workspace, principal string, epoch int64) bool {
	return p.workspace == workspace && p.principal == principal && p.epoch == epoch && drafts.UUID(p.session) && time.Now().Before(p.expires)
}

func gated(ctx context.Context, g *startgate.Gate, p permit, f func(context.Context) error) error {
	if !p.matches(p.workspace, p.principal, p.epoch) || !drafts.UUID(p.workspace) || !drafts.UUID(p.principal) || p.epoch < 1 {
		return drafts.ErrDenied
	}
	c, cancel := context.WithDeadline(ctx, p.expires)
	defer cancel()
	ws := p.workspace
	subjects := []startgate.Subject{
		{Scope: startgate.Session, WorkspaceID: &ws, SubjectID: p.session},
		{Scope: startgate.Membership, WorkspaceID: &ws, SubjectID: p.principal},
		{Scope: startgate.Workspace, WorkspaceID: &ws, SubjectID: ws},
	}
	for _, id := range p.connections {
		subjects = append(subjects, startgate.Subject{Scope: startgate.Connection, WorkspaceID: &ws, SubjectID: id})
	}
	req := startgate.StartRequest{Authorities: make([]startgate.Authority, 0, len(subjects))}
	for _, s := range subjects {
		req.Authorities = append(req.Authorities, startgate.Authority{Subject: s, Epoch: uint64(p.epoch), Allowed: true})
	}
	err := g.Start(c, req, f)
	if errors.Is(err, startgate.ErrClosed) || errors.Is(err, startgate.ErrDenied) || errors.Is(err, startgate.ErrInvalid) {
		return drafts.ErrDenied
	}
	return err
}
