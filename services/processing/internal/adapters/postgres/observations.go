package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

// ObservationStore extends the existing schema. It neither dispatches an intent
// nor requires its sender to have received an ACK. Files are Processing-owned.
type ObservationContent interface {
	Write(context.Context, []byte, string, preparecase.Origin, time.Time, time.Time) (preparecase.Ref, error)
	ReadOwned(context.Context, preparecase.Ref, preparecase.Origin, time.Time) ([]byte, error)
}
type ObservationStore struct {
	DB               Beginner
	Content          ObservationContent
	HistoryRetention time.Duration
}

func (db ObservationStore) tx(ctx context.Context, ws string) (pgx.Tx, error) {
	if observations.Missing(db.DB) || observations.Missing(db.Content) || db.HistoryRetention <= 0 || db.HistoryRetention > 90*24*time.Hour {
		return nil, observations.ErrUnavailable
	}
	tx, e := scoped(ctx, db.DB, ws)
	if e != nil {
		return nil, observations.ErrUnavailable
	}
	var ok bool
	e = tx.QueryRow(ctx, `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcreaterole AND NOT rolcreatedb AND pg_has_role(current_user,'hp_processing_app','member') FROM pg_roles WHERE rolname=current_user`).Scan(&ok)
	if e != nil || !ok {
		tx.Rollback(ctx)
		return nil, observations.ErrUnavailable
	}
	return tx, nil
}
func observationReplay(ctx context.Context, tx pgx.Tx, o observations.TaskObservation) (*observations.Receipt, error) {
	var digest, disposition string
	e := tx.QueryRow(ctx, `SELECT digest,disposition FROM hp_processing.inbox WHERE workspace_id=$1 AND message_id=$2`, o.WorkspaceID, o.MessageID).Scan(&digest, &disposition)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, observations.ErrUnavailable
	}
	if digest != o.MessageDigest {
		return nil, observations.ErrConflict
	}
	if disposition != "APPLIED" && disposition != "HELD" {
		return nil, observations.ErrUnavailable
	}
	// Only required Receipt fields are emitted originally, so inbox metadata is
	// sufficient to restore exactly that complete Receipt, even after content TTL.
	return &observations.Receipt{MessageID: o.MessageID, Disposition: disposition}, nil
}
func observationCase(ctx context.Context, tx pgx.Tx, o observations.TaskObservation, g observations.Grant, lock bool) (observations.Snapshot, error) {
	s := observations.Snapshot{WorkspaceID: strings.ToLower(o.WorkspaceID), CaseID: strings.ToLower(o.CaseID)}
	var lineage *string
	var config []byte
	q := `SELECT k.operation_id::text,k.config_id::text,k.owner_user_id::text,k.state,k.outcome_knowledge,k.revision,k.lineage_id::text,k.known_task_id,k.last_confirmed_at,k.ever_dispatch_fenced,a.canonical_config FROM hp_processing.case_identity k JOIN hp_processing.accepted_configs a ON (k.workspace_id,k.operation_id,k.config_id)=(a.workspace_id,a.operation_id,a.config_id) WHERE k.workspace_id=$1 AND k.case_id=$2`
	if lock {
		q += ` FOR UPDATE OF k`
	}
	e := tx.QueryRow(ctx, q, o.WorkspaceID, o.CaseID).Scan(&s.OperationID, &s.ConfigID, &s.OwnerID, &s.State, &s.Outcome, &s.Revision, &lineage, &s.KnownTaskID, &s.LastConfirmedAt, &s.Fenced, &config)
	if errors.Is(e, pgx.ErrNoRows) {
		return s, observations.ErrNotFound
	}
	if e != nil {
		return s, observations.ErrUnavailable
	}
	var cfg struct {
		Destination string `json:"destination_connection_id"`
	}
	if json.Unmarshal(config, &cfg) != nil {
		return s, observations.ErrUnavailable
	}
	s.ConnectionID = cfg.Destination
	if !strings.EqualFold(g.WorkspaceID, s.WorkspaceID) || !strings.EqualFold(g.OwnerID, s.OwnerID) || !strings.EqualFold(g.OperationID, s.OperationID) || !strings.EqualFold(g.ConnectionID, s.ConnectionID) {
		return s, observations.ErrDenied
	}
	if lineage != nil {
		s.LineageID = *lineage
	}
	return s, nil
}
func (db ObservationStore) binding(ctx context.Context, tx pgx.Tx, o observations.TaskObservation, s *observations.Snapshot, now time.Time) error {
	if s.LineageID == "" || !strings.EqualFold(s.LineageID, o.LineageID) {
		return observations.ErrConflict
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2 AND kind='DELIVERY_INTENT')`, s.WorkspaceID, s.LineageID).Scan(&exists); e != nil || !exists {
		return observations.ErrConflict
	}
	// The immutable HIT-79 HISTORY binds lineage to episode after the shorter
	// intent-payload TTL. No read/renewal of expired intent payload is attempted.
	var ref preparecase.Ref
	var at time.Time
	ref.Class = "HISTORY"
	e := tx.QueryRow(ctx, `SELECT c.detail_ref::text,c.detail_sha256,c.detail_bytes,c.expires_at,h.at FROM hp_processing.safety_history h JOIN hp_processing.history_content c ON (h.workspace_id,h.entry_id)=(c.workspace_id,c.entry_id) WHERE h.workspace_id=$1 AND h.case_id=$2 AND h.reason_code='FINANCIAL_ELIGIBILITY_ONLY' AND h.new_state='WORKING' ORDER BY h.at DESC,h.entry_id DESC LIMIT 1`, s.WorkspaceID, s.CaseID).Scan(&ref.ID, &ref.Digest, &ref.Bytes, &ref.ExpiresAt, &at)
	if e != nil || !ref.ExpiresAt.After(now) {
		return observations.ErrUnavailable
	}
	b, e := db.Content.ReadOwned(ctx, ref, preparecase.Origin{WorkspaceID: s.WorkspaceID, CaseID: s.CaseID, Revision: s.Revision}, now)
	if e != nil {
		return observations.ErrUnavailable
	}
	var h struct {
		CaseID   string
		Revision int64
		Scope    preparecase.Scope
		Decision preparecase.Decision
	}
	if json.Unmarshal(b, &h) != nil || h.CaseID != s.CaseID || h.Revision < 1 || h.Revision > s.Revision || h.Scope.WorkspaceID != s.WorkspaceID || h.Scope.OperationID != s.OperationID || h.Scope.ConfigID != s.ConfigID || h.Scope.OwnerID != s.OwnerID || h.Decision.LineageID != s.LineageID || h.Decision.MessageID != s.LineageID || !strings.EqualFold(h.Decision.EpisodeID, o.EpisodeID) || h.Decision.Intent == nil || !h.Decision.At.UTC().Truncate(time.Microsecond).Equal(at) {
		return observations.ErrConflict
	}
	s.EpisodeID = h.Decision.EpisodeID
	var latest string
	e = tx.QueryRow(ctx, `SELECT episode_id::text,started_at FROM hp_processing.episodes WHERE workspace_id=$1 AND case_id=$2 ORDER BY started_at DESC,episode_id DESC LIMIT 1`, s.WorkspaceID, s.CaseID).Scan(&latest, &s.EpisodeStartedAt)
	if e != nil || latest != s.EpisodeID {
		return observations.ErrStale
	}

	var observedRef preparecase.Ref
	observedRef.Class = "HISTORY"
	var received time.Time
	e = tx.QueryRow(ctx, `SELECT h.at,c.detail_ref::text,c.detail_sha256,c.detail_bytes,c.expires_at FROM hp_processing.safety_history h JOIN hp_processing.history_content c ON (h.workspace_id,h.entry_id)=(c.workspace_id,c.entry_id) WHERE h.workspace_id=$1 AND h.case_id=$2 AND h.reason_code IN('DELIVERY_RESULT_VERIFIED','DELIVERY_TARGET_ID_OBSERVED','DELIVERY_POSSIBLE_EFFECT','DELIVERY_RESULT_MISMATCH','DELIVERY_INCOMPLETE','DELIVERY_KNOWN_NO_EFFECT') ORDER BY h.at DESC,h.entry_id DESC LIMIT 1`, s.WorkspaceID, s.CaseID).Scan(&received, &observedRef.ID, &observedRef.Digest, &observedRef.Bytes, &observedRef.ExpiresAt)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return observations.ErrUnavailable
	}
	if e == nil {
		// Once detailed content expires, acceptance time is a conservative upper
		// bound on its observation time; it is never exposed as provider read time.
		s.LatestObservationAt = received
		if observedRef.ExpiresAt.After(now) {
			raw, err := db.Content.ReadOwned(ctx, observedRef, preparecase.Origin{WorkspaceID: s.WorkspaceID, CaseID: s.CaseID, Revision: s.Revision}, now)
			if err != nil {
				return observations.ErrUnavailable
			}
			var previous observationHistory
			if json.Unmarshal(raw, &previous) != nil || previous.CaseID != s.CaseID || !strings.EqualFold(previous.Observation.CaseID, s.CaseID) || previous.Observation.ObservedAt.After(received) {
				return observations.ErrUnavailable
			}
			s.LatestObservationAt = previous.Observation.ObservedAt.UTC().Truncate(time.Microsecond)
		}
	}

	return nil
}
func (db ObservationStore) Load(ctx context.Context, o observations.TaskObservation, g observations.Grant) (observations.Snapshot, error) {
	var z observations.Snapshot
	tx, e := db.tx(ctx, o.WorkspaceID)
	if e != nil {
		return z, e
	}
	defer tx.Rollback(ctx)
	s, e := observationCase(ctx, tx, o, g, false)
	if e != nil {
		return z, e
	}
	s.Replay, e = observationReplay(ctx, tx, o)
	if e != nil {
		return z, e
	}
	if s.Replay == nil {
		if e = db.binding(ctx, tx, o, &s, time.Now()); e != nil {
			return z, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return z, observations.ErrUnavailable
	}
	return s, nil
}

type observationHistory struct {
	CaseID                 string
	Revision               int64
	OldState               string
	OldOutcome, NewOutcome string
	Authority              observations.Grant
	ReceivedAt             time.Time
	Observation            observations.TaskObservation
	Decision               struct {
		EpisodeID, Reason, State string
		At                       time.Time
	}
}

func (db ObservationStore) Commit(ctx context.Context, o observations.TaskObservation, g observations.Grant, expected observations.Snapshot, change observations.Change) (observations.Receipt, error) {
	z := observations.Receipt{}
	tx, e := db.tx(ctx, o.WorkspaceID)
	if e != nil {
		return z, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, o.WorkspaceID+":"+o.MessageID); e != nil {
		return z, observations.ErrUnavailable
	}
	current, e := observationCase(ctx, tx, o, g, true)
	if e != nil {
		return z, e
	}
	replay, e := observationReplay(ctx, tx, o)
	if e != nil {
		return z, e
	}
	if replay != nil {
		if e = tx.Commit(ctx); e != nil {
			return z, observations.ErrUnavailable
		}
		return *replay, nil
	}
	if current.Revision != expected.Revision || current.Revision >= 9007199254740991 {
		return z, observations.ErrStale
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil || !now.Before(g.ExpiresAt) {
		return z, observations.ErrDenied
	}
	if e = db.binding(ctx, tx, o, &current, now); e != nil {
		return z, e
	}
	if o.ObservedAt.After(now) || o.ObservedAt.Before(current.EpisodeStartedAt) {
		return z, observations.ErrInvalid
	}
	// Recompute under the locked current version and DB clock, including expiry.
	change = observations.Decide(current, o, now)
	at := now.UTC().Truncate(time.Microsecond)
	expiry := at.Add(db.HistoryRetention)
	if !expiry.After(now) {
		return z, observations.ErrUnavailable
	}
	id, e := newID()
	if e != nil {
		return z, observations.ErrUnavailable
	}
	h := observationHistory{CaseID: current.CaseID, Revision: current.Revision + 1, OldState: current.State, OldOutcome: current.Outcome, NewOutcome: change.Outcome, Authority: g, ReceivedAt: now, Observation: o}
	h.Decision.EpisodeID = current.EpisodeID
	h.Decision.Reason = change.Reason
	h.Decision.State = change.State
	h.Decision.At = at
	b, e := json.Marshal(h)
	if e != nil {
		return z, observations.ErrUnavailable
	}
	ref, e := db.Content.Write(ctx, b, "HISTORY", preparecase.Origin{WorkspaceID: strings.ToLower(o.WorkspaceID), CaseID: strings.ToLower(o.CaseID), Revision: h.Revision}, expiry, now)
	if e != nil {
		return z, observations.ErrUnavailable
	}
	tag, e := tx.Exec(ctx, `UPDATE hp_processing.case_identity SET state=$3,outcome_knowledge=$4,revision=revision+1,known_task_id=$5,ever_dispatch_fenced=$6,last_confirmed_at=$7 WHERE workspace_id=$1 AND case_id=$2 AND revision=$8`, o.WorkspaceID, o.CaseID, change.State, change.Outcome, change.KnownTaskID, change.Fenced, change.LastConfirmedAt, current.Revision)
	if e != nil || tag.RowsAffected() != 1 {
		return z, observations.ErrStale
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,old_state,new_state,proof_ref) VALUES($1,$2,$3,'SYSTEM',$4,$5,$6,$7,$8)`, o.WorkspaceID, id, o.CaseID, at, change.Reason, current.State, change.State, o.EvidenceRef)
	if e != nil {
		return z, observations.ErrUnavailable
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.history_content(workspace_id,entry_id,detail_ref,detail_sha256,detail_bytes,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, o.WorkspaceID, id, ref.ID, ref.Digest, ref.Bytes, at, ref.ExpiresAt)
	if e != nil {
		return z, observations.ErrUnavailable
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.inbox(workspace_id,message_id,digest,disposition,record_id) VALUES($1,$2,$3,$4,$5)`, o.WorkspaceID, o.MessageID, o.MessageDigest, change.Disposition, id)
	if e != nil {
		return z, observations.ErrConflict
	}
	if ctx.Err() != nil {
		return z, ctx.Err()
	}
	if e = tx.Commit(ctx); e != nil {
		return z, observations.ErrUnavailable
	}
	return observations.Receipt{MessageID: o.MessageID, Disposition: change.Disposition}, nil
}
