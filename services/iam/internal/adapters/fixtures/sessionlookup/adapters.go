package sessionlookup

import (
	"context"
	"errors"
	"os"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

var errLookup = errors.New("lookup_unavailable")

// File reads and strictly decodes its fixture on each Lookup. The reference is
// a synthetic routing key; no production identifier-to-file mapping is implied.
type File struct{ path string }

// NewFile binds one synthetic fixture, with no filesystem side effects.
func NewFile(path string) (*File, error) {
	if path == "" {
		return nil, errLookup
	}
	return &File{path: path}, nil
}

func (f *File) Lookup(ctx context.Context, _ string) (sessionpolicy.Snapshot, error) {
	if f == nil || ctx == nil || ctx.Err() != nil {
		return sessionpolicy.Snapshot{}, errLookup
	}
	file, err := os.Open(f.path)
	if err != nil {
		return sessionpolicy.Snapshot{}, errLookup
	}
	defer file.Close()
	snapshot, _, err := Decode(file)
	if err != nil || ctx.Err() != nil {
		return sessionpolicy.Snapshot{}, errLookup
	}
	return snapshot, nil
}

// Fixed is an immutable in-memory synthetic double. Lookup returns values, never
// caller-owned pointers. Unlike File it performs no IO or freshness check.
type Fixed struct {
	snapshot sessionpolicy.Snapshot
	failed   bool
}

func NewFixed(snapshot sessionpolicy.Snapshot, failed bool) *Fixed {
	return &Fixed{snapshot: snapshot, failed: failed}
}

func (f *Fixed) Lookup(ctx context.Context, _ string) (sessionpolicy.Snapshot, error) {
	if f == nil || ctx == nil || ctx.Err() != nil || f.failed {
		return sessionpolicy.Snapshot{}, errLookup
	}
	return f.snapshot, nil
}

// FixedClock returns the immutable synthetic observation read from a fixture.
// It makes deterministic tests possible; it does not attest host clock health.
type FixedClock struct{ observation sessionpolicy.Observation }

func NewFixedClock(observation sessionpolicy.Observation) *FixedClock {
	return &FixedClock{observation: observation}
}

func (f *FixedClock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	if f == nil || ctx == nil || ctx.Err() != nil {
		return sessionpolicy.Observation{}, errors.New("clock_unavailable")
	}
	return f.observation, nil
}

var _ authenticatesession.SessionLookup = (*File)(nil)
var _ authenticatesession.SessionLookup = (*Fixed)(nil)
var _ authenticatesession.Clock = (*FixedClock)(nil)
