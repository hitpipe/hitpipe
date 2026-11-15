package sessionissuer

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/jackc/pgx/v5/pgxpool"
)

func validFixtureRecord() issuesession.Record {
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	return issuesession.Record{Issuer: "https://issuer.invalid", Subject: "synthetic-subject", PrincipalID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", SessionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SessionHash: sha256.Sum256([]byte("synthetic-session")), CSRFHash: sha256.Sum256([]byte("synthetic-csrf")), CreatedAt: now, ExpiresAt: now.Add(time.Hour), IdleExpiresAt: now.Add(time.Minute), Epoch: 7}
}

func TestIssuerConfigurationAndInvalidRecords(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrConfiguration) {
		t.Fatal("nil pool accepted")
	}
	cfg, err := pgxpool.ParseConfig("postgres://synthetic@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatal("synthetic pool configuration")
	}
	cfg.MaxConns = 11
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("synthetic pool construction")
	}
	defer pool.Close()
	if _, err := New(pool); !errors.Is(err, ErrConfiguration) {
		t.Fatal("unbounded pool accepted")
	}
	cfg2 := cfg.Copy()
	cfg2.MaxConns = 5
	pool2, err := pgxpool.NewWithConfig(context.Background(), cfg2)
	if err != nil {
		t.Fatal("synthetic bounded pool construction")
	}
	defer pool2.Close()
	issuer, err := New(pool2)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []func(*issuesession.Record){
		func(r *issuesession.Record) { r.Issuer = "" },
		func(r *issuesession.Record) { r.Subject = "\x00" },
		func(r *issuesession.Record) { r.PrincipalID = "00000000-0000-0000-0000-000000000000" },
		func(r *issuesession.Record) { r.SessionID = "not-a-uuid" },
		func(r *issuesession.Record) { r.SessionHash = [32]byte{} },
		func(r *issuesession.Record) { r.CSRFHash = r.SessionHash },
		func(r *issuesession.Record) { r.ExpiresAt = r.CreatedAt },
		func(r *issuesession.Record) { r.IdleExpiresAt = r.ExpiresAt.Add(time.Second) },
		func(r *issuesession.Record) { r.IdleExpiresAt = r.CreatedAt },
		func(r *issuesession.Record) { r.Epoch = 0 },
		func(r *issuesession.Record) { r.ExpiresAt = r.CreatedAt.Add(25 * time.Hour) },
	}
	for n, mutate := range invalid {
		r := validFixtureRecord()
		mutate(&r)
		if got, err := issuer.Create(context.Background(), r); got != (issuesession.Persisted{}) || !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("invalid record %d reached database", n)
		}
	}
	if got, err := issuer.Create(nil, validFixtureRecord()); got != (issuesession.Persisted{}) || !errors.Is(err, ErrUnavailable) {
		t.Fatal("nil context returned success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := issuer.Create(ctx, validFixtureRecord()); got != (issuesession.Persisted{}) || !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation did not fail closed")
	}
	var zero Issuer
	if got, err := zero.Create(context.Background(), validFixtureRecord()); got != (issuesession.Persisted{}) || !errors.Is(err, ErrUnavailable) {
		t.Fatal("zero issuer returned success")
	}
}
