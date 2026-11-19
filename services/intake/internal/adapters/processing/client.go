// Package processing delivers one Intake message to the configured Processing
// listener. The outbox worker alone decides when another attempt is allowed.
package processing

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/sendoutbox"
)

const Path = "/internal/v1/intake-signals"

type Client struct {
	address    string
	serverName string
	tls        *tls.Config
}

// New takes a trusted private listener and explicit Intake mTLS material.
// Network location and certificate identity are configured independently.
func New(address, serverName string, roots *x509.CertPool, identity tls.Certificate) (*Client, error) {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	p, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) || portErr != nil || p == 0 ||
		serverName == "" || strings.ContainsAny(serverName, "/:?#@[]\\ \t\r\n") || roots == nil ||
		len(identity.Certificate) == 0 || identity.PrivateKey == nil {
		return nil, app.ErrUnavailable
	}
	cert := identity
	cert.Certificate = make([][]byte, len(identity.Certificate))
	for i, b := range identity.Certificate {
		cert.Certificate[i] = bytes.Clone(b)
	}
	return &Client{address, serverName, &tls.Config{
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: roots.Clone(),
		ServerName: serverName, Certificates: []tls.Certificate{cert}, NextProtos: []string{"http/1.1"},
	}}, nil
}

type startFunc func(context.Context, func(context.Context) error) error

type guardedConn struct {
	net.Conn
	ctx   context.Context
	start startFunc
}

func (c guardedConn) Write(b []byte) (int, error) {
	var n int
	err := c.start(c.ctx, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || ctx.Err() != nil {
			return app.ErrDenied
		}
		if err := c.Conn.SetWriteDeadline(deadline); err != nil {
			return app.ErrUnavailable
		}
		var e error
		n, e = c.Conn.Write(b)
		return e
	})
	return n, err
}

func (c *Client) Send(ctx context.Context, requestID, messageID string, raw []byte, start func(context.Context, func(context.Context) error) error) (app.Result, error) {
	if !app.ValidUUID(requestID) || !app.ValidUUID(messageID) || start == nil || len(raw) == 0 || len(raw) > 65536 {
		return app.Result{}, app.ErrDenied
	}
	var denied atomic.Bool
	check := func(c context.Context, f func(context.Context) error) error {
		err := start(c, f)
		if errors.Is(err, app.ErrDenied) || errors.Is(err, app.ErrStale) {
			denied.Store(true)
		}
		return err
	}
	cfg := c.tls.Clone()
	t := &http.Transport{
		TLSClientConfig: cfg, DisableKeepAlives: true, DisableCompression: true,
		MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 2 * time.Second,
		ResponseHeaderTimeout: 2 * time.Second,
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			var conn net.Conn
			err := check(ctx, func(g context.Context) error {
				var e error
				conn, e = (&net.Dialer{}).DialContext(g, "tcp", c.address)
				return e
			})
			if err != nil {
				if conn != nil {
					_ = conn.Close()
				}
				return nil, err
			}
			return guardedConn{conn, ctx, check}, nil
		},
	}
	defer t.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+c.serverName+Path, bytes.NewReader(raw))
	if err != nil {
		return app.Result{}, app.ErrUnavailable
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	resp, err := t.RoundTrip(req)
	if err != nil {
		var cert *tls.CertificateVerificationError
		if denied.Load() || errors.As(err, &cert) {
			return app.Result{}, app.ErrDenied
		}
		return app.Result{Retry: true}, app.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		switch resp.StatusCode {
		case 429, 500, 502, 503, 504:
			return app.Result{Retry: true, RetryAfter: retryAfter(resp.Header.Values("Retry-After"), time.Now())}, nil
		default:
			return app.Result{}, nil
		}
	}
	med, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || med != "application/json" || len(resp.Header.Values("Content-Type")) != 1 ||
		len(resp.Header.Values("Content-Encoding")) != 0 {
		return app.Result{}, app.ErrUnavailable
	}
	const limit = 4096
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return app.Result{Retry: true}, app.ErrUnavailable
	}
	if len(body) > limit {
		return app.Result{}, app.ErrUnavailable
	}
	disposition, ok := receipt(body, messageID)
	if !ok {
		return app.Result{}, app.ErrUnavailable
	}
	return app.Result{Disposition: disposition}, nil
}

func receipt(raw []byte, messageID string) (string, bool) {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return "", false
	}
	if _, err := jcs.Transform(raw); err != nil {
		return "", false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || len(obj) < 2 || len(obj) > 4 {
		return "", false
	}
	var id, disposition string
	if json.Unmarshal(obj["message_id"], &id) != nil || id != messageID ||
		json.Unmarshal(obj["disposition"], &disposition) != nil {
		return "", false
	}
	switch disposition {
	case "APPLIED", "DUPLICATE", "HELD", "CONFLICT":
	default:
		return "", false
	}
	for key, value := range obj {
		switch key {
		case "message_id", "disposition":
		case "record_id":
			var id string
			if json.Unmarshal(value, &id) != nil || !app.ValidUUID(id) {
				return "", false
			}
		case "revision":
			var n int64
			if json.Unmarshal(value, &n) != nil || n < 1 || n > 9007199254740991 {
				return "", false
			}
		default:
			return "", false
		}
	}
	return disposition, true
}

func retryAfter(values []string, now time.Time) time.Time {
	if len(values) != 1 || values[0] == "" {
		return time.Time{}
	}
	value := values[0]
	if strings.Trim(value, "0123456789") == "" {
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil || n > uint64((1<<63-1)/int64(time.Second)) {
			return time.Unix(253402300799, 0)
		}
		return now.Add(time.Duration(n) * time.Second)
	}
	if t, err := http.ParseTime(value); err == nil {
		return t
	}
	return time.Time{}
}
