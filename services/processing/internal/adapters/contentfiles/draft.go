package contentfiles

import (
	"context"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"time"
)

// Origin.CaseID is the historical owner-file aggregate ID field. For DRAFT_DATA
// it is exactly the draft UUID; no case_identity row or real order is created.
func (s *Store) ReadDraft(ctx context.Context, ref drafts.Ref, origin preparecase.Origin, now time.Time) ([]byte, error) {
	if s == nil || s.root == nil || ctx == nil || ctx.Err() != nil || now.IsZero() || !validOrigin(origin) || ref.Revision != origin.Revision || ref.Class != "DRAFT_DATA" {
		return nil, drafts.ErrUnavailable
	}
	h, b, e := s.read(ref.ID)
	if e != nil || h.Origin != origin || h.Ref.ID != ref.ID || h.Ref.Class != "DRAFT_DATA" || h.Ref.Bytes != ref.Bytes || h.Ref.Digest != ref.Digest || !h.Ref.ExpiresAt.Equal(ref.ExpiresAt) {
		return nil, drafts.ErrUnavailable
	}
	if !now.Before(ref.ExpiresAt) {
		return nil, drafts.ErrUnavailable
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return b, nil
}
func (s *Store) ReadDraftRoot(ctx context.Context, id, digest string, expiry time.Time, origin preparecase.Origin, now time.Time) ([]byte, error) {
	if s == nil || s.root == nil || ctx == nil || ctx.Err() != nil || now.IsZero() || !validOrigin(origin) || !idPattern.MatchString(id) {
		return nil, drafts.ErrUnavailable
	}
	h, _, e := s.read(id)
	if e != nil || h.Origin != origin || h.Ref.ID != id || h.Ref.Digest != digest || h.Ref.Class != "DRAFT_DATA" || !h.Ref.ExpiresAt.Equal(expiry) {
		return nil, drafts.ErrUnavailable
	}
	return s.ReadDraft(ctx, drafts.Ref{Ref: h.Ref, Revision: origin.Revision}, origin, now)
}
