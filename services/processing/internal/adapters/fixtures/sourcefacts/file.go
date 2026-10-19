// Package sourcefacts implements offline SourceFactsReader using synthetic
// versioned files. It proves no authenticated Intake API/provider properties.
package sourcefacts

import (
	"context"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
)

// File reads a fresh value snapshot on each call; no shared mutable cache.
type File struct{ Path string }

var _ evaluatepaidorder.SourceFactsReader = File{}

// Read loads the declared fixture. Query matching belongs to the consumer.
// Cancellation checked before/after bounded local IO; no network or retries.
func (f File) Read(ctx context.Context, q evaluatepaidorder.Query) (evaluatepaidorder.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return evaluatepaidorder.Snapshot{}, err
	}
	s, err := input.Read(f.Path)
	if err != nil {
		return evaluatepaidorder.Snapshot{}, err
	}
	if err = ctx.Err(); err != nil {
		return evaluatepaidorder.Snapshot{}, err
	}
	return evaluatepaidorder.Snapshot{ConfigID: s.Policy.ConfigID, SchemaVersion: s.Policy.SchemaVersion, EvaluatorVersion: s.Policy.EvaluatorVersion, Facts: s.Facts}, nil
}
