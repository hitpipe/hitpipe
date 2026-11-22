//go:build integration

package profileupdate

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The fixture lets PostgreSQL complete a real COMMIT and discards its reply
// on this one synthetic test connection before the adapter can see it.
type commitFault struct {
	armed, acknowledged atomic.Bool
}

func (f *commitFault) TraceQueryStart(ctx context.Context, _ *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if query.SQL == "commit" {
		f.armed.Store(true)
	}
	return ctx
}

func (*commitFault) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (f *commitFault) frontend(r io.Reader, w io.Writer) *pgproto3.Frontend {
	return pgproto3.NewFrontend(&commitFaultReader{source: r, fault: f}, w)
}

type commitFaultReader struct {
	source io.Reader
	fault  *commitFault
}

func (r *commitFaultReader) Read(p []byte) (int, error) {
	if !r.fault.armed.Swap(false) {
		return r.source.Read(p)
	}
	backend := pgproto3.NewFrontend(r.source, io.Discard)
	message, err := backend.Receive()
	if err != nil {
		return 0, err
	}
	if tag, ok := message.(*pgproto3.CommandComplete); ok && string(tag.CommandTag) == "COMMIT" {
		message, err = backend.Receive()
		if err != nil {
			return 0, err
		}
		if ready, ok := message.(*pgproto3.ReadyForQuery); ok && ready.TxStatus == 'I' {
			r.fault.acknowledged.Store(true)
		}
	}
	return 0, io.ErrUnexpectedEOF
}

func TestPostgresProfileUpdateUnknownCommitCanReadDurablePointer(t *testing.T) {
	f := isolatedFixture(t)
	who := f.identity()
	fault := &commitFault{}
	config := f.iam.Config()
	config.MaxConns = 1
	config.ConnConfig.Tracer = fault
	config.ConnConfig.BuildFrontend = fault.frontend
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal("isolated fault pool unavailable")
	}
	t.Cleanup(pool.Close)
	writer, err := New(pool, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	next := nextPointer(t, "e")
	err = writer.WithProfile(t.Context(), who, func(ctx context.Context, tx app.ProfileTx) error {
		return tx.Switch(ctx, f.frozen, next)
	})
	if !errors.Is(err, app.ErrUncertain) || !fault.acknowledged.Load() {
		t.Fatal("uncertain result did not follow a verified real database commit")
	}
	if f.readPointer(who.PrincipalID) != next {
		t.Fatal("independent connection did not observe committed pointer")
	}
	reopened, err := New(f.iam, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	current := f.identity()
	if err := reopened.WithProfile(t.Context(), current, func(_ context.Context, tx app.ProfileTx) error {
		if tx.Pointer() != next {
			t.Fatal("current owner did not see committed pointer after reconnect")
		}
		return nil
	}); err != nil {
		t.Fatalf("authorized reconciliation read failed with %v", err)
	}
	err = reopened.WithProfile(t.Context(), current, func(ctx context.Context, tx app.ProfileTx) error {
		return tx.Switch(ctx, f.frozen, nextPointer(t, "f"))
	})
	if !errors.Is(err, app.ErrStale) || f.readPointer(who.PrincipalID) != next {
		t.Fatal("blind retry changed the already committed result")
	}
}
