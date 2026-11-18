package sessionlookup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReferenceUsesExactOpaqueSecret(t *testing.T) {
	// Synthetic values only; no production cookie or identity is a test input.
	for _, secret := range []string{
		strings.Repeat("a", 43), strings.Repeat("x", 128),
		"_" + strings.Repeat("s", 42), "-" + strings.Repeat("s", 42),
		strings.Repeat("a", 42) + "=", strings.Repeat("a", 42) + "+",
	} {
		got, err := Reference(secret)
		digest := sha256.Sum256([]byte(secret))
		if err != nil || got != hex.EncodeToString(digest[:]) {
			t.Fatal("reference must hash the exact secret without choosing a new cookie encoding")
		}
	}
	left, _ := Reference(strings.Repeat("a", 43))
	right, _ := Reference(strings.Repeat("A", 43))
	if left == right {
		t.Fatal("secret must not be normalized")
	}
	for _, secret := range []string{"", strings.Repeat("x", 42), strings.Repeat("x", 129)} {
		ref, err := Reference(secret)
		if ref != "" || !errors.Is(err, ErrInvalidReference) {
			t.Fatal("invalid length must not produce a usable reference")
		}
	}
}

func TestNewRejectsMissingOrOversizedPool(t *testing.T) {
	if lookup, err := New(nil); lookup != nil || !errors.Is(err, ErrConfiguration) {
		t.Fatal("nil pool was accepted")
	}
	config, err := pgxpool.ParseConfig("postgres://localhost/unreachable_test?sslmode=disable")
	if err != nil {
		t.Fatal("invalid synthetic pool configuration")
	}
	config.MaxConns = 11
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal("could not construct lazy test pool")
	}
	defer pool.Close()
	if lookup, err := New(pool); lookup != nil || !errors.Is(err, ErrConfiguration) {
		t.Fatal("pool above the accepted connection bound was accepted")
	}
}

func TestZeroLookupFailsClosed(t *testing.T) {
	var missing *Lookup
	for _, lookup := range []*Lookup{missing, {}} {
		got, err := lookup.Lookup(context.Background(), "synthetic-reference")
		if got != (sessionpolicy.Snapshot{}) || !errors.Is(err, ErrUnavailable) {
			t.Fatal("unconfigured lookup must not return session data")
		}
	}
}
