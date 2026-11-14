package sourcefacts

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
)

// CallerContext is an already established caller scope, not authority inferred
// by this adapter. RequestID is the OAS-required X-Request-ID. LiveMode is nullable
// only locally so missing context cannot silently become known test/false.
type CallerContext struct {
	WorkspaceID, CaseID, SessionID, ConnectionID, AccountID, RequestID string
	ExpectedEpoch                                                      int64
	DeadlineAt                                                         time.Time
	LiveMode                                                           *bool
}

// StartBoundary is the injected shared runtime boundary. It must synchronously
// execute start under its current scope/epoch/start guard, or deny without calling
// it. It must not cache permission, retain start, or return before it completes.
// No production implementation or permissive default is supplied here.
type StartBoundary interface {
	Start(context.Context, CallerContext, func() error) error
}

// ReadError separates local context, start denial, transport, status and message
// failures. It never contains a URL, response body, identity or TLS diagnostic.
type ReadError struct {
	Kind       string
	StatusCode int
}

func (e *ReadError) Error() string { return "SOURCE_READ_" + e.Kind }
func failure(kind string) error    { return &ReadError{Kind: kind} }

// Reader owns immutable local policy/scope and a private clone of the explicitly
// supplied transport. Each read owns a new non-reusing HTTP/1 connection. This
// prevents hidden replay/reuse from bypassing the raw socket start boundary.
type Reader struct {
	endpoint         string
	policy           eligibility.Policy
	caller           CallerContext
	transport        *http.Transport
	boundary         StartBoundary
	maxResponseBytes int64
}

var _ evaluatepaidorder.SourceFactsReader = (*Reader)(nil)

// NewReader has no default transport, gate, origin, scope or body buffer bound.
// maxResponseBytes is a required caller transport resource bound, not an added
// OAS/product limit. Test values qualify only the controlled transport fixture;
// production composition must supply its established transport configuration.
func NewReader(origin string, p eligibility.Policy, c CallerContext, transport *http.Transport, boundary StartBoundary, maxResponseBytes int64) (*Reader, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, failure("CONTEXT")
	}
	if !eligibility.ValidatePolicy(p) || !uuid.MatchString(c.WorkspaceID) || !uuid.MatchString(c.CaseID) || !uuid.MatchString(c.ConnectionID) || !uuid.MatchString(c.RequestID) || c.SessionID == "" || len([]rune(c.SessionID)) > requestSessionMax || c.ExpectedEpoch < requestEpochMin || c.ExpectedEpoch > requestEpochMax || c.DeadlineAt.IsZero() || c.LiveMode == nil || c.AccountID != p.AccountID || c.ConnectionID != p.ConnectionID || *c.LiveMode != *p.LiveMode {
		return nil, failure("CONTEXT")
	}
	if transport == nil || missingBoundary(boundary) || maxResponseBytes <= 0 || maxResponseBytes == int64(^uint64(0)>>1) {
		return nil, failure("DEPENDENCY")
	}
	cfg := transport.TLSClientConfig
	if cfg == nil || cfg.InsecureSkipVerify || cfg.RootCAs == nil || len(cfg.Certificates) == 0 || cfg.MinVersion < tls.VersionTLS13 || (cfg.MaxVersion != 0 && cfg.MaxVersion < tls.VersionTLS13) || transport.Proxy != nil || transport.DialTLS != nil || transport.DialTLSContext != nil {
		return nil, failure("TRANSPORT_CONFIG")
	}
	// The origin identity must be checked by ordinary TLS verification, not an
	// override selecting a different peer name. Additional verifiers may only deny.
	if cfg.ServerName != "" && cfg.ServerName != u.Hostname() {
		return nil, failure("TRANSPORT_CONFIG")
	}
	t := transport.Clone()
	t.TLSClientConfig = cfg.Clone()
	t.TLSClientConfig.RootCAs = cfg.RootCAs.Clone()
	t.TLSClientConfig.ServerName = u.Hostname()
	t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	t.DisableKeepAlives = true
	t.DisableCompression = true
	t.ForceAttemptHTTP2 = false
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	live := *c.LiveMode
	c.LiveMode = &live
	plive := *p.LiveMode
	p.LiveMode = &plive
	return &Reader{endpoint: u.Scheme + "://" + u.Host + "/internal/v1/source-facts/read", policy: p, caller: c, transport: t, boundary: boundary, maxResponseBytes: maxResponseBytes}, nil
}

// Read implements the existing application port. ReadProjection exposes the same
// read plus original provenance to its caller, without mutable last-result state.
func (r *Reader) Read(ctx context.Context, q evaluatepaidorder.Query) (evaluatepaidorder.Snapshot, error) {
	p, err := r.ReadProjection(ctx, q)
	return p.Snapshot, err
}

func (r *Reader) ReadProjection(ctx context.Context, q evaluatepaidorder.Query) (Projection, error) {
	fail := func(err error) (Projection, error) { return Projection{}, err }
	if r == nil || ctx == nil {
		return fail(failure("CONTEXT"))
	}
	c := r.caller
	if q.CaseID != c.CaseID || q.SessionID != c.SessionID || q.ConnectionID != c.ConnectionID || q.AccountID != c.AccountID || q.LiveMode != *c.LiveMode || q.ConfigID != r.policy.ConfigID {
		return fail(failure("CONTEXT"))
	}
	ctx, cancel := context.WithDeadline(ctx, c.DeadlineAt)
	defer cancel()
	if ctx.Err() != nil {
		return fail(failure("CANCELED"))
	}
	body, err := json.Marshal(sourceReadRequest{WorkspaceID: c.WorkspaceID, ConnectionID: c.ConnectionID, SessionID: c.SessionID, CaseID: c.CaseID, ExpectedEpoch: c.ExpectedEpoch, DeadlineAt: c.DeadlineAt})
	if err != nil {
		return fail(failure("CONTEXT"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fail(failure("CONTEXT"))
	}
	req.GetBody = nil // Never offer replay of this POST to net/http.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", c.RequestID)
	t := r.transport.Clone()
	// The shared boundary is applied at socket connect AND every raw socket write,
	// including TLS records. A pre-check before RoundTrip would leave a revoke gap.
	t.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
		var conn net.Conn
		err := r.start(ctx, func() error {
			dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			var err error
			dial := r.transport.DialContext
			if dial == nil {
				dial = (&net.Dialer{}).DialContext
			}
			conn, err = dial(dialCtx, network, address)
			return err
		})
		if err != nil {
			if conn != nil {
				conn.Close()
			}
			return nil, err
		}
		return &startedConn{Conn: conn, ctx: ctx, reader: r}, nil
	}
	defer t.CloseIdleConnections()
	response, err := t.RoundTrip(req)
	if err != nil {
		var e *ReadError
		if errors.As(err, &e) {
			return fail(e)
		}
		if ctx.Err() != nil {
			return fail(failure("CANCELED"))
		}
		return fail(failure("TRANSPORT"))
	}
	if response.Body == nil {
		return fail(failure("MESSAGE"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fail(&ReadError{Kind: "HTTP_STATUS", StatusCode: response.StatusCode})
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return fail(failure("MESSAGE"))
	}
	if response.ContentLength > r.maxResponseBytes {
		return fail(failure("RESPONSE_BOUND"))
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, r.maxResponseBytes+1))
	if ctx.Err() != nil {
		return fail(failure("CANCELED"))
	}
	if err != nil {
		return fail(failure("TRANSPORT"))
	}
	if int64(len(b)) > r.maxResponseBytes {
		return fail(failure("RESPONSE_BOUND"))
	}
	p, err := Decode(b, Expected{CaseID: c.CaseID, SessionID: c.SessionID, AccountID: c.AccountID, ConnectionID: c.ConnectionID, LiveMode: *c.LiveMode}, r.policy)
	if err != nil {
		return fail(failure("MESSAGE"))
	}
	if ctx.Err() != nil {
		return fail(failure("CANCELED"))
	}
	p.Snapshot.Proof = &evaluatepaidorder.SourceProof{FactsID: p.Provenance.FactsID, Digest: p.Provenance.Digest, SessionCreatedAt: p.Provenance.SessionCreatedAt, OldestObservedAt: p.Provenance.OldestObservedAt, CompletedAt: p.Provenance.CompletedAt, ValidUntil: p.Provenance.ValidUntil}
	return p, nil
}

func (r *Reader) start(ctx context.Context, start func() error) error {
	if ctx.Err() != nil {
		return failure("CANCELED")
	}
	var called atomic.Bool
	var active atomic.Bool
	active.Store(true)
	c := r.caller
	live := *c.LiveMode
	c.LiveMode = &live // no shared caller-owned pointer
	var actionErr error
	err := r.boundary.Start(ctx, c, func() error {
		if !active.Load() || !called.CompareAndSwap(false, true) {
			return failure("START_DENIED")
		}
		if ctx.Err() != nil {
			return failure("CANCELED")
		}
		actionErr = start()
		return actionErr
	})
	active.Store(false)
	if actionErr != nil {
		return actionErr
	}
	if err != nil || !called.Load() {
		return failure("START_DENIED")
	}
	return nil
}

type startedConn struct {
	net.Conn
	ctx    context.Context
	reader *Reader
}

func (c *startedConn) Write(b []byte) (int, error) {
	n := 0
	err := c.reader.start(c.ctx, func() error {
		// A012 bounds a critical start operation to <=2s, not a new whole-read SLA.
		deadline := time.Now().Add(2 * time.Second)
		if d, ok := c.ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if err := c.Conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		var err error
		n, err = c.Conn.Write(b)
		return err
	})
	return n, err
}

func missingBoundary(b StartBoundary) bool {
	if b == nil {
		return true
	}
	v := reflect.ValueOf(b)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	}
	return false
}
