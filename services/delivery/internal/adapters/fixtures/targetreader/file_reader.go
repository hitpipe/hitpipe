package targetreader

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hitpipe/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

// FileReader reads a strict saved observation from a local file; it performs no provider I/O.
type FileReader struct {
	Path   string
	Source string
}

type savedObservation struct {
	TargetID    string               `json:"target_id"`
	Observation readback.Observation `json:"observation"`
	ObservedAt  string               `json:"observed_at"`
	Source      string               `json:"source"`
}

// Read validates target identity, retrieval metadata, source and timestamp before returning evidence.
func (r FileReader) Read(ctx context.Context, targetID string) (readback.Observation, verifytarget.Provenance, error) {
	if err := ctx.Err(); err != nil {
		return readback.Observation{}, verifytarget.Provenance{}, err
	}
	data, err := os.ReadFile(r.Path)
	if err != nil {
		return readback.Observation{}, verifytarget.Provenance{Source: r.Source}, err
	}
	var saved savedObservation
	if err := readback.ParseSavedObservation(data, &saved); err != nil {
		return readback.Observation{}, verifytarget.Provenance{Source: r.Source}, err
	}
	if saved.TargetID == "" || saved.TargetID != targetID {
		return readback.Observation{}, verifytarget.Provenance{Source: r.Source}, fmt.Errorf("saved target ID does not match requested target %s", targetID)
	}
	if saved.Source == "" {
		return readback.Observation{}, verifytarget.Provenance{Source: r.Source}, fmt.Errorf("saved observation source is missing")
	}
	saved.Observation.TargetID = saved.TargetID
	if !saved.Observation.Retrieved {
		return saved.Observation, verifytarget.Provenance{Source: r.Source}, fmt.Errorf("saved observation is not retrieved for %s", targetID)
	}
	observedAt, err := time.Parse(time.RFC3339, saved.ObservedAt)
	if err != nil {
		return readback.Observation{}, verifytarget.Provenance{Source: r.Source}, err
	}
	return saved.Observation, verifytarget.Provenance{ObservedAt: observedAt, Source: saved.Source}, nil
}
