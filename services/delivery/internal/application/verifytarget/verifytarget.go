package verifytarget

import (
	"context"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/domain/readback"
)

// TargetReader supplies one read-only observation and its provenance.
type TargetReader interface {
	Read(ctx context.Context, targetID string) (readback.Observation, Provenance, error)
}

// Provenance identifies the observation source and capture time.
type Provenance struct {
	ObservedAt time.Time `json:"observed_at"`
	Source     string    `json:"source"`
}

// Result preserves the requested target, comparison and provenance.
type Result struct {
	TargetID   string               `json:"target_id"`
	Comparison readback.Result      `json:"comparison"`
	Provenance Provenance           `json:"provenance"`
	Observed   readback.Observation `json:"observed"`
}

type Verifier struct {
	reader TargetReader
}

// New constructs the single VerifyTarget consumer around a TargetReader port.
func New(reader TargetReader) Verifier {
	return Verifier{reader: reader}
}

// VerifyTarget reads one target and returns match, mismatch, incomplete or unknown evidence.
// It never writes provider state, retries, emits messages or changes business state.
func (v Verifier) VerifyTarget(ctx context.Context, targetID string, expected readback.Expected) Result {
	if targetID == "" {
		return Result{TargetID: targetID, Comparison: readback.Result{Outcome: readback.Unknown}}
	}
	observed, provenance, err := v.reader.Read(ctx, targetID)
	if err != nil {
		return Result{TargetID: targetID, Comparison: readback.Result{Outcome: readback.Unknown}, Provenance: provenance}
	}
	if observed.TargetID != targetID || provenance.Source == "" || provenance.ObservedAt.IsZero() {
		return Result{TargetID: targetID, Comparison: readback.Result{Outcome: readback.Unknown}, Provenance: provenance}
	}
	return Result{TargetID: targetID, Comparison: readback.Compare(expected, observed), Provenance: provenance, Observed: observed}
}
