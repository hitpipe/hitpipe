package sessionlookup

import (
	"errors"
	"strings"
	"testing"
)

// The sinks keep the production decoder's returned value observable. The
// PostgreSQL benchmarks measure complete Lookup calls separately.
var benchmarkDecodedReference [32]byte
var benchmarkDecodeError error

func BenchmarkReferenceDecode(b *testing.B) {
	for _, tc := range []struct {
		name      string
		reference string
		invalid   bool
	}{
		{"valid", strings.Repeat("ab", 32), false},
		{"uppercase", strings.Repeat("AB", 32), true},
		{"malformed", strings.Repeat("xz", 32), true},
		{"short", strings.Repeat("ab", 31), true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			decoded, err := decodeReference(tc.reference)
			if (tc.invalid && (!errors.Is(err, ErrInvalidReference) || decoded != [32]byte{})) ||
				(!tc.invalid && (err != nil || decoded[0] != 0xab || decoded[31] != 0xab)) {
				b.Fatal("reference decoding changed its canonical semantics")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				benchmarkDecodedReference, benchmarkDecodeError = decodeReference(tc.reference)
			}
		})
	}
}
