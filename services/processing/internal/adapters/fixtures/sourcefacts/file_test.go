package sourcefacts

import (
	"context"
	app "github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"path/filepath"
	"testing"
)

func TestFileReaderBoundaries(t *testing.T) {
	f := File{Path: "../../../../testdata/valid.json"}
	s, e := f.Read(context.Background(), app.Query{})
	if e != nil || s.Facts.AmountTotal == nil || *s.Facts.AmountTotal != 12000 {
		t.Fatalf("%+v %v", s, e)
	}
	// Every read returns fresh storage rather than leaking mutable cached facts.
	*s.Facts.AmountTotal = 1
	s2, e := f.Read(context.Background(), app.Query{})
	if e != nil || *s2.Facts.AmountTotal != 12000 {
		t.Fatal("shared mutable state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = f.Read(ctx, app.Query{}); e == nil {
		t.Fatal("cancellation ignored")
	}
	if _, e = (File{Path: filepath.Join(t.TempDir(), "missing")}).Read(context.Background(), app.Query{}); e == nil {
		t.Fatal("failure ignored")
	}
}
