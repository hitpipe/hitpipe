package dispatch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
)

type poolFixture struct {
	clickup        *ClickUp
	opened, closed atomic.Int64
}

func poolUpstream(t *testing.T, handler http.HandlerFunc) *poolFixture {
	t.Helper()
	f := new(poolFixture)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/task/fixture-task" ||
			r.URL.RawQuery != "" || r.Header.Get("Authorization") != fixtureToken ||
			r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
			t.Error("unexpected ClickUp wire request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		handler(w, r)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			f.opened.Add(1)
		case http.StateClosed:
			f.closed.Add(1)
		}
	}
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	origin, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	f.clickup = clickUpAt(*origin, roots)
	return f
}

func eventuallyClosed(t *testing.T, f *poolFixture, want int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for f.closed.Load() != want {
		select {
		case <-ctx.Done():
			t.Fatalf("closed %d of %d opened, wanted %d", f.closed.Load(), f.opened.Load(), want)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestPooledReadObservationAndBarrier(t *testing.T) {
	f := setup(t)
	fp := f.budget(t, 20)
	job, auth, query := knownFixture(t, f, fp)
	body := targetJSON(t, auth, "fixture-task", "")
	started := make(chan struct{})
	returnAnswer := make(chan struct{})
	var received atomic.Int64
	target := poolUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if received.Add(1) == 3 {
			close(started)
			select {
			case <-returnAnswer:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = w.Write(body)
	})
	service := queryFixture(t, f, auth, fp, target.clickup)
	var revoked atomic.Bool
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	service.gate = allowGate(func(ctx context.Context, _ uuid.UUID, _ int64, run func() error) error {
		select {
		case <-lock:
			defer func() { lock <- struct{}{} }()
		case <-ctx.Done():
			return ErrHeld
		}
		if revoked.Load() {
			return intake.ErrDenied
		}
		return run()
	})
	read := func() intake.TaskObservation {
		t.Helper()
		o, err := service.QueryKnownTask(t.Context(), query)
		if err != nil || o.Knowledge != "RESULT_VERIFIED" || !o.Complete || o.EvidenceRef == nil ||
			o.MessageDigest == "" || o.ProofDigest == "" || o.ObservedAt.IsZero() {
			t.Fatalf("invalid verified observation: %s, %v", o.Knowledge, err)
		}
		return o
	}
	first, reused := read(), read()
	if first.MessageID == reused.MessageID || first.Observation == reused.Observation || *first.EvidenceRef == *reused.EvidenceRef {
		t.Fatal("transport reuse cached an observation or identifier")
	}
	compare := func(o intake.TaskObservation) intake.TaskObservation {
		o.MessageID, o.Observation = uuid.Nil, uuid.Nil
		o.EvidenceRef = nil
		o.ObservedAt = time.Time{}
		o.MessageDigest, o.ProofDigest = "", ""
		return o
	}
	if !reflect.DeepEqual(compare(first), compare(reused)) || target.opened.Load() != 1 {
		t.Fatal("reused socket changed TaskObservation or opened another TCP connection")
	}

	inFlight := make(chan error, 1)
	go func() {
		_, err := service.QueryKnownTask(t.Context(), query)
		inFlight <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight GET never arrived")
	}
	// Apply a barrier while a GET is awaiting its response on the reused
	// socket. The barrier cannot pass the write's protected start section.
	barrierCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	select {
	case <-lock:
	case <-barrierCtx.Done():
		t.Fatal("response read held the protected start gate")
	}
	_, err := f.admin.Exec(barrierCtx, "UPDATE hp_delivery.destination_connections SET revoked_at=clock_timestamp(),security_epoch=2 WHERE workspace_id=$1", job.Workspace)
	revoked.Store(true)
	lock <- struct{}{}
	close(returnAnswer)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-inFlight:
		if !errors.Is(err, intake.ErrDenied) {
			t.Fatalf("old epoch could record an observation after barrier: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("in-flight GET did not resolve")
	}
	if _, err = service.QueryKnownTask(t.Context(), query); !errors.Is(err, intake.ErrDenied) ||
		received.Load() != 3 || target.opened.Load() != 1 {
		t.Fatalf("start after barrier: err=%v GETs=%d TCP=%d", err, received.Load(), target.opened.Load())
	}
	t.Logf("same validated TaskObservation on fresh and reused TLS; GETs=%d TCP=%d; further old-epoch GETs=0", received.Load(), target.opened.Load())
}

func poolReadStart(t *testing.T, ctx context.Context, p *readPool, beforeWrite func() error) (*readExchange, error) {
	t.Helper()
	req, err := p.upstream.readRequest("fixture-task", fixtureToken)
	if err != nil {
		t.Fatal(err)
	}
	return p.start(ctx, req, beforeWrite)
}

func TestPooledReadClosesOnUnsafeResponse(t *testing.T) {
	for _, scenario := range []struct {
		name string
		code string
		send http.HandlerFunc
	}{
		{"redirect", "READ_FAILED", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/other", http.StatusFound)
		}},
		{"429", "READ_RATE_LIMITED", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Limit", "100")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "2000000000")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}},
		{"oversize", "READ_FAILED", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", maxResponse+1))
		}},
		{"malformed", "READ_INVALID_RESPONSE", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"id":"fixture-task","id":"duplicate"}`)
		}},
		{"refused", "READ_NOT_ACCESSIBLE", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}},
		{"reset", "READ_FAILED", func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}},
		{"timeout", "READ_FAILED", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"id":`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var received, charged atomic.Int64
			target := poolUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				if received.Add(1) == 2 {
					scenario.send(w, r)
					return
				}
				_, _ = io.WriteString(w, `{"id":"fixture-task"}`)
			})
			pool := newReadPool(target.clickup)
			read := func(timeout time.Duration) (string, error) {
				t.Helper()
				start, stop := context.WithTimeout(t.Context(), time.Second)
				defer stop()
				ex, err := poolReadStart(t, start, pool, func() error {
					charged.Add(1)
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				stop()
				ctx, cancel := context.WithTimeout(t.Context(), timeout)
				defer cancel()
				defer ex.finish(false)
				reader := &targetReader{ex: ex, id: "fixture-task"}
				_, _, err = reader.Read(ctx, "fixture-task")
				return reader.code, err
			}
			if _, err := read(time.Second); err != nil {
				t.Fatal(err)
			}
			if code, err := read(200 * time.Millisecond); code != scenario.code || !errors.Is(err, ErrHeld) {
				t.Fatalf("got %s, %v; expected %s", code, err, scenario.code)
			}
			eventuallyClosed(t, target, 1)
			if received.Load() != 2 || charged.Load() != 2 || target.opened.Load() != 1 || len(pool.slots) != 0 {
				t.Fatalf("failed read was retried or leaked: GETs=%d charges=%d TCP=%d active=%d",
					received.Load(), charged.Load(), target.opened.Load(), len(pool.slots))
			}
		})
	}
}

func TestReadPoolBoundsAndIdleExpiry(t *testing.T) {
	target := poolUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"fixture-task"}`)
	})
	pool := newReadPool(target.clickup)
	pool.idleFor = 250 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	exchanges := make([]*readExchange, 0, maxReadConnections)
	defer func() {
		for _, e := range exchanges {
			e.finish(false)
		}
	}()
	for range maxReadConnections {
		ex, err := poolReadStart(t, ctx, pool, func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		exchanges = append(exchanges, ex)
	}
	blocked, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	if extra, err := poolReadStart(t, blocked, pool, func() error {
		t.Error("saturated pool allowed charging")
		return nil
	}); !errors.Is(err, ErrHeld) || extra != nil || target.opened.Load() != maxReadConnections {
		t.Fatalf("saturated pool acquired a lease: %v, opened=%d", err, target.opened.Load())
	}
	for _, e := range exchanges {
		r := &targetReader{ex: e, id: "fixture-task"}
		if _, _, err := r.Read(ctx, "fixture-task"); err != nil {
			t.Fatal(err)
		}
	}
	pool.mu.Lock()
	idle := len(pool.idle)
	pool.mu.Unlock()
	if idle != maxIdleReads || len(pool.slots) != 0 {
		t.Fatalf("resource bounds not applied: idle=%d active=%d", idle, len(pool.slots))
	}
	eventuallyClosed(t, target, maxReadConnections)
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.idle) != 0 {
		t.Fatal("expired idle connections remain in the pool")
	}
}

func TestPooledReadCancelAtStart(t *testing.T) {
	var received atomic.Int64
	target := poolUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		_, _ = io.WriteString(w, `{"id":"fixture-task"}`)
	})
	pool := newReadPool(target.clickup)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	ex, err := poolReadStart(t, ctx, pool, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	reader := &targetReader{ex: ex, id: "fixture-task"}
	if _, _, err = reader.Read(ctx, "fixture-task"); err != nil {
		t.Fatal(err)
	}
	ex, err = poolReadStart(t, ctx, pool, func() error {
		cancel()
		return nil
	})
	if !errors.Is(err, ErrHeld) || ex != nil {
		t.Fatalf("canceled reused start escaped: %v", err)
	}
	eventuallyClosed(t, target, 1)
	if received.Load() != 1 || target.opened.Load() != 1 || len(pool.slots) != 0 {
		t.Fatalf("canceled GET reached the wire or leaked a lease: GETs=%d TCP=%d active=%d",
			received.Load(), target.opened.Load(), len(pool.slots))
	}
}
