package sessionlookup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5/pgxpool"
)

// legacyDecodeReference is the previous production acceptance rule. Keeping the
// standard-library implementation here checks compatibility independently of the
// optimized decoder, including rejection of uppercase and partially decoded data.
func legacyDecodeReference(reference string) ([sha256.Size]byte, error) {
	if len(reference) != sha256.Size*2 {
		return [sha256.Size]byte{}, ErrInvalidReference
	}
	digest, err := hex.DecodeString(reference)
	if err != nil || hex.EncodeToString(digest) != reference {
		return [sha256.Size]byte{}, ErrInvalidReference
	}
	var result [sha256.Size]byte
	copy(result[:], digest)
	return result, nil
}

func assertDecodeReferenceEquivalent(t testing.TB, reference string) {
	t.Helper()
	want, wantErr := legacyDecodeReference(reference)
	got, gotErr := decodeReference(reference)
	if gotErr != wantErr || got != want {
		t.Fatalf("decoder disagrees with legacy rule for %d-byte input: got error %v, want %v; digest equality %t",
			len(reference), gotErr, wantErr, got == want)
	}
	if gotErr != nil && got != ([sha256.Size]byte{}) {
		t.Fatal("invalid reference exposed partially decoded digest")
	}
}

func TestDecodeReferenceAcceptsExactLowercaseDigests(t *testing.T) {
	var zero, allOnes, increasing [sha256.Size]byte
	for i := range allOnes {
		allOnes[i] = 0xff
		increasing[i] = byte(i)
	}
	for _, digest := range [][sha256.Size]byte{
		zero, allOnes, increasing,
		sha256.Sum256([]byte("HIT-82 synthetic session")),
		sha256.Sum256([]byte(strings.Repeat("s", 43))),
	} {
		reference := hex.EncodeToString(digest[:])
		got, err := decodeReference(reference)
		if err != nil || got != digest {
			t.Fatal("valid lowercase SHA-256 reference was not decoded exactly")
		}
	}
}

func TestDecodeReferenceRejectsEveryNonLowercaseHexByteAtEveryPosition(t *testing.T) {
	// A nonzero prefix makes partial-result leakage observable when a bad byte
	// appears near the end, rather than testing only an all-zero reference.
	base := []byte(strings.Repeat("f", sha256.Size*2))
	for position := range base {
		for value := 0; value <= 255; value++ {
			b := byte(value)
			if b >= '0' && b <= '9' || b >= 'a' && b <= 'f' {
				continue
			}
			candidate := append([]byte(nil), base...)
			candidate[position] = b
			got, err := decodeReference(string(candidate))
			if err != ErrInvalidReference || got != ([sha256.Size]byte{}) {
				t.Fatalf("invalid byte 0x%02x at position %d must return only the sentinel and zero digest", b, position)
			}
		}
	}
}

func TestDecodeReferenceLengthUnicodeAndFormatting(t *testing.T) {
	valid := strings.Repeat("ab", sha256.Size)
	cases := []string{
		"", "0", strings.Repeat("a", 31), strings.Repeat("a", 32),
		strings.Repeat("a", 63), strings.Repeat("a", 65),
		strings.Repeat("a", 128), strings.Repeat("a", 4096),
		strings.ToUpper(valid), " " + valid, valid + " ",
		valid + "\n", "\t" + valid, "0x" + valid,
		strings.Repeat("é", 32), strings.Repeat("а", 32), // 64 bytes, no ASCII hex.
		strings.Repeat("０", 21) + "a",      // Fullwidth zero: exactly 64 bytes.
		strings.Repeat("a", 62) + "é",      // Invalid multibyte suffix after decoded data.
		strings.Repeat("a", 61) + "\u200b", // Zero-width suffix is not normalized.
		strings.Repeat("a", 63) + "\x00",
	}
	for i, reference := range cases {
		got, err := decodeReference(reference)
		if err != ErrInvalidReference || got != ([sha256.Size]byte{}) {
			t.Fatalf("invalid length/format case %d must return only the sentinel and zero digest", i)
		}
		assertDecodeReferenceEquivalent(t, reference)
	}
}

func TestDecodeReferenceMatchesLegacyDeterministicCorpus(t *testing.T) {
	rng := rand.New(rand.NewSource(82))
	for iteration := 0; iteration < 4096; iteration++ {
		var digest [sha256.Size]byte
		if _, err := rng.Read(digest[:]); err != nil {
			t.Fatal("could not produce deterministic synthetic digest")
		}
		reference := hex.EncodeToString(digest[:])
		assertDecodeReferenceEquivalent(t, reference)
		assertDecodeReferenceEquivalent(t, strings.ToUpper(reference))

		// Include every byte value and varying lengths, not just UTF-8 strings.
		raw := make([]byte, rng.Intn(97))
		if _, err := rng.Read(raw); err != nil {
			t.Fatal("could not produce deterministic synthetic byte input")
		}
		assertDecodeReferenceEquivalent(t, string(raw))

		ascii := make([]byte, sha256.Size*2)
		for i := range ascii {
			ascii[i] = byte(rng.Intn(128))
		}
		assertDecodeReferenceEquivalent(t, string(ascii))

		// Inputs close to valid are especially useful for finding nibble/position
		// mistakes, which overwhelmingly invalid random ASCII would miss.
		mutated := []byte(reference)
		mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		assertDecodeReferenceEquivalent(t, string(mutated))
	}
}

func FuzzDecodeReferenceMatchesLegacy(f *testing.F) {
	for _, seed := range []string{
		"", strings.Repeat("0", 64), strings.Repeat("f", 64),
		strings.Repeat("ab", 32), strings.Repeat("AB", 32),
		strings.Repeat("a", 63) + "\x00", strings.Repeat("é", 32),
		strings.Repeat("a", 63), strings.Repeat("a", 65),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, reference string) {
		assertDecodeReferenceEquivalent(t, reference)
	})
}

func closedBoundedLookup(t *testing.T) *Lookup {
	t.Helper()
	// MinConns=0 keeps construction lazy. Closing before lookup guarantees these
	// validation tests cannot contact a database or depend on network timings.
	config, err := pgxpool.ParseConfig("postgres://localhost:1/hit82_synthetic?sslmode=disable")
	if err != nil {
		t.Fatal("could not parse synthetic pool configuration")
	}
	config.MinConns = 0
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal("could not construct lazy bounded pool")
	}
	lookup, err := New(pool)
	pool.Close()
	if err != nil {
		t.Fatal("bounded pool was rejected")
	}
	return lookup
}

func TestLookupCancellationPrecedesReferenceValidation(t *testing.T) {
	lookup := closedBoundedLookup(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, expire := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer expire()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{
		{cancelled, context.Canceled},
		{expired, context.DeadlineExceeded},
	} {
		for _, reference := range []string{"invalid synthetic reference", strings.Repeat("f", 64)} {
			got, err := lookup.Lookup(tc.ctx, reference)
			if err != tc.want || got != (sessionpolicy.Snapshot{}) {
				t.Fatal("cancelled lookup must return context error and zero snapshot before decoding or querying")
			}
		}
	}
}

func TestLookupReferenceValidationAndUnavailableDoNotExposeInput(t *testing.T) {
	lookup := closedBoundedLookup(t)
	for _, reference := range []string{
		"synthetic-private-session-value-that-is-not-a-reference",
		strings.Repeat("F", 64), strings.Repeat("f", 63) + "!",
	} {
		got, err := lookup.Lookup(context.Background(), reference)
		if err != ErrInvalidReference || got != (sessionpolicy.Snapshot{}) {
			t.Fatal("invalid reference must return the fixed validation sentinel and zero snapshot before querying")
		}
	}
	got, err := lookup.Lookup(context.Background(), strings.Repeat("f", 64))
	if !errors.Is(err, ErrUnavailable) || err != ErrUnavailable || got != (sessionpolicy.Snapshot{}) {
		t.Fatal("closed pool must return only the fixed unavailable sentinel and zero snapshot")
	}
}
