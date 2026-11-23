package dispatch

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This opt-in run includes the real QueryService, TargetReader, VerifyTarget,
// PostgreSQL budget and durable observation on a synthetic HTTPS workload.
func TestHIT94ReadbackBenchmark(t *testing.T) {
	if os.Getenv("HIT94_BENCHMARK") != "1" {
		t.Skip("run tests/run_hit91.ps1 -ReadbackBenchmark")
	}
	const repeats, warmup, requests = 5, 16, 96
	t.Logf("HIT94_ENV go=%s os=%s arch=%s cpu=%d gomaxprocs=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0))
	for _, scenario := range []struct {
		name        string
		concurrency int
		status      int
		knowledge   string
		proof       string
	}{
		{"verified-serial", 1, http.StatusOK, "RESULT_VERIFIED", "READ_OBSERVED"},
		{"verified-parallel", 4, http.StatusOK, "RESULT_VERIFIED", "READ_OBSERVED"},
		{"limited-parallel", 4, http.StatusTooManyRequests, "INCOMPLETE", "READ_RATE_LIMITED"},
	} {
		for repeat := 1; repeat <= repeats; repeat++ {
			t.Run(fmt.Sprintf("%s/%d", scenario.name, repeat), func(t *testing.T) {
				f := setup(t)
				fp := f.budget(t, 1+warmup+requests)
				_, auth, query := knownFixture(t, f, fp)
				targetBody := targetJSON(t, auth, "fixture-task", "")
				var connections, received atomic.Int64
				upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != "/api/v2/task/fixture-task" ||
						r.URL.RawQuery != "" || r.Header.Get("Authorization") != fixtureToken ||
						r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
						t.Error("invalid request on controlled provider")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if scenario.status == http.StatusTooManyRequests {
						w.Header().Set("X-RateLimit-Limit", "100")
						w.Header().Set("X-RateLimit-Remaining", "0")
						w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Minute).Unix()))
						w.Header().Set("Retry-After", "60")
						w.WriteHeader(scenario.status)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(targetBody)
				}))
				upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
					if state == http.StateNew {
						connections.Add(1)
					}
				}
				upstream.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
				upstream.StartTLS()
				t.Cleanup(upstream.Close)
				roots := x509.NewCertPool()
				roots.AddCert(upstream.Certificate())
				origin, err := url.Parse(upstream.URL)
				if err != nil {
					t.Fatal(err)
				}
				service := queryFixture(t, f, auth, fp, clickUpAt(*origin, roots))
				run := func(n int) []int64 {
					durations := make([]int64, n)
					var workers sync.WaitGroup
					for worker := range scenario.concurrency {
						workers.Go(func() {
							for i := worker; i < n; i += scenario.concurrency {
								start := time.Now()
								observation, err := service.QueryKnownTask(t.Context(), query)
								durations[i] = time.Since(start).Microseconds()
								if err != nil || observation.Knowledge != scenario.knowledge || observation.ProofCode != scenario.proof ||
									observation.TaskID == nil || *observation.TaskID != "fixture-task" ||
									observation.Complete != (scenario.status == http.StatusOK) {
									t.Errorf("unexpected observation in sample %d: %s/%s (%v)", i, observation.Knowledge, observation.ProofCode, err)
								}
							}
						})
					}
					workers.Wait()
					return durations
				}
				run(warmup)
				warmConnections := connections.Load()
				begin := time.Now()
				durations := run(requests)
				elapsed := time.Since(begin)
				if received.Load() != warmup+requests {
					t.Fatalf("unexpected number of provider GETs: %d", received.Load())
				}
				sorted := slices.Clone(durations)
				slices.Sort(sorted)
				sample := struct {
					Scenario         string  `json:"scenario"`
					Repeat           int     `json:"repeat"`
					Concurrency      int     `json:"concurrency"`
					Warmup           int     `json:"warmup"`
					Requests         int     `json:"requests"`
					ElapsedUS        int64   `json:"elapsed_us"`
					P50US            int64   `json:"p50_us"`
					P95US            int64   `json:"p95_us"`
					Throughput       float64 `json:"requests_per_second"`
					TCPDuringWarmup  int64   `json:"tcp_warmup"`
					TCPAfterWarmup   int64   `json:"tcp_measured"`
					RequestLatencies []int64 `json:"latency_us"`
				}{scenario.name, repeat, scenario.concurrency, warmup, requests, elapsed.Microseconds(),
					sorted[(50*requests+99)/100-1], sorted[(95*requests+99)/100-1], float64(requests) / elapsed.Seconds(),
					warmConnections, connections.Load() - warmConnections, durations}
				raw, err := json.Marshal(sample)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("HIT94_SAMPLE %s", raw)
			})
		}
	}
}
