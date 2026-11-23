package dispatch

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"
)

const (
	maxReadConnections = 8
	maxIdleReads       = 4
	readIdleTimeout    = 15 * time.Second
	readTLSLifetime    = time.Minute
	readIdleProbe      = 250 * time.Microsecond
)

// readPool belongs to one QueryService and its fixed, verified ClickUp origin.
// Each lease permits one HTTP/1.1 GET and is never passed to a write transport.
type readPool struct {
	upstream *ClickUp
	slots    chan struct{}
	idleFor  time.Duration
	mu       sync.Mutex
	idle     []*idleRead
}

type readConn struct {
	tls       *tls.Conn
	expiresAt time.Time
}

func (c readConn) abort() {
	// Close TCP directly so a stalled peer cannot delay release of a slot.
	_ = c.tls.NetConn().Close()
}

type idleRead struct {
	readConn
	until time.Time
	timer *time.Timer
}

func newReadPool(upstream *ClickUp) *readPool {
	return &readPool{
		upstream: upstream,
		slots:    make(chan struct{}, maxReadConnections),
		idleFor:  readIdleTimeout,
	}
}

func (p *readPool) takeIdle() (readConn, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for n := len(p.idle); n > 0; n-- {
		entry := p.idle[n-1]
		p.idle[n-1] = nil
		p.idle = p.idle[:n-1]
		entry.timer.Stop()
		if time.Now().Before(entry.until) {
			return entry.readConn, true
		}
		entry.abort()
	}
	return readConn{}, false
}

// reuseProbe discards a closed idle connection or one with unexpected input
// before any request is written. It never replays a GET after a write.
func (c readConn) reuseProbe(ctx context.Context, deadline time.Time) bool {
	if !time.Now().Before(c.expiresAt) || c.tls.SetReadDeadline(earlier(deadline, time.Now().Add(readIdleProbe))) != nil {
		return false
	}
	var b [1]byte
	n, err := c.tls.Read(b[:])
	var timeout net.Error
	return n == 0 && errors.As(err, &timeout) && timeout.Timeout() && ctx.Err() == nil
}

func (p *readPool) connect(ctx context.Context, deadline time.Time) (readConn, error) {
	for {
		idle, ok := p.takeIdle()
		if !ok {
			break
		}
		if idle.reuseProbe(ctx, deadline) {
			return idle, nil
		}
		idle.abort()
	}
	dialer := tls.Dialer{Config: p.upstream.tlsConfig}
	raw, err := dialer.DialContext(ctx, "tcp", p.upstream.address)
	if err != nil {
		return readConn{}, ErrHeld
	}
	conn := raw.(*tls.Conn)
	expires := earlier(time.Now().Add(readTLSLifetime), conn.ConnectionState().PeerCertificates[0].NotAfter)
	return readConn{tls: conn, expiresAt: expires}, nil
}

func (p *readPool) keep(c readConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	until := earlier(time.Now().Add(p.idleFor), c.expiresAt)
	if len(p.idle) >= maxIdleReads || !time.Now().Before(until) || c.tls.SetDeadline(time.Time{}) != nil {
		c.abort()
		return
	}
	entry := &idleRead{readConn: c, until: until}
	entry.timer = time.AfterFunc(time.Until(until), func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if i := slices.Index(p.idle, entry); i >= 0 {
			p.idle = slices.Delete(p.idle, i, i+1)
			entry.abort()
		}
	})
	p.idle = append(p.idle, entry)
}

type readExchange struct {
	readConn
	pool    *readPool
	request *http.Request
}

func (e *readExchange) finish(reuse bool) {
	if e.tls == nil {
		return
	}
	if reuse {
		e.pool.keep(e.readConn)
	} else {
		e.abort()
	}
	e.tls = nil
	<-e.pool.slots
}

// start is invoked inside the actual-start gate. Acquiring or reusing a
// connection does not authorize a send. The budget/SQL recheck runs before
// each individual wire write on both fresh and idle connections.
func (p *readPool) start(ctx context.Context, req *http.Request, beforeWrite func() error) (*readExchange, error) {
	deadline, ok := ctx.Deadline()
	if !ok || ctx.Err() != nil || req.Method != http.MethodGet || req.Body != nil {
		return nil, ErrHeld
	}
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrHeld
	}
	conn, err := p.connect(ctx, deadline)
	if err != nil {
		<-p.slots
		return nil, err
	}
	ex := &readExchange{readConn: conn, pool: p, request: req}
	abort := context.AfterFunc(ctx, conn.abort)
	started := false
	defer func() {
		if !started {
			abort()
			ex.finish(false)
		}
	}()
	if conn.tls.SetDeadline(deadline) != nil {
		return nil, ErrHeld
	}
	if err := beforeWrite(); err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !time.Now().Before(conn.expiresAt) || req.Write(conn.tls) != nil || !abort() {
		return nil, ErrHeld
	}
	started = true
	return ex, nil
}

func (e *readExchange) receive(ctx context.Context) (_ boundedResponse, reusable bool, _ error) {
	deadline, ok := ctx.Deadline()
	if !ok || ctx.Err() != nil || e.tls.SetDeadline(deadline) != nil {
		return boundedResponse{}, false, ErrHeld
	}
	conn := e.readConn
	abort := context.AfterFunc(ctx, conn.abort)
	defer func() {
		reusable = abort() && ctx.Err() == nil && reusable
	}()
	wire := bufio.NewReader(&io.LimitedReader{R: e.tls, N: 2 * maxResponse})
	resp, err := http.ReadResponse(wire, e.request)
	if err != nil {
		return boundedResponse{}, false, ErrHeld
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		// Close the socket first. Closing an unread net/http body may drain it.
		conn.abort()
		_ = resp.Body.Close()
		return boundedResponse{}, false, ErrHeld
	}
	if resp.Body.Close() != nil {
		return boundedResponse{}, false, ErrHeld
	}
	return boundedResponse{status: resp.StatusCode, body: data},
		resp.ProtoMajor == 1 && resp.ProtoMinor == 1 && !resp.Close && wire.Buffered() == 0, nil
}
