package stripesignature

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
)

// Independent concatenating reference signs exact binary data, including NUL and
// invalid UTF-8. No normalizer or signature-success cache is introduced.
func TestStreamingMatchesConcatenatedMessage(t *testing.T) {
	for _, size := range []int{1, 63, 64, 65, 4096, 65536, 262144} {
		for _, stamp := range []int64{1, 1800000000, 9223372036854775500} {
			raw := make([]byte, size)
			for i := range raw {
				raw[i] = byte(i * 17)
			}
			key := []byte("synthetic-isolated-key")
			original := append([]byte(nil), raw...)
			msg := append([]byte(fmt.Sprintf("%d.", stamp)), raw...)
			h := hmac.New(sha256.New, key)
			h.Write(msg)
			header := fmt.Sprintf("t=%d,v1=%s", stamp, hex.EncodeToString(h.Sum(nil)))
			if got := Verify(raw, header, key, stamp); got.Status != Verified {
				t.Fatal(size, stamp, got)
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("input mutated")
			}
			raw[len(raw)-1] ^= 1
			if got := Verify(raw, header, key, stamp); got.Status != Rejected {
				t.Fatal("changed raw accepted")
			}
		}
	}
}
func TestStreamingConcurrentRequestIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw := bytes.Repeat([]byte{byte(i)}, 4096)
			key := []byte(fmt.Sprintf("synthetic-key-%d", i))
			h := hmac.New(sha256.New, key)
			h.Write(append([]byte("1800000000."), raw...))
			header := "t=1800000000,v1=" + hex.EncodeToString(h.Sum(nil))
			for j := 0; j < 100; j++ {
				if Verify(raw, header, key, 1800000000).Status != Verified {
					t.Error("cross request contamination")
				}
				if Verify(raw, header, []byte("another-key"), 1800000000).Status != Rejected {
					t.Error("cached success")
				}
			}
		}(i)
	}
	wg.Wait()
}
