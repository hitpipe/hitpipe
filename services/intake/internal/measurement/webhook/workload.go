// Package webhook is the HIT-87 synthetic, local measurement harness. It is not
// an HTTP handler, a receipt or an authority source. Fixtures never leave process.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
)

const Now int64 = 1800000000

var syntheticSecret = []byte("HIT-87-public-synthetic-secret")

type ports struct{}

func (ports) Resolve(context.Context, string) ([]byte, error) { return syntheticSecret, nil }
func (ports) NowUnix(context.Context) (int64, error)          { return Now, nil }
func Consumer() normalizesignal.Consumer {
	c, e := normalizesignal.New(ports{}, ports{})
	if e != nil {
		panic(e)
	}
	return c
}

type Case struct {
	Name   string
	Input  normalizesignal.Input
	Accept bool
}

func signature(raw []byte, timestamp int64) string {
	h := hmac.New(sha256.New, syntheticSecret)
	fmt.Fprintf(h, "%d.", timestamp)
	h.Write(raw)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(h.Sum(nil)))
}
func input(kind string, size int) normalizesignal.Input {
	mode := false
	raw := []byte(fmt.Sprintf(`{"id":"evt_synthetic_87","object":"event","account":"acct_synthetic","api_version":"synthetic-version","type":%q,"created":1800000000,"livemode":false,"data":{"object":{"id":"cs_synthetic_87","object":"checkout.session","created":1700000000,"livemode":false,"metadata":{"brief":"SYNTHETIC_DO_NOT_RETAIN"}}}}`, kind))
	// Legal trailing JSON whitespace changes the signed/read byte count without
	// introducing undocumented provider fields. This is not representative traffic.
	if size > len(raw) {
		raw = append(raw, bytes.Repeat([]byte(" "), size-len(raw))...)
	}
	return normalizesignal.Input{RawBody: raw, Signature: signature(raw, Now), Binding: normalizesignal.Binding{InstallationID: "synthetic-installation", WorkspaceID: "11111111-1111-4111-8111-111111111111", ConnectionID: "22222222-2222-4222-8222-222222222222", StripeAccountID: "acct_synthetic", Livemode: &mode, APIVersion: "synthetic-version", AccountScope: normalizesignal.ConnectedAccount}, Receipt: normalizesignal.Receipt{MessageID: "33333333-3333-4333-8333-333333333333", ReceiptID: "44444444-4444-4444-8444-444444444444", FirstReceivedAt: time.Unix(Now-1, 0).UTC(), ExpiresAt: time.Unix(Now-1, 0).Add(72 * time.Hour).UTC()}}
}
func Corpus() []Case {
	var cases []Case
	for _, kind := range []string{"checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed"} {
		for _, n := range []int{512, 4096, 65536, 262143, 262144, 262145} {
			in := input(kind, n)
			cases = append(cases, Case{fmt.Sprintf("%s/%d", kind, n), in, n <= normalizesignal.MaxBodyBytes})
		}
	}
	base := input("checkout.session.completed", 512)
	cases = append(cases, Case{"repeat", base, true})
	for _, n := range []int{512, 4096, 65536, 262144} {
		in := input("checkout.session.completed", n)
		in.Signature = "t=1800000000,v1=" + strings.Repeat("0", 64)
		cases = append(cases, Case{fmt.Sprintf("bad-signature/%d", n), in, false})
	}
	mutations := []struct {
		name string
		f    func(*normalizesignal.Input)
	}{
		{"changed-bytes", func(i *normalizesignal.Input) { i.RawBody = append([]byte(nil), i.RawBody...); i.RawBody[2] = 'X' }},
		{"conflicting-timestamp", func(i *normalizesignal.Input) { i.Signature += ",t=1800000001" }},
		{"unsupported-signature", func(i *normalizesignal.Input) { i.Signature = strings.Replace(i.Signature, "v1=", "v2=", 1) }},
		{"stale", func(i *normalizesignal.Input) { i.Signature = signature(i.RawBody, Now-301) }},
		{"tolerance-300", func(i *normalizesignal.Input) { i.Signature = signature(i.RawBody, Now-300) }},
		{"null-context", func(i *normalizesignal.Input) { i.Binding.Livemode = nil }},
		{"unknown-field", func(i *normalizesignal.Input) {
			i.RawBody = []byte(strings.Replace(string(i.RawBody), `"id":`, `"unknown":true,"id":`, 1))
			i.Signature = signature(i.RawBody, Now)
		}},
		{"duplicate", func(i *normalizesignal.Input) {
			i.RawBody = []byte(strings.Replace(string(i.RawBody), `"id":`, `"id":"duplicate","id":`, 1))
			i.Signature = signature(i.RawBody, Now)
		}},
		{"missing-created", func(i *normalizesignal.Input) {
			i.RawBody = []byte(strings.Replace(string(i.RawBody), `"created":1700000000,`, "", 1))
			i.Signature = signature(i.RawBody, Now)
		}},
		{"null-created", func(i *normalizesignal.Input) {
			i.RawBody = []byte(strings.Replace(string(i.RawBody), `"created":1700000000`, `"created":null`, 1))
			i.Signature = signature(i.RawBody, Now)
		}},
		{"malformed-signed", func(i *normalizesignal.Input) { i.RawBody = []byte(`{"id":`); i.Signature = signature(i.RawBody, Now) }},
		{"empty", func(i *normalizesignal.Input) { i.RawBody = nil; i.Signature = signature(nil, Now) }},
	}
	for _, m := range mutations {
		in := base
		m.f(&in)
		cases = append(cases, Case{m.name, in, m.name == "tolerance-300"})
	}
	return cases
}

// ReadNormalize is identical in both measured versions. io.LimitReader reads at
// most limit+1 to detect overflow. ctx is checked between reads and before parse;
// a blocking transport reader must itself enforce its deadline (no HTTP here).
func ReadNormalize(ctx context.Context, c normalizesignal.Consumer, in normalizesignal.Input) (*normalizesignal.Signal, error) {
	return FromReader(ctx, c, bytes.NewReader(in.RawBody), in)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, errors.New("cancelled")
	}
	return r.r.Read(p)
}
func FromReader(ctx context.Context, c normalizesignal.Consumer, r io.Reader, in normalizesignal.Input) (*normalizesignal.Signal, error) {
	raw, e := io.ReadAll(io.LimitReader(contextReader{ctx, r}, normalizesignal.MaxBodyBytes+1))
	if e != nil {
		return nil, errors.New("read_failed")
	}
	if len(raw) > normalizesignal.MaxBodyBytes {
		return nil, errors.New("input_size")
	}
	in.RawBody = raw
	return c.Normalize(ctx, in)
}

// Outcome contains only the normalized projection or bounded failure, never raw.
type Outcome struct {
	Name     string                  `json:"name"`
	Bytes    int                     `json:"bytes"`
	Accepted bool                    `json:"accepted"`
	Error    string                  `json:"error,omitempty"`
	Signal   *normalizesignal.Signal `json:"signal,omitempty"`
	InputSHA string                  `json:"input_sha256"`
}

func Check(c normalizesignal.Consumer, x Case) Outcome {
	s, e := ReadNormalize(context.Background(), c, x.Input)
	if (e == nil) != x.Accept {
		panic("unexpected outcome: " + x.Name)
	}
	h := sha256.Sum256(x.Input.RawBody)
	o := Outcome{Name: x.Name, Bytes: len(x.Input.RawBody), Accepted: e == nil, Signal: s, InputSHA: hex.EncodeToString(h[:])}
	if e != nil {
		o.Error = e.Error()
	}
	return o
}
func Fingerprint(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
