package sessionlookup_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/fixtures/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

func encoded(t *testing.T, s sessionpolicy.Snapshot, o sessionpolicy.Observation) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"version": 1, "session": map[string]any{"found": s.Found, "revoked": s.Revoked, "created_at": s.CreatedAt.Format(time.RFC3339Nano), "expires_at": s.ExpiresAt.Format(time.RFC3339Nano), "idle_expires_at": s.IdleExpiresAt.Format(time.RFC3339Nano)}, "clock": map[string]any{"trusted": o.Trusted, "now": o.At.Format(time.RFC3339Nano)}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSameConsumerContractWithBothAdapters(t *testing.T) {
	created := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	snapshot := sessionpolicy.Snapshot{Found: true, CreatedAt: created, ExpiresAt: created.Add(24 * time.Hour), IdleExpiresAt: created.Add(8 * time.Hour)}
	for _, kind := range []string{"file", "fixed"} {
		for _, scenario := range []string{"normal", "idle equal", "unknown", "revoked", "error"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				s := snapshot
				o := sessionpolicy.Observation{At: created.Add(7 * time.Hour), Trusted: true}
				want := "session_valid"
				switch scenario {
				case "idle equal":
					o.At = s.IdleExpiresAt
					want = "idle_expired"
				case "unknown":
					s.Found = false
					want = "unknown_session"
				case "revoked":
					s.Revoked = true
					want = "revoked_session"
				case "error":
					want = "lookup_unavailable"
				}
				var lookup authenticatesession.SessionLookup
				if kind == "file" {
					path := filepath.Join(t.TempDir(), "fixture.json")
					if err := os.WriteFile(path, encoded(t, s, o), 0600); err != nil {
						t.Fatal(err)
					}
					file, err := sessionlookup.NewFile(path)
					if err != nil {
						t.Fatal(err)
					}
					lookup = file
					if scenario == "error" {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
				} else {
					lookup = sessionlookup.NewFixed(s, scenario == "error")
				}
				// Only the injected adapter varies; Service and its operation are identical.
				consumer, err := authenticatesession.New(lookup, sessionlookup.NewFixedClock(o))
				if err != nil {
					t.Fatal(err)
				}
				result := consumer.Execute(context.Background(), "synthetic")
				if result.Reason != want || result.IsValid() != (scenario == "normal") {
					t.Fatal(result)
				}
				if scenario == "error" && result.State != "unavailable" {
					t.Fatal(result)
				}
			})
		}
	}
}

func TestFileReloadFixedOwnershipAndConcurrency(t *testing.T) {
	created := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	s := sessionpolicy.Snapshot{Found: true, CreatedAt: created, ExpiresAt: created.Add(24 * time.Hour), IdleExpiresAt: created.Add(8 * time.Hour)}
	o := sessionpolicy.Observation{At: created.Add(time.Hour), Trusted: true}
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, encoded(t, s, o), 0600); err != nil {
		t.Fatal(err)
	}
	f, _ := sessionlookup.NewFile(path)
	fixed := sessionlookup.NewFixed(s, false)
	s.Revoked = true
	if err := os.WriteFile(path, encoded(t, s, o), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := f.Lookup(context.Background(), "a")
	if err != nil || !got.Revoked {
		t.Fatal("File cached stale snapshot")
	}
	got, err = fixed.Lookup(context.Background(), "a")
	if err != nil || got.Revoked {
		t.Fatal("Fixed aliased caller data")
	}
	got.Revoked = true
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := fixed.Lookup(context.Background(), "a")
			if e != nil || v.Revoked {
				t.Error("mutable snapshot escaped")
			}
		}()
	}
	wg.Wait()
}

func TestAdapterFailureAndBoundaryInputs(t *testing.T) {
	if _, err := sessionlookup.NewFile(""); err == nil {
		t.Fatal("empty file accepted")
	}
	var f *sessionlookup.File
	var fixed *sessionlookup.Fixed
	var clock *sessionlookup.FixedClock
	if _, e := f.Lookup(context.Background(), "a"); e == nil {
		t.Fatal("nil file")
	}
	if _, e := fixed.Lookup(context.Background(), "a"); e == nil {
		t.Fatal("nil fixed")
	}
	if _, e := clock.Now(context.Background()); e == nil {
		t.Fatal("nil clock")
	}
	for _, data := range []string{"null", "{", `{"version":1,"session":{},"clock":{}}`, strings.Repeat("[", 18) + strings.Repeat("]", 18)} {
		if _, _, e := sessionlookup.Decode(strings.NewReader(data)); e == nil {
			t.Fatal("bad fixture accepted")
		}
	}
	if _, _, e := sessionlookup.Decode(nil); e == nil {
		t.Fatal("nil reader")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := sessionlookup.NewFixed(sessionpolicy.Snapshot{}, false).Lookup(ctx, "a"); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e := sessionlookup.NewFixedClock(sessionpolicy.Observation{}).Now(ctx); e == nil {
		t.Fatal("cancel clock ignored")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	broken, _ := sessionlookup.NewFile(path)
	if _, e := broken.Lookup(context.Background(), "a"); e == nil || strings.Contains(e.Error(), path) {
		t.Fatal("bad file allowed or path leaked")
	}
}
