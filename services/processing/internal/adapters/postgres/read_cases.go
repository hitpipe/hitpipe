package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

var readDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type OwnedContent interface {
	ReadOwned(context.Context, preparecase.Ref, preparecase.Origin, time.Time) ([]byte, error)
}

// CaseReadStore uses only the owner database and files. Presentation has no
// fallback for required fields missing from the predecessor result.
type CaseReadStore struct {
	DB              Beginner
	Content         OwnedContent
	Cursors         readcases.CursorKey
	RecoveryEnabled bool
}

func (db CaseReadStore) Read(ctx context.Context, r readcases.Request, g readcases.Grant) (any, error) {
	if db.DB == nil || db.Content == nil || g.WorkspaceID != r.WorkspaceID || !readcases.UUID(r.WorkspaceID) || r.Kind < readcases.List || r.Kind > readcases.History {
		return nil, readcases.ErrUnavailable
	}
	if r.Kind != readcases.List && !readcases.UUID(r.CaseID) {
		return nil, readcases.ErrInvalid
	}
	if r.Kind == readcases.List && (r.Limit < 1 || r.Limit > 100 || (r.State != "" && !readcases.State(r.State))) {
		return nil, readcases.ErrInvalid
	}
	var p readcases.Page
	var e error
	if r.Cursor != "" {
		p, e = db.Cursors.Decode(r)
		if e != nil {
			return nil, e
		}
	}
	tx, e := db.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, g.WorkspaceID); e != nil {
		return nil, readcases.ErrUnavailable
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&now); e != nil {
		return nil, readcases.ErrUnavailable
	}
	now = now.UTC()
	if p.Snapshot.IsZero() {
		p.Snapshot = now
	}
	if p.Snapshot.After(now) {
		return nil, readcases.ErrInvalid
	}
	var out any
	if r.Kind == readcases.History {
		out, e = db.readHistory(ctx, tx, r, p, now)
	} else {
		out, e = db.readCases(ctx, tx, r, p, now, g)
	}
	if e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, readcases.ErrUnavailable
	}
	return out, nil
}

const caseReadSQL = `SELECT k.case_id::text,k.operation_id::text,k.config_id::text,a.digest,k.state,k.revision,k.owner_user_id::text,k.admitted_at,k.outcome_knowledge,a.canonical_config,h.at,h.new_state,h.reason_code,
 (SELECT e.deadline_at FROM hp_processing.episodes e WHERE e.workspace_id=k.workspace_id AND e.case_id=k.case_id AND e.completed_at IS NULL ORDER BY e.started_at,e.episode_id LIMIT 1),k.known_task_id,k.ever_dispatch_fenced,k.last_confirmed_at
 FROM hp_processing.case_identity k JOIN hp_processing.accepted_configs a ON (k.workspace_id,k.operation_id,k.config_id)=(a.workspace_id,a.operation_id,a.config_id)
 LEFT JOIN LATERAL (SELECT at,new_state,reason_code FROM hp_processing.safety_history WHERE workspace_id=k.workspace_id AND case_id=k.case_id ORDER BY at DESC,entry_id DESC LIMIT 1) h ON true
 WHERE k.workspace_id=$1 AND ($2='' OR k.case_id::text=$2) AND ($3='' OR k.state=$3) AND k.admitted_at<=$4
 AND ($5::timestamptz IS NULL OR (k.admitted_at,k.case_id)>($5::timestamptz,$6::uuid)) ORDER BY k.admitted_at,k.case_id LIMIT $7`

func (db CaseReadStore) readCases(ctx context.Context, tx pgx.Tx, r readcases.Request, p readcases.Page, now time.Time, g readcases.Grant) (any, error) {
	limit := r.Limit
	if r.Kind == readcases.Detail {
		limit = 1
	}
	var after *time.Time
	var id *string
	if !p.After.IsZero() {
		after = &p.After
		id = &p.AfterID
	}
	rows, e := tx.Query(ctx, caseReadSQL, r.WorkspaceID, r.CaseID, r.State, p.Snapshot, after, id, limit+1)
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	var records []readcases.Record
	for rows.Next() {
		var rec readcases.Record
		var at *time.Time
		var state, reason *string
		var deadline *time.Time
		s := &rec.Summary
		e = rows.Scan(&s.CaseID, &s.OperationID, &s.ConfigID, &s.ConfigDigest, &s.State, &s.Revision, &s.OwnerUserID, &s.ReceivedAt, &s.OutcomeKnowledge, &rec.Config, &at, &state, &reason, &deadline, &rec.KnownTaskID, &rec.DispatchFenced, &rec.LastConfirmedAt)
		if e != nil {
			rows.Close()
			return nil, readcases.ErrUnavailable
		}
		if len(records) == limit {
			records = append(records, rec)
			break
		}
		if at != nil && at.After(now) && (s.State == "WORKING" || s.State == "WAITING_DATA") {
			rows.Close()
			return nil, readcases.ErrFutureHistory
		}
		if at == nil || state == nil || reason == nil || *state != s.State || at.Before(s.ReceivedAt) || at.After(now) || s.Revision < 1 || s.Revision > 9007199254740991 {
			rows.Close()
			return nil, readcases.ErrUnavailable
		}
		s.ReceivedAt = s.ReceivedAt.UTC()
		s.UpdatedAt = at.UTC()
		rec.LatestReason = *reason
		if deadline != nil {
			d := deadline.UTC()
			s.DueAt = &d
		}
		canon, e := jcs.Transform(rec.Config)
		if e != nil || intent.Digest(canon) != s.ConfigDigest {
			rows.Close()
			return nil, readcases.ErrUnavailable
		}
		var cfg struct {
			SchemaVersion string   `json:"schema_version"`
			Required      []string `json:"required_context_keys"`
		}
		if json.Unmarshal(rec.Config, &cfg) != nil || cfg.Required == nil || len(cfg.Required) > 20 {
			rows.Close()
			return nil, readcases.ErrUnavailable
		}
		rec.RequiredKeys = cfg.Required
		records = append(records, rec)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	if r.Kind == readcases.Detail && len(records) == 0 {
		return nil, readcases.ErrNotFound
	}
	result := readcases.Cases{Items: []readcases.CaseSummary{}, SnapshotAt: p.Snapshot}
	more := len(records) > limit
	if more {
		records = records[:limit]
	}
	var detail readcases.CaseDetail
	for _, rec := range records {
		if e = db.loadReadContext(ctx, tx, r.WorkspaceID, &rec, now); e != nil {
			return nil, e
		}
		if rec.Summary.State == "WORKING" || rec.Summary.State == "WAITING_DATA" {
			rec.Summary.DueAt = nil
			if !casetimeouts.SourceHeld(rec.LatestReason) {
				source, err := loadTimeoutSource(ctx, tx, r.WorkspaceID, rec.Summary.CaseID, rec.Summary.State)
				if err != nil {
					return nil, readcases.ErrUnavailable
				}
				due, problem := source.Deadline(now)
				if problem == "" {
					problem = source.checkContent(ctx, db.Content, r.WorkspaceID, rec.Summary.CaseID, rec.Summary.Revision, now)
				}
				if problem == "" {
					rec.Summary.DueAt = optionalDue(due.UTC())
				} else {
					rec.LatestReason = problem
				}
			}
		}
		labels, e := readcases.Project(rec)
		if db.RecoveryEnabled {
			available := false
			if r.Kind == readcases.Detail && rec.KnownTaskID != nil && g.RecoverConnection != "" && now.Before(g.RecoverUntil) {
				if c, err := readKnownCase(ctx, tx, r.WorkspaceID, rec.Summary.CaseID, g.PrincipalID, false); err == nil {
					_, err = c.checkLineage(ctx, tx, db.Content, g.RecoverConnection, now)
					available = err == nil
				}
			}
			labels, e = readcases.ProjectRecovery(rec, available)
		}
		if e != nil {
			return nil, e
		}
		if !utf8.ValidString(labels.OrderLabel) || !utf8.ValidString(labels.NextAction) || utf8.RuneCountInString(labels.OrderLabel) > 120 || utf8.RuneCountInString(labels.NextAction) > 512 || labels.Actions == nil || len(labels.Actions) > 6 {
			return nil, readcases.ErrUnavailable
		}
		rec.Summary.OrderLabel = labels.OrderLabel
		rec.Summary.NextAction = labels.NextAction

		observed, observationComplete, target, err := db.observationView(ctx, tx, rec, r.WorkspaceID, now)
		if err != nil {
			return nil, err
		}
		rec.Summary.TargetURL = target
		result.Items = append(result.Items, rec.Summary)
		if r.Kind == readcases.Detail {
			complete := readcases.ContextComplete(rec)
			detail = readcases.CaseDetail{Case: rec.Summary, Context: rec.Context, ContextComplete: complete, ContextExpired: rec.ContextExpired, AvailableActions: labels.Actions}
			// Retain a known historical verification boundary when the stored
			// state/knowledge explicitly identify it. This is not a new observation,
			// current ClickUp health, a verified URL or complete readback attributes.
			if rec.LastConfirmedAt != nil && rec.Summary.State == "CONFIRMED" && rec.Summary.OutcomeKnowledge == "RESULT_VERIFIED" {
				if rec.LastConfirmedAt.After(now) || rec.LastConfirmedAt.Before(rec.Summary.ReceivedAt) {
					return nil, readcases.ErrUnavailable
				}
				at := rec.LastConfirmedAt.UTC()
				detail.LastObservationAt = &at
			}
			if observed != nil {
				detail.LastObservationAt = observed
				detail.ObservationComplete = observationComplete
			}
		}
	}
	if r.Kind == readcases.Detail {
		return detail, nil
	}
	if more {
		last := records[len(records)-1].Summary
		next, e := db.Cursors.Encode(r, readcases.Page{Snapshot: p.Snapshot, After: last.ReceivedAt, AfterID: last.CaseID})
		if e != nil {
			return nil, e
		}
		result.NextCursor = &next
	}
	return result, nil
}

func (db CaseReadStore) loadReadContext(ctx context.Context, tx pgx.Tx, ws string, rec *readcases.Record, now time.Time) error {
	rows, e := tx.Query(ctx, `SELECT field_key,value_ref::text,value_sha256,serialized_value_bytes,added_at,expires_at FROM hp_processing.case_content WHERE workspace_id=$1 AND case_id=$2 ORDER BY field_key`, ws, rec.Summary.CaseID)
	if e != nil {
		return readcases.ErrUnavailable
	}
	type field struct {
		key string
		ref preparecase.Ref
		at  time.Time
	}
	var fields []field
	for rows.Next() {
		var f field
		f.ref.Class = "CASE_CONTEXT"
		if e = rows.Scan(&f.key, &f.ref.ID, &f.ref.Digest, &f.ref.Bytes, &f.at, &f.ref.ExpiresAt); e != nil {
			rows.Close()
			return readcases.ErrUnavailable
		}
		fields = append(fields, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(fields) > 20 {
		return readcases.ErrUnavailable
	}
	allowed := map[string]bool{}
	for _, k := range rec.RequiredKeys {
		if allowed[k] {
			return readcases.ErrUnavailable
		}
		allowed[k] = true
	}
	rec.Context = []readcases.ContextField{}
	total := 2
	for _, f := range fields {
		if !allowed[f.key] || f.at.After(now) || !f.ref.ExpiresAt.After(f.at) {
			return readcases.ErrUnavailable
		}
		if !now.Before(f.ref.ExpiresAt) {
			rec.ContextExpired = true
			continue
		}
		b, e := db.Content.ReadOwned(ctx, f.ref, preparecase.Origin{WorkspaceID: ws, CaseID: rec.Summary.CaseID, Revision: rec.Summary.Revision}, now)
		if errors.Is(e, readcases.ErrDeleted) {
			continue
		}
		if errors.Is(e, readcases.ErrExpired) {
			rec.ContextExpired = true
			continue
		}
		if e != nil {
			return readcases.ErrUnavailable
		}
		var value string
		if json.Unmarshal(b, &value) != nil || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 32768 {
			return readcases.ErrUnavailable
		}
		total += len(b) + len(f.key) + 4
		if total > 32769 {
			return readcases.ErrUnavailable
		}
		rec.Context = append(rec.Context, readcases.ContextField{Key: f.key, Value: value, AddedAt: f.at.UTC(), ExpiresAt: f.ref.ExpiresAt.UTC()})
	}
	return nil
}

func (db CaseReadStore) readHistory(ctx context.Context, tx pgx.Tx, r readcases.Request, p readcases.Page, now time.Time) (any, error) {
	var revision int64
	var future bool
	e := tx.QueryRow(ctx, `SELECT k.revision,k.state IN('WORKING','WAITING_DATA') AND EXISTS(
 SELECT 1 FROM hp_processing.safety_history h WHERE h.workspace_id=k.workspace_id AND h.case_id=k.case_id AND h.at>$3)
 FROM hp_processing.case_identity k WHERE k.workspace_id=$1 AND k.case_id=$2`, r.WorkspaceID, r.CaseID, now).Scan(&revision, &future)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, readcases.ErrNotFound
	}
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	if future {
		return nil, readcases.ErrFutureHistory
	}
	var after *time.Time
	var id *string
	if !p.After.IsZero() {
		after = &p.After
		id = &p.AfterID
	}
	rows, e := tx.Query(ctx, `SELECT h.entry_id::text,h.case_id::text,h.at,h.actor_kind,h.actor_id::text,h.old_state,h.new_state,h.reason_code,h.proof_ref::text,c.detail_ref::text,c.detail_sha256,c.detail_bytes,c.expires_at,c.created_at
 FROM hp_processing.safety_history h LEFT JOIN hp_processing.history_content c ON (h.workspace_id,h.entry_id)=(c.workspace_id,c.entry_id)
 WHERE h.workspace_id=$1 AND h.case_id=$2 AND h.at<=$3 AND ($4::timestamptz IS NULL OR (h.at,h.entry_id)>($4::timestamptz,$5::uuid)) ORDER BY h.at,h.entry_id LIMIT 101`, r.WorkspaceID, r.CaseID, p.Snapshot, after, id)
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	type source struct {
		entry       readcases.HistoryEntry
		refID, hash *string
		bytes       *int
		expiry      *time.Time
		created     *time.Time
		newState    *string
	}
	var all []source
	for rows.Next() {
		var s source
		h := &s.entry
		e = rows.Scan(&h.EntryID, &h.CaseID, &h.At, &h.ActorKind, &h.ActorID, &h.OldState, &s.newState, &h.ReasonCode, &h.EvidenceRef, &s.refID, &s.hash, &s.bytes, &s.expiry, &s.created)
		if e != nil {
			rows.Close()
			return nil, readcases.ErrUnavailable
		}
		all = append(all, s)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, readcases.ErrUnavailable
	}
	out := readcases.HistoryView{Items: []readcases.HistoryEntry{}}
	more := len(all) > 100
	if more {
		all = all[:100]
	}
	if len(all) == 0 && r.Cursor == "" {
		return nil, readcases.ErrUnavailable
	}
	for _, s := range all {
		h := s.entry
		if s.newState == nil || !readcases.State(*s.newState) || s.refID == nil || s.hash == nil || s.bytes == nil || s.expiry == nil {
			return nil, readcases.ErrUnavailable
		}
		if s.created == nil || !s.created.Equal(h.At) || !s.expiry.After(*s.created) || s.expiry.After(s.created.Add(90*24*time.Hour)) || !readcases.UUID(*s.refID) || !readDigest.MatchString(*s.hash) || *s.bytes < 1 || *s.bytes > 65536 {
			return nil, readcases.ErrUnavailable
		}
		h.At = h.At.UTC()
		h.NewState = *s.newState
		h.ExpiresAt = s.expiry.UTC()
		if !now.Before(*s.expiry) {
			out.Items = append(out.Items, h)
			continue
		}
		ref := preparecase.Ref{ID: *s.refID, Digest: *s.hash, Class: "HISTORY", Bytes: *s.bytes, ExpiresAt: *s.expiry}
		b, e := db.Content.ReadOwned(ctx, ref, preparecase.Origin{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, Revision: revision}, now)
		if errors.Is(e, readcases.ErrDeleted) {
			out.Items = append(out.Items, h)
			continue
		}
		if e != nil {
			return nil, readcases.ErrUnavailable
		}
		// This is the actual HIT-79 history envelope, not a new event writer. Missing
		// source metadata (notably HIT-78 initial expiry) fails closed, never at+TTL.
		var envelope struct {
			CaseID   string
			Revision int64
			Decision struct {
				EpisodeID string
				Reason    string
				State     string
				At        time.Time
			}
		}
		if json.Unmarshal(b, &envelope) != nil || envelope.CaseID != r.CaseID || envelope.Revision < 1 || envelope.Revision > revision || envelope.Decision.Reason != h.ReasonCode || envelope.Decision.State != *s.newState || !envelope.Decision.At.UTC().Truncate(time.Microsecond).Equal(h.At) {
			return nil, readcases.ErrUnavailable
		}
		if envelope.Decision.EpisodeID != "" {
			if !readcases.UUID(envelope.Decision.EpisodeID) {
				return nil, readcases.ErrUnavailable
			}
			var belongs bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_processing.episodes WHERE workspace_id=$1 AND case_id=$2 AND episode_id=$3)`, r.WorkspaceID, r.CaseID, envelope.Decision.EpisodeID).Scan(&belongs); e != nil || !belongs {
				return nil, readcases.ErrUnavailable
			}
			h.EpisodeID = &envelope.Decision.EpisodeID
		}
		h.At = h.At.UTC()
		h.NewState = *s.newState
		h.ExpiresAt = s.expiry.UTC()
		out.Items = append(out.Items, h)
	}
	if more {
		last := out.Items[len(out.Items)-1]
		next, e := db.Cursors.Encode(r, readcases.Page{Snapshot: p.Snapshot, After: last.At, AfterID: last.EntryID})
		if e != nil {
			return nil, e
		}
		out.NextCursor = &next
	}
	return out, nil
}
