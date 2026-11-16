package authorization

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReaderConfigurationAndInput(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrConfiguration) {
		t.Fatal("nil pool accepted")
	}
	cfg, err := pgxpool.ParseConfig("postgres://synthetic@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatal("synthetic config invalid")
	}
	cfg.MaxConns = 11
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("synthetic pool invalid")
	}
	defer pool.Close()
	if _, err := New(pool); !errors.Is(err, ErrConfiguration) {
		t.Fatal("oversized pool accepted")
	}
	cfg2 := cfg.Copy()
	cfg2.MaxConns = 5
	bounded, err := pgxpool.NewWithConfig(context.Background(), cfg2)
	if err != nil {
		t.Fatal("bounded pool invalid")
	}
	defer bounded.Close()
	reader, err := New(bounded)
	if err != nil {
		t.Fatal(err)
	}
	base := authorizeaction.ReadRequest{Reference: strings.Repeat("a", 64), WorkspaceID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", RuntimeID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}
	invalid := []func(*authorizeaction.ReadRequest){
		func(r *authorizeaction.ReadRequest) { r.Reference = "raw-session-secret" },
		func(r *authorizeaction.ReadRequest) { r.Reference = strings.Repeat("A", 64) },
		func(r *authorizeaction.ReadRequest) { r.Reference = strings.Repeat("g", 64) },
		func(r *authorizeaction.ReadRequest) { r.WorkspaceID = "" },
		func(r *authorizeaction.ReadRequest) { r.RuntimeID = "00000000-0000-0000-0000-000000000000" },
		func(r *authorizeaction.ReadRequest) { r.BindingGrantID = "foreign-connection-id" },
	}
	for n, mutate := range invalid {
		request := base
		mutate(&request)
		if got, err := reader.Read(context.Background(), request); !errors.Is(err, ErrInvalidRequest) || got.Session.Found || got.Runtime.Found {
			t.Fatalf("invalid request %d reached database", n)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := reader.Read(ctx, base); !errors.Is(err, context.Canceled) || got.Session.Found {
		t.Fatal("canceled input did not fail closed")
	}
	if got, err := reader.Read(nil, base); !errors.Is(err, ErrUnavailable) || got.Session.Found {
		t.Fatal("nil context did not fail closed")
	}
	var zero Reader
	if got, err := zero.Read(context.Background(), base); !errors.Is(err, ErrUnavailable) || got.Session.Found {
		t.Fatal("unconfigured reader returned data")
	}
}
