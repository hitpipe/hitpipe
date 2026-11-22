package dispatch

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

const maxResponse = 64 * 1024

type ClickUp struct {
	origin    url.URL
	address   string
	tlsConfig *tls.Config
}

// NewClickUp fixes the live origin. The local fixture uses a separate,
// package-private constructor. Neither request content nor response redirects
// can override the host, port or TLS name.
func NewClickUp(roots *x509.CertPool) *ClickUp {
	return clickUpAt(url.URL{Scheme: "https", Host: "api.clickup.com"}, roots)
}

func clickUpAt(origin url.URL, roots *x509.CertPool) *ClickUp {
	port := origin.Port()
	if port == "" {
		port = "443"
	}
	if roots != nil {
		roots = roots.Clone()
	}
	return &ClickUp{origin: origin, address: net.JoinHostPort(origin.Hostname(), port),
		tlsConfig: &tls.Config{MinVersion: tls.VersionTLS13, ServerName: origin.Hostname(), RootCAs: roots,
			NextProtos: []string{"http/1.1"}}}
}

func validToken(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	for _, b := range []byte(s) {
		if b <= 0x20 || b >= 0x7f {
			return false
		}
	}
	return true
}

func positiveDecimal(s string) bool {
	if len(s) == 0 || len(s) > 255 || s[0] < '1' || s[0] > '9' {
		return false
	}
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

func (c *ClickUp) request(p payload, token string) (*http.Request, error) {
	if !validToken(token) || !positiveDecimal(p.List) || !positiveDecimal(p.Assignee) ||
		p.Title == "" || p.Description == "" || !utf8.ValidString(p.Title) || !utf8.ValidString(p.Description) {
		return nil, ErrHeld
	}
	person, err := strconv.ParseInt(p.Assignee, 10, 64)
	if err != nil {
		return nil, ErrHeld
	}
	body, err := json.Marshal(struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Assignees   []int64 `json:"assignees"`
	}{p.Title, p.Description, []int64{person}})
	if err != nil {
		return nil, ErrHeld
	}
	destination := c.origin
	destination.Path = "/api/v2/list/" + p.List + "/task"
	req, err := http.NewRequest(http.MethodPost, destination.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrHeld
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	req.Close = true
	return req, nil
}

type exchange struct {
	conn net.Conn
	req  *http.Request
}

func (c *ClickUp) start(ctx context.Context, req *http.Request, beforeWrite func() error) (*exchange, error) {
	deadline, ok := ctx.Deadline()
	if !ok || ctx.Err() != nil {
		return nil, ErrHeld
	}
	dialer := tls.Dialer{Config: c.tlsConfig}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return nil, ErrHeld
	}
	disarm := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer disarm()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, ErrHeld
	}
	if err := beforeWrite(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if ctx.Err() != nil || req.Write(conn) != nil {
		_ = conn.Close()
		return nil, ErrHeld
	}
	return &exchange{conn: conn, req: req}, nil
}

type outcome struct {
	result Result
	code   string
	proof  string
}

func (e *exchange) receive(ctx context.Context) outcome {
	unknown := outcome{result: Result{Disposition: Ambiguous}, code: "PROVIDER_OUTCOME_UNKNOWN"}
	deadline, ok := ctx.Deadline()
	if !ok || ctx.Err() != nil || e.conn.SetDeadline(deadline) != nil {
		return unknown
	}
	stop := context.AfterFunc(ctx, func() { _ = e.conn.Close() })
	defer stop()
	wire := &io.LimitedReader{R: e.conn, N: 2 * maxResponse}
	resp, err := http.ReadResponse(bufio.NewReader(wire), e.req)
	if err != nil {
		return unknown
	}
	defer e.conn.Close()
	// No further action follows a redirect or other status. No provider
	// rejection is classified as proof that no external write occurred.
	if resp.StatusCode != http.StatusOK {
		return unknown
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse || !utf8.Valid(data) {
		return unknown
	}
	canonical, err := jcs.Transform(data)
	if err != nil {
		return unknown
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(canonical, &value) != nil {
		return unknown
	}
	var id string
	if json.Unmarshal(value["id"], &id) != nil || !validTaskID(id) {
		return unknown
	}
	return outcome{result: Result{Disposition: TargetIDObserved, TaskID: id}, code: "PROVIDER_TASK_ID_RETURNED",
		proof: fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))}
}

func validTaskID(id string) bool {
	if id == "" || len(id) > 255 || strings.TrimSpace(id) != id {
		return false
	}
	for _, b := range []byte(id) {
		if b <= 0x20 || b >= 0x7f {
			return false
		}
	}
	return true
}
