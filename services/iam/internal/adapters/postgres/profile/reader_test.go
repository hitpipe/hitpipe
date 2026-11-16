package profile

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProfileReaderConfigurationAndInput(t *testing.T) {
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
	caller := authorizeaction.Caller{RuntimeID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
	for _, reference := range []string{"", "raw-session-secret", strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if got, err := reader.Read(context.Background(), reference, caller); !errors.Is(err, ErrInvalidRequest) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
			t.Fatal("invalid session selector reached SQL or returned data")
		}
	}
	invalidCaller := caller
	invalidCaller.RuntimeID = "00000000-0000-0000-0000-000000000000"
	if got, err := reader.Read(context.Background(), strings.Repeat("a", 64), invalidCaller); !errors.Is(err, ErrInvalidRequest) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
		t.Fatal("invalid runtime selector reached SQL")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := reader.Read(ctx, strings.Repeat("a", 64), caller); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
		t.Fatal("canceled read returned profile authority")
	}
	if got, err := reader.Read(nil, strings.Repeat("a", 64), caller); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
		t.Fatal("nil context returned data")
	}
	var zero Reader
	if got, err := zero.Read(context.Background(), strings.Repeat("a", 64), caller); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(got, getprofile.Snapshot{}) {
		t.Fatal("unconfigured reader returned data")
	}
}
