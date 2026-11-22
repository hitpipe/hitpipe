//go:build integration

package workspaces

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This fixture wraps only its dedicated single-connection pool. It lets real
// PostgreSQL finish COMMIT and drops the confirmed reply before pgx sees it.
type commitFault struct {
	armed        atomic.Bool
	acknowledged atomic.Bool
}

func (fault *commitFault) TraceQueryStart(ctx context.Context, _ *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if query.SQL == "commit" {
		fault.armed.Store(true)
	}
	return ctx
}

func (*commitFault) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (fault *commitFault) frontend(r io.Reader, w io.Writer) *pgproto3.Frontend {
	return pgproto3.NewFrontend(&commitFaultReader{source: r, fault: fault}, w)
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
	tag, committed := message.(*pgproto3.CommandComplete)
	if committed && string(tag.CommandTag) == "COMMIT" {
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

func TestPostgresWorkspacesUnknownCommitRecoversDurableReceipt(t *testing.T) {
	f := isolatedFixture(t)
	ctx, who, change := context.Background(), f.identity(), f.change()
	fault := &commitFault{}
	config := f.iam.Config()
	config.MaxConns = 1
	config.ConnConfig.Tracer = fault
	config.ConnConfig.BuildFrontend = fault.frontend
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("isolated fault pool unavailable")
	}
	t.Cleanup(pool.Close)
	s, err := New(pool, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.Create(ctx, who, change.Change, change.prepare)
	if row != (app.Row{}) || !errors.Is(err, app.ErrUncertain) || !fault.acknowledged.Load() {
		t.Fatalf("confirmed server commit was not reported as uncertain to its caller: %v", err)
	}
	if f.counts(change.WorkspaceID) != [5]int{1, 1, 1, 1, 1} {
		t.Fatal("independent read did not recover exactly one complete durable effect")
	}
	restarted, err := New(f.iam, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	row, err = restarted.Create(ctx, who, change.Change, func(context.Context) (app.Material, error) {
		t.Fatal("a confirmed but unacknowledged commit must not stage another result")
		return app.Material{}, app.ErrUnavailable
	})
	if err != nil || row.WorkspaceID != change.WorkspaceID || row.NameRef != change.material.NameRef || row.Role != "OWNER" {
		t.Fatalf("fresh adapter did not replay the durable receipt after uncertain commit: %v", err)
	}
}
