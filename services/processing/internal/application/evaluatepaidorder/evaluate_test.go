package evaluatepaidorder_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts"
	app "github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type double struct {
	s     app.Snapshot
	err   error
	calls int
	query app.Query
}

func (d *double) Read(ctx context.Context, q app.Query) (app.Snapshot, error) {
	d.calls++
	d.query = q
	return d.s, d.err
}
func load(t *testing.T) input.Scenario {
	t.Helper()
	s, e := input.Read("../../../testdata/valid.json")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func snapshot(s input.Scenario) app.Snapshot {
	return app.Snapshot{ConfigID: s.Policy.ConfigID, SchemaVersion: s.Policy.SchemaVersion, EvaluatorVersion: s.Policy.EvaluatorVersion, Facts: s.Facts}
}
func request(s input.Scenario) app.Request {
	return app.Request{Policy: s.Policy, CaseID: s.Facts.CaseID, SessionID: s.Facts.SessionID, Now: s.Now}
}
func TestReaderSubstitution(t *testing.T) {
	// Same unchanged consumer and expected result for each adapter and scenario.
	for _, impl := range []string{"file", "double"} {
		for _, tc := range []struct {
			name   string
			change func(*input.Scenario)
			status eligibility.Status
			reason string
		}{
			{"normal", func(s *input.Scenario) {}, eligibility.Valid, "FINANCIAL_ELIGIBILITY_ONLY"},
			{"boundary", func(s *input.Scenario) { s.Now = s.Facts.ValidUntil }, eligibility.RefreshRequired, "PROOF_EXPIRED"},
			{"error_unknown", func(s *input.Scenario) { s.Facts.AmountCaptured = nil }, eligibility.Incomplete, "UNKNOWN_FACT"},
			{"denied", func(s *input.Scenario) { v := int64(1); s.Facts.AmountRefunded = &v }, eligibility.Ineligible, "REFUND"},
		} {
			t.Run(impl+"/"+tc.name, func(t *testing.T) {
				s := load(t)
				tc.change(&s)
				var r app.SourceFactsReader
				if impl == "file" {
					b, e := json.Marshal(s)
					if e != nil {
						t.Fatal(e)
					}
					p := filepath.Join(t.TempDir(), "scenario.json")
					if e = os.WriteFile(p, b, 0600); e != nil {
						t.Fatal(e)
					}
					r = sourcefacts.File{Path: p}
				} else {
					r = &double{s: snapshot(s)}
				}
				got := app.EvaluatePaidOrder(context.Background(), request(s), r)
				if got.Status != tc.status || got.Reason != tc.reason {
					t.Fatalf("got %+v want %s/%s", got, tc.status, tc.reason)
				}
			})
		}
		t.Run(impl+"/reader_failure", func(t *testing.T) {
			s := load(t)
			var r app.SourceFactsReader = sourcefacts.File{Path: filepath.Join(t.TempDir(), "missing")}
			if impl == "double" {
				r = &double{err: errors.New("source offline")}
			}
			got := app.EvaluatePaidOrder(context.Background(), request(s), r)
			if got.Status != eligibility.Incomplete || got.Reason != "READER_UNAVAILABLE" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
func TestContextPins(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		change       func(*app.Snapshot)
	}{
		{"config", "PIN_MISMATCH", func(s *app.Snapshot) { s.ConfigID = "other" }},
		{"schema", "PIN_MISMATCH", func(s *app.Snapshot) { s.SchemaVersion = "2.0" }},
		{"evaluator", "PIN_MISMATCH", func(s *app.Snapshot) { s.EvaluatorVersion = "future" }},
		{"case", "IDENTITY_MISMATCH", func(s *app.Snapshot) { s.Facts.CaseID = "other" }},
		{"session", "IDENTITY_MISMATCH", func(s *app.Snapshot) { s.Facts.SessionID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := load(t)
			d := &double{s: snapshot(s)}
			tc.change(&d.s)
			r := request(s)
			got := app.EvaluatePaidOrder(context.Background(), r, d)
			if got.Status != eligibility.Incomplete || got.Reason != tc.reason || d.calls != 1 {
				t.Fatalf("%+v calls=%d", got, d.calls)
			}
			if d.query.ConfigID != s.Policy.ConfigID || d.query.AccountID != s.Policy.AccountID || d.query.SessionID != s.Facts.SessionID {
				t.Fatal("query lost pinned scope")
			}
		})
	}
}
func TestCanceledAndInvalidContext(t *testing.T) {
	s := load(t)
	d := &double{s: snapshot(s)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := app.EvaluatePaidOrder(ctx, request(s), d)
	if got.Reason != "READER_UNAVAILABLE" || d.calls != 0 {
		t.Fatalf("%+v", got)
	}
	r := request(s)
	r.Now = time.Time{}
	got = app.EvaluatePaidOrder(context.Background(), r, d)
	if got.Reason != "INVALID_CONTEXT" || d.calls != 0 {
		t.Fatalf("%+v", got)
	}
	got = app.EvaluatePaidOrder(context.Background(), request(s), nil)
	if got.Reason != "READER_UNAVAILABLE" {
		t.Fatalf("%+v", got)
	}
}
func TestFinancialScope(t *testing.T) {
	s := load(t)
	d := &double{s: snapshot(s)}
	r := request(s)
	for i := 0; i < 2; i++ {
		got := app.EvaluatePaidOrder(context.Background(), r, d)
		if got.Reason != "FINANCIAL_ELIGIBILITY_ONLY" {
			t.Fatalf("%+v", got)
		}
	}
	if d.calls != 2 {
		t.Fatal("hidden cache/retry")
	}
}
