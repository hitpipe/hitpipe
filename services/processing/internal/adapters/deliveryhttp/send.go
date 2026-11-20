// Package deliveryhttp owns only the private Processing→Delivery transport.
package deliveryhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Sender struct {
	origin     string
	transport  *http.Transport
	maxReceipt int64
	clock      dispatchintent.Clock
}

func New(origin string, t *http.Transport, maxReceipt int64, clock dispatchintent.Clock) (*Sender, error) {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || t == nil || dispatchintent.Missing(clock) || maxReceipt < 1 || maxReceipt > 1048576 {
		return nil, dispatchintent.ErrHeld
	}
	cfg := t.TLSClientConfig
	if cfg == nil || cfg.InsecureSkipVerify || cfg.RootCAs == nil || len(cfg.Certificates) == 0 || cfg.MinVersion < tls.VersionTLS13 || (cfg.MaxVersion != 0 && cfg.MaxVersion < tls.VersionTLS13) || t.Proxy != nil || t.DialTLS != nil || t.DialTLSContext != nil || (cfg.ServerName != "" && cfg.ServerName != u.Hostname()) {
		return nil, dispatchintent.ErrHeld
	}
	t = t.Clone()
	t.TLSClientConfig = cfg.Clone()
	t.TLSClientConfig.RootCAs = cfg.RootCAs.Clone()
	t.TLSClientConfig.ServerName = u.Hostname()
	t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	t.DisableKeepAlives = true
	t.DisableCompression = true
	t.ForceAttemptHTTP2 = false
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	return &Sender{u.Scheme + "://" + u.Host, t, maxReceipt, clock}, nil
}
func (s *Sender) Send(ctx context.Context, c dispatchintent.Claim, m dispatchintent.Message, b dispatchintent.Boundary) (dispatchintent.Receipt, error) {
	out, raw, err := s.exchange(ctx, c, m, b, "/internal/v1/delivery-intents", http.StatusAccepted)
	if err != nil || out.HTTPStatus != http.StatusAccepted {
		if err != nil {
			return out, dispatchintent.ErrHeld
		}
		return out, nil
	}
	receipt, e := decodeReceipt(raw, c.MessageID)
	receipt.HTTPStatus = out.HTTPStatus
	receipt.RetryAfter = out.RetryAfter
	return receipt, e
}

func (s *Sender) exchange(ctx context.Context, c dispatchintent.Claim, m dispatchintent.Message, b dispatchintent.Boundary, path string, success int) (dispatchintent.Receipt, []byte, error) {
	zero := dispatchintent.Receipt{}
	if s == nil || ctx == nil || dispatchintent.Missing(b) || len(m.Body) == 0 {
		return zero, nil, dispatchintent.ErrHeld
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, s.origin+path, bytes.NewReader(m.Body))
	if e != nil {
		return zero, nil, dispatchintent.ErrHeld
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", c.MessageID)
	t := s.transport.Clone()
	t.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
		var conn net.Conn
		e := dispatchintent.Protected(ctx, b, c, func(x context.Context) error {
			dial := s.transport.DialContext
			if dial == nil {
				dial = (&net.Dialer{}).DialContext
			}
			var err error
			conn, err = dial(x, network, address)
			return err
		})
		if e != nil {
			if conn != nil {
				conn.Close()
			}
			return nil, e
		}
		return &guardedConn{Conn: conn, ctx: ctx, scope: c, boundary: b}, nil
	}
	defer t.CloseIdleConnections()
	// Direct RoundTrip neither follows redirects nor provides a Client retry hook.
	// New non-reusing HTTP/1 connection + GetBody=nil prevents hidden POST replay.
	resp, e := t.RoundTrip(req)
	if e != nil {
		return zero, nil, e
	}
	if resp.Body == nil {
		return zero, nil, dispatchintent.ErrHeld
	}
	defer resp.Body.Close()
	out := dispatchintent.Receipt{HTTPStatus: resp.StatusCode}
	now := s.clock.Now()
	if value := resp.Header.Get("Retry-After"); value != "" {
		if seconds, e := strconv.ParseInt(value, 10, 32); e == nil && seconds >= 0 {
			out.RetryAfter = now.Add(time.Duration(seconds) * time.Second)
		} else if at, e := http.ParseTime(value); e == nil && at.After(now) {
			out.RetryAfter = at
		}
	}
	if resp.StatusCode != success {
		return out, nil, nil
	}
	media, _, e := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if e != nil || media != "application/json" || resp.Header.Get("Content-Encoding") != "" || resp.ContentLength > s.maxReceipt {
		return out, nil, dispatchintent.ErrHeld
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, s.maxReceipt+1))
	if e != nil || int64(len(raw)) > s.maxReceipt || ctx.Err() != nil {
		return out, nil, dispatchintent.ErrHeld
	}
	return out, raw, nil
}

type guardedConn struct {
	net.Conn
	ctx      context.Context
	scope    dispatchintent.Claim
	boundary dispatchintent.Boundary
}

func (c *guardedConn) Write(b []byte) (int, error) {
	n := 0
	e := dispatchintent.Protected(c.ctx, c.boundary, c.scope, func(x context.Context) error {
		deadline := time.Now().Add(2 * time.Second)
		if d, ok := x.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if e := c.Conn.SetWriteDeadline(deadline); e != nil {
			return e
		}
		var e error
		n, e = c.Conn.Write(b)
		return e
	})
	return n, e
}
