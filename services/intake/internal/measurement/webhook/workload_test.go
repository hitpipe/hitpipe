package webhook

import (
	"bytes"
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
	"io"
	"sync"
	"testing"
	"time"
)

type broken struct{}

func (broken) Read([]byte) (int, error) { return 0, errors.New("SENSITIVE") }

type counter struct{ n int }

func (c *counter) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	c.n += len(p)
	return len(p), nil
}
func TestBoundedReadingAndFailures(t *testing.T) {
	c := Consumer()
	in := Corpus()[0].Input
	r := &counter{}
	if _, e := FromReader(context.Background(), c, r, in); e == nil || e.Error() != "input_size" || r.n != normalizesignal.MaxBodyBytes+1 {
		t.Fatal("unbounded", r.n, e)
	}
	if _, e := FromReader(context.Background(), c, broken{}, in); e == nil || e.Error() != "read_failed" {
		t.Fatal(e)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, e := FromReader(ctx, c, bytes.NewReader(in.RawBody), in); e == nil {
		t.Fatal("deadline")
	}
	if _, e := FromReader(context.Background(), c, io.LimitReader(bytes.NewReader(in.RawBody), 3), in); e == nil {
		t.Fatal("truncated accepted")
	}
}
func TestCorpusConcurrentIsolation(t *testing.T) {
	c := Consumer()
	cases := Corpus()
	expected := make([]string, len(cases))
	for i, x := range cases {
		expected[i] = Fingerprint(Check(c, x))
	}
	var wg sync.WaitGroup
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i, x := range cases {
				before := Fingerprint(x.Input)
				if Fingerprint(Check(c, x)) != expected[i] || Fingerprint(x.Input) != before {
					t.Error("outcome/input changed", x.Name)
				}
			}
		}()
	}
	wg.Wait()
}
