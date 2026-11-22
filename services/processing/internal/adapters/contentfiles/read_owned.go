package contentfiles

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

// ReadOwned additionally checks the durable causal owner metadata before exposing
// content. A guessed/ref-swapped ID cannot expose a foreign body.
// The origin revision is a ceiling: context can predate the current case revision.
func (s *Store) ReadOwned(ctx context.Context, ref preparecase.Ref, origin preparecase.Origin, now time.Time) ([]byte, error) {
	if s == nil || s.root == nil || ctx == nil || ctx.Err() != nil || now.IsZero() || !validOrigin(origin) {
		return nil, readcases.ErrUnavailable
	}
	deleted, err := s.deleted(ref, origin)
	if err != nil {
		return nil, readcases.ErrUnavailable
	}
	if deleted {
		return nil, readcases.ErrDeleted
	}
	h, b, e := s.read(ref.ID)
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	if h.Origin.WorkspaceID != origin.WorkspaceID || h.Origin.CaseID != origin.CaseID || h.Origin.Revision > origin.Revision || h.Ref.ID != ref.ID || h.Ref.Class != ref.Class || h.Ref.Digest != ref.Digest || h.Ref.Bytes != ref.Bytes || !h.Ref.ExpiresAt.Equal(ref.ExpiresAt) {
		return nil, readcases.ErrUnavailable
	}
	if !now.Before(ref.ExpiresAt) {
		return nil, readcases.ErrExpired
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return b, nil
}

// Expired reports only authenticated metadata expiry, never corrupt/missing data.
func Expired(e error) bool { return errors.Is(e, readcases.ErrExpired) }
