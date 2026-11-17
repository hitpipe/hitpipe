//go:build integration

package receipt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This loopback-only fixture forwards PostgreSQL's actual protocol. It drops the
// second COMMIT CommandComplete AFTER the server has committed, not a fabricated
// tx return. First COMMIT belongs to Lookup, second to the receipt transaction.
func TestIntegrationActualLostPostgresCommitResponse(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	cfg := f.app.Config().Copy()
	backend := net.JoinHostPort(cfg.ConnConfig.Host, fmtPort(cfg.ConnConfig.Port))
	cfg.ConnConfig.Host = "127.0.0.1"
	cfg.ConnConfig.Port = uint16(listener.Addr().(*net.TCPAddr).Port)
	cfg.MaxConns = 1
	var dropped atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		client, e := listener.Accept()
		if e != nil {
			return
		}
		defer client.Close()
		server, e := net.DialTimeout("tcp", backend, 2*time.Second)
		if e != nil {
			return
		}
		defer server.Close()
		client.SetDeadline(time.Now().Add(15 * time.Second))
		server.SetDeadline(time.Now().Add(15 * time.Second))
		var commits atomic.Int32
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer server.Close()
			var header [4]byte
			if _, e := io.ReadFull(client, header[:]); e != nil {
				return
			}
			length := binary.BigEndian.Uint32(header[:])
			if length < 4 || length > 65536 {
				return
			}
			body := make([]byte, int(length)-4)
			if _, e := io.ReadFull(client, body); e != nil {
				return
			}
			if _, e := server.Write(append(header[:], body...)); e != nil {
				return
			}
			for {
				kind, body, e := pgPacket(client)
				if e != nil {
					return
				}
				if kind == 'Q' && string(body) == "commit\x00" {
					commits.Add(1)
				}
				if pgWrite(server, kind, body) != nil {
					return
				}
			}
		}()
		for {
			kind, body, e := pgPacket(server)
			if e != nil {
				break
			}
			if kind == 'C' && string(body) == "COMMIT\x00" && commits.Load() == 2 {
				dropped.Store(true)
				break
			}
			if pgWrite(client, kind, body) != nil {
				break
			}
		}
		client.Close()
		server.Close()
		<-done
	}()
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	repo, e := New(pool)
	if e != nil {
		t.Fatal(e)
	}
	c, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, repo, f.files)
	if e != nil {
		t.Fatal(e)
	}
	d := f.delivery("evt_wire_commit_loss", "checkout.session.completed")
	result, e := c.Receive(ctx, d)
	if result != nil || !errors.Is(e, receivesignal.ErrUncertain) {
		t.Fatal("lost wire commit was not uncertain", e)
	}
	listener.Close()
	wg.Wait()
	if !dropped.Load() {
		t.Fatal("COMMIT response was not actually dropped")
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 1 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("actual commit not preserved")
	}
	retry, e := f.consumer.Receive(ctx, d)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.consumer.Receive(ctx, d)
	if e != nil || *again != *retry {
		t.Fatal("wire loss recovery duplicate", e)
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("duplicate after lost response")
	}
	if n, e := f.consumer.Cleanup(ctx, f.connection); e != nil || n != 0 {
		t.Fatal("accepted file purged", e)
	}
}
func pgPacket(r io.Reader) (byte, []byte, error) {
	var h [5]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return 0, nil, e
	}
	n := binary.BigEndian.Uint32(h[1:])
	if n < 4 || n > 1024*1024 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, int(n)-4)
	_, e := io.ReadFull(r, b)
	return h[0], b, e
}
func pgWrite(w io.Writer, kind byte, b []byte) error {
	h := make([]byte, 5+len(b))
	h[0] = kind
	binary.BigEndian.PutUint32(h[1:5], uint32(len(b)+4))
	copy(h[5:], b)
	for len(h) > 0 {
		n, e := w.Write(h)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		h = h[n:]
	}
	return nil
}
func fmtPort(p uint16) string { return fmt.Sprintf("%d", p) }
