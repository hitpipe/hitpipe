package targetreader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hitpipe/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

type FileReader struct {
	Path string
	Source string
}

type savedObservation struct {
	Observation readback.Observation `json:"observation"`
	ObservedAt string `json:"observed_at"`
}

func (r FileReader) Read(ctx context.Context, targetID string) (readback.Observation, verifytarget.Provenance, error) {
	if err := ctx.Err(); err != nil { return readback.Observation{}, verifytarget.Provenance{}, err }
	data, err := os.ReadFile(r.Path)
	if err != nil { return readback.Observation{}, verifytarget.Provenance{Source:r.Source}, err }
	var saved savedObservation
	if err := json.Unmarshal(data, &saved); err != nil { return readback.Observation{}, verifytarget.Provenance{Source:r.Source}, err }
	if !saved.Observation.Retrieved {
		return saved.Observation, verifytarget.Provenance{Source:r.Source}, fmt.Errorf("saved observation is not retrieved for %s", targetID)
	}
	observedAt, err := time.Parse(time.RFC3339, saved.ObservedAt)
	if err != nil { return readback.Observation{}, verifytarget.Provenance{Source:r.Source}, err }
	return saved.Observation, verifytarget.Provenance{ObservedAt:observedAt, Source:r.Source}, nil
}
