package verifytarget

import (
	"context"
	"time"

	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

type TargetReader interface {
	Read(ctx context.Context, targetID string) (readback.Observation, Provenance, error)
}

type Provenance struct {
	ObservedAt time.Time `json:"observed_at"`
	Source string `json:"source"`
}

type Result struct {
	TargetID string `json:"target_id"`
	Comparison readback.Result `json:"comparison"`
	Provenance Provenance `json:"provenance"`
}

type Verifier struct {
	reader TargetReader
}

func New(reader TargetReader) Verifier {
	return Verifier{reader: reader}
}

func (v Verifier) VerifyTarget(ctx context.Context, targetID string, expected readback.Expected) Result {
	observed, provenance, err := v.reader.Read(ctx, targetID)
	if err != nil {
		return Result{TargetID: targetID, Comparison: readback.Result{Outcome: readback.Unknown, Reason: "target read failed"}, Provenance: provenance}
	}
	return Result{TargetID: targetID, Comparison: readback.Compare(expected, observed), Provenance: provenance}
}
