package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strings"
	"time"
)

// Adds this producer's actual observation time/completeness to the existing
// owner projection. No Delivery evidence body or provider URL is fetched.
// A fully verified native ID uses ClickUp's documented short internal route.
func (db CaseReadStore) observationView(ctx context.Context, tx pgx.Tx, r readcases.Record, ws string, now time.Time) (*time.Time, bool, *string, error) {
	var at time.Time
	ref := preparecase.Ref{Class: "HISTORY"}
	e := tx.QueryRow(ctx, `SELECT h.at,c.detail_ref::text,c.detail_sha256,c.detail_bytes,c.expires_at FROM hp_processing.safety_history h JOIN hp_processing.history_content c ON (h.workspace_id,h.entry_id)=(c.workspace_id,c.entry_id) WHERE h.workspace_id=$1 AND h.case_id=$2 AND h.reason_code IN('DELIVERY_RESULT_VERIFIED','DELIVERY_TARGET_ID_OBSERVED','DELIVERY_POSSIBLE_EFFECT','DELIVERY_RESULT_MISMATCH','DELIVERY_INCOMPLETE','DELIVERY_KNOWN_NO_EFFECT') ORDER BY h.at DESC,h.entry_id DESC LIMIT 1`, ws, r.Summary.CaseID).Scan(&at, &ref.ID, &ref.Digest, &ref.Bytes, &ref.ExpiresAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, false, nil, nil
	}
	if e != nil || at.After(now) {
		return nil, false, nil, readcases.ErrUnavailable
	}
	at = at.UTC()
	if !now.Before(ref.ExpiresAt) {
		return nil, false, nil, nil
	}
	b, e := db.Content.ReadOwned(ctx, ref, preparecase.Origin{WorkspaceID: ws, CaseID: r.Summary.CaseID, Revision: r.Summary.Revision}, now)
	if e != nil {
		return nil, false, nil, readcases.ErrUnavailable
	}
	var h observationHistory
	if json.Unmarshal(b, &h) != nil || h.CaseID != r.Summary.CaseID || h.Revision < 1 || h.Revision > r.Summary.Revision || !strings.EqualFold(h.Observation.WorkspaceID, ws) || !strings.EqualFold(h.Observation.CaseID, r.Summary.CaseID) || !h.Decision.At.UTC().Truncate(time.Microsecond).Equal(at) || h.Observation.ObservedAt.After(at) {
		return nil, false, nil, readcases.ErrUnavailable
	}
	o := h.Observation
	complete := o.Complete && o.WorkspaceMatches != nil && o.ListMatches != nil && o.AssigneeMatches != nil && o.TitleMatches != nil && o.MarkerMatches != nil && o.ContextMatches != nil && o.EvidenceRef != nil && o.EvidenceExpiresAt != nil && o.EvidenceExpiresAt.After(now)
	observed := o.ObservedAt.UTC()
	var target *string
	if complete && o.Knowledge == "RESULT_VERIFIED" && r.Summary.State == "CONFIRMED" && r.Summary.OutcomeKnowledge == "RESULT_VERIFIED" && o.TaskID != nil && r.KnownTaskID != nil && *o.TaskID == *r.KnownTaskID && regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(*o.TaskID) {
		matched := true
		for _, v := range []*bool{o.WorkspaceMatches, o.ListMatches, o.AssigneeMatches, o.TitleMatches, o.MarkerMatches, o.ContextMatches} {
			if v == nil || !*v {
				matched = false
			}
		}
		if matched {
			url := "https://app.clickup.com/t/" + *o.TaskID
			target = &url
		}
	}
	return &observed, complete, target, nil
}
