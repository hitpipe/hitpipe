// webhookbench measures only a synthetic in-process path, never a deployed SLO.
package main

import (
	"context"
	"encoding/json"
	"flag"
	w "github.com/hitpipe/hitpipe/services/intake/internal/measurement/webhook"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"
)

type Sample struct {
	Name           string  `json:"name"`
	Bytes          int     `json:"bytes"`
	Accepted       bool    `json:"accepted"`
	N              int     `json:"n"`
	LatencyNS      []int64 `json:"latency_ns"`
	P50            int64   `json:"p50_ns"`
	P95            int64   `json:"p95_ns"`
	ElapsedNS      int64   `json:"elapsed_ns"`
	Throughput     float64 `json:"events_per_second"`
	BytesPerEvent  float64 `json:"bytes_per_event"`
	AllocsPerEvent float64 `json:"allocs_per_event"`
	OutcomeSHA     string  `json:"outcome_sha256"`
}

func main() {
	n := flag.Int("n", 200, "latency/throughput/memory events per case, each separately")
	profile := flag.String("profile", "", "profile output prefix; separate mixed workload")
	duration := flag.Duration("duration", 3*time.Second, "CPU profile duration")
	flag.Parse()
	if *n < 20 {
		panic("n must be >=20")
	}
	runtime.GOMAXPROCS(1)
	c := w.Consumer()
	cases := w.Corpus()
	ctx := context.Background()
	outcomes := make([]w.Outcome, len(cases))
	for i, x := range cases {
		outcomes[i] = w.Check(c, x)
	}
	if *profile != "" {
		f, e := os.Create(*profile + ".cpu.pprof")
		if e != nil {
			panic(e)
		}
		runtime.MemProfileRate = 1
		if e = pprof.StartCPUProfile(f); e != nil {
			panic(e)
		}
		start := time.Now()
		ops := 0
		for time.Since(start) < *duration {
			for _, x := range cases {
				w.ReadNormalize(ctx, c, x.Input)
				ops++
			}
		}
		pprof.StopCPUProfile()
		f.Close()
		runtime.GC()
		h, e := os.Create(*profile + ".heap.pprof")
		if e != nil {
			panic(e)
		}
		if e = pprof.WriteHeapProfile(h); e != nil {
			panic(e)
		}
		h.Close()
		json.NewEncoder(os.Stdout).Encode(map[string]any{"profile_ops": ops, "elapsed_ns": time.Since(start).Nanoseconds(), "corpus_sha256": w.Fingerprint(outcomes)})
		return
	}
	samples := make([]Sample, 0, len(cases))
	for i, x := range cases {
		for j := 0; j < 10; j++ {
			w.ReadNormalize(ctx, c, x.Input)
		}
		lat := make([]int64, *n)
		for j := range lat {
			t := time.Now()
			w.ReadNormalize(ctx, c, x.Input)
			lat[j] = time.Since(t).Nanoseconds()
		}
		start := time.Now()
		for j := 0; j < *n; j++ {
			w.ReadNormalize(ctx, c, x.Input)
		}
		elapsed := time.Since(start).Nanoseconds()
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		for j := 0; j < *n; j++ {
			w.ReadNormalize(ctx, c, x.Input)
		}
		runtime.ReadMemStats(&b)
		sorted := append([]int64(nil), lat...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		s := Sample{Name: x.Name, Bytes: len(x.Input.RawBody), Accepted: x.Accept, N: *n, LatencyNS: lat, P50: sorted[(*n+1)/2-1], P95: sorted[(95*(*n)+99)/100-1], ElapsedNS: elapsed, Throughput: float64(*n) * 1e9 / float64(elapsed), BytesPerEvent: float64(b.TotalAlloc-a.TotalAlloc) / float64(*n), AllocsPerEvent: float64(b.Mallocs-a.Mallocs) / float64(*n), OutcomeSHA: w.Fingerprint(outcomes[i])}
		samples = append(samples, s)
	}
	if e := json.NewEncoder(os.Stdout).Encode(map[string]any{"go": runtime.Version(), "gomaxprocs": runtime.GOMAXPROCS(0), "corpus_sha256": w.Fingerprint(outcomes), "outcomes": outcomes, "samples": samples}); e != nil {
		panic(e)
	}
}
