package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

var ErrTimeoutLease = errors.New("case timer lease is stale")
var ErrTimeoutTimeline = errors.New(casetimeouts.HistoryInvalid)

const timeoutID = "md5('HIT-114/CASE_TIMEOUT/' || k.workspace_id::text || '/' || k.case_id::text)::uuid"
const timerCases = ` FROM hp_processing.case_identity k
 LEFT JOIN LATERAL (
  SELECT entry_id,at,new_state,reason_code FROM hp_processing.safety_history
  WHERE workspace_id=k.workspace_id AND case_id=k.case_id AND old_state IS DISTINCT FROM new_state
  ORDER BY at DESC,entry_id DESC LIMIT 1
 ) h ON true
 LEFT JOIN hp_processing.history_content hc ON (hc.workspace_id,hc.entry_id)=(k.workspace_id,h.entry_id)
 LEFT JOIN LATERAL (
   SELECT episode_id,started_at,deadline_at,count(*) OVER() AS open_count FROM hp_processing.episodes
   WHERE workspace_id=k.workspace_id AND case_id=k.case_id AND completed_at IS NULL AND k.state='WORKING'
   ORDER BY started_at DESC,episode_id DESC LIMIT 1
 ) ep ON true
`

// A diagnostic is more recent than the entry into the current state. It holds
// that period and is not itself a new entry or a source for a deadline.
const timerEligible = `
 k.workspace_id=$1 AND k.owner_user_id=$2 AND k.state IN ('WORKING','WAITING_DATA')
 AND NOT EXISTS (
  SELECT 1 FROM hp_processing.safety_history d
  WHERE d.workspace_id=k.workspace_id AND d.case_id=k.case_id AND d.old_state=k.state AND d.new_state=k.state
    AND d.reason_code LIKE 'CASE\_TIMER\_%' AND (h.at IS NULL OR d.at>=h.at)
 )
`

type CaseTimeoutStore struct {
	DB      Beginner
	Files   preparecase.Content
	History OwnedContent
	Clock   preparecase.Clock
}

type TimeoutClaim struct {
	WorkspaceID, OwnerID, CaseID, State string
	Revision                            int64
	jobID, token                        string
	until                               time.Time
}

type TimeoutResult struct {
	CaseID, State, Reason string
}

func (s CaseTimeoutStore) now() time.Time { return s.Clock.Now().UTC().Truncate(time.Microsecond) }

func (s CaseTimeoutStore) Index(ctx context.Context, ws, owner string, limit int) error {
	if limit < 1 || limit > 100 || s.Clock == nil {
		return preparecase.ErrHold
	}
	now := s.now()
	if now.IsZero() {
		return preparecase.ErrHold
	}
	tx, err := (DispatchStore{DB: s.DB}).tx(ctx, ws)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Terminal cases can change while an unclaimed item is indexed. Bounded
	// cleanup drops metadata only, and leaves a competing live lease alone.
	_, err = tx.Exec(ctx, `WITH obsolete AS (
 SELECT j.job_id FROM hp_processing.job_index j
 WHERE j.workspace_id=$1 AND j.kind='CASE_TIMEOUT' AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
   AND NOT EXISTS (SELECT 1`+timerCases+` WHERE `+timerEligible+` AND `+timeoutID+`=j.job_id)
 ORDER BY j.due_at,j.job_id LIMIT $3 FOR UPDATE OF j SKIP LOCKED
 ) DELETE FROM hp_processing.job_index j USING obsolete WHERE j.job_id=obsolete.job_id`, ws, owner, limit)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.job_index(job_id,workspace_id,kind,due_at)
 SELECT `+timeoutID+`,k.workspace_id,'CASE_TIMEOUT',scheduled.at`+timerCases+`
 CROSS JOIN LATERAL (SELECT CASE
  WHEN h.entry_id IS NULL OR h.new_state<>k.state OR h.at<k.admitted_at OR h.at>$3
    OR hc.entry_id IS NULL OR hc.expires_at<=$3 THEN $3::timestamptz
  WHEN k.state='WORKING' AND (COALESCE(ep.open_count,0)<>1 OR ep.started_at<k.admitted_at
    OR ep.started_at>h.at OR ep.deadline_at<=ep.started_at OR ep.deadline_at<h.at
    OR ep.deadline_at>ep.started_at+$5::bigint*interval '1 second') THEN $3::timestamptz
  WHEN k.state='WORKING' THEN ep.deadline_at
  ELSE h.at+$6::bigint*interval '1 second' END AS at) scheduled
 WHERE `+timerEligible+`
  AND scheduled.at<=$3 AND NOT EXISTS (SELECT 1 FROM hp_processing.job_index j WHERE j.job_id=`+timeoutID+`)
 ORDER BY scheduled.at,k.case_id LIMIT $4 ON CONFLICT(job_id) DO NOTHING`, ws, owner, now, limit, int64(casetimeouts.WorkingLimit/time.Second), int64(casetimeouts.WaitingLimit/time.Second))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s CaseTimeoutStore) Claim(ctx context.Context, ws, owner string) (TimeoutClaim, error) {
	var c TimeoutClaim
	c.WorkspaceID, c.OwnerID = ws, owner
	if s.Clock == nil || s.now().IsZero() {
		return TimeoutClaim{}, preparecase.ErrHold
	}
	var err error
	if c.token, err = newID(); err != nil {
		return TimeoutClaim{}, err
	}
	tx, err := (DispatchStore{DB: s.DB}).tx(ctx, ws)
	if err != nil {
		return TimeoutClaim{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT j.job_id::text,k.case_id::text,k.state,k.revision
 FROM hp_processing.job_index j JOIN hp_processing.case_identity k
 ON k.workspace_id=j.workspace_id AND `+timeoutID+`=j.job_id
 WHERE j.workspace_id=$1 AND k.owner_user_id=$2 AND j.kind='CASE_TIMEOUT'
 AND k.state IN ('WORKING','WAITING_DATA') AND j.due_at<=$3
 AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
 ORDER BY j.due_at,j.job_id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`, ws, owner, s.now()).Scan(&c.jobID, &c.CaseID, &c.State, &c.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return TimeoutClaim{}, nil
	}
	if err != nil {
		return TimeoutClaim{}, err
	}
	err = tx.QueryRow(ctx, `UPDATE hp_processing.job_index SET claim_token=$3,lease_until=clock_timestamp()+interval '10 seconds'
 WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_TIMEOUT' RETURNING lease_until`, ws, c.jobID, c.token).Scan(&c.until)
	if err != nil {
		return TimeoutClaim{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TimeoutClaim{}, err
	}
	return c, nil
}

type timeoutSource struct {
	casetimeouts.Source
	ref               preparecase.Ref
	created           time.Time
	reason, episodeID string
	lineage           string
}

func loadTimeoutSource(ctx context.Context, tx pgx.Tx, ws, caseID string, state string) (timeoutSource, error) {
	var t timeoutSource
	t.State = state
	var entered, start, deadline, expiry, created *time.Time
	var enteredState, reason, ep, lineage, fileID, digest *string
	var bytes *int
	err := tx.QueryRow(ctx, `SELECT k.admitted_at,h.at,h.new_state,h.reason_code,
 ep.episode_id::text,ep.started_at,ep.deadline_at,COALESCE(ep.open_count,0),k.lineage_id::text,
 hc.detail_ref::text,hc.detail_sha256,hc.detail_bytes,hc.expires_at,hc.created_at`+timerCases+
		` WHERE k.workspace_id=$1 AND k.case_id=$2 AND k.state=$3`, ws, caseID, state).
		Scan(&t.Admitted, &entered, &enteredState, &reason, &ep, &start, &deadline, &t.OpenEpisodes, &lineage,
			&fileID, &digest, &bytes, &expiry, &created)
	if err != nil {
		return t, err
	}
	if entered != nil {
		t.Entered = *entered
	}
	if enteredState != nil {
		t.EnteredState = *enteredState
	}
	if reason != nil {
		t.reason = *reason
	}
	if ep != nil {
		t.episodeID = *ep
	}
	if start != nil {
		t.EpisodeStarted = *start
	}
	if deadline != nil {
		t.EpisodeDeadline = *deadline
	}
	if lineage != nil {
		t.lineage = *lineage
	}
	if fileID != nil && digest != nil && bytes != nil && expiry != nil && created != nil {
		t.ref = preparecase.Ref{ID: *fileID, Digest: *digest, Bytes: *bytes, Class: "HISTORY", ExpiresAt: *expiry}
		t.created = *created
	}
	return t, nil
}

func (t timeoutSource) checkContent(ctx context.Context, files OwnedContent, ws, id string, rev int64, now time.Time) string {
	if t.ref.ID == "" {
		return casetimeouts.HistoryMissing
	}
	if !t.created.Equal(t.Entered) || !t.ref.ExpiresAt.After(t.Entered) ||
		t.ref.ExpiresAt.After(t.Entered.Add(90*24*time.Hour)) {
		return casetimeouts.HistoryInvalid
	}
	if !now.Before(t.ref.ExpiresAt) {
		return casetimeouts.HistoryExpired
	}
	b, err := files.ReadOwned(ctx, t.ref, preparecase.Origin{WorkspaceID: ws, CaseID: id, Revision: rev}, now)
	var h struct {
		CaseID   string
		Revision int64
		Decision preparecase.Decision
	}
	if err != nil || json.Unmarshal(b, &h) != nil || h.CaseID != id || h.Revision < 1 || h.Revision >= rev ||
		h.Decision.State != t.State || h.Decision.Reason != t.reason || !h.Decision.At.UTC().Truncate(time.Microsecond).Equal(t.Entered) {
		return casetimeouts.HistoryInvalid
	}
	if t.State == "WORKING" && (h.Decision.EpisodeID != t.episodeID || !h.Decision.DeadlineAt.UTC().Truncate(time.Microsecond).Equal(t.EpisodeDeadline) ||
		t.lineage == "" || h.Decision.LineageID != t.lineage || h.Decision.MessageID != t.lineage || h.Decision.Intent == nil) {
		return casetimeouts.EpisodeInvalid
	}
	return ""
}

// Apply owns only the timer lease, case revision and that case's current
// history. It never writes outcome knowledge, task identity or a new intent.
func (s CaseTimeoutStore) Apply(ctx context.Context, c TimeoutClaim) (TimeoutResult, error) {
	if s.Clock == nil || s.Files == nil || s.History == nil {
		return TimeoutResult{}, preparecase.ErrHold
	}
	tx, err := (DispatchStore{DB: s.DB}).tx(ctx, c.WorkspaceID)
	if err != nil {
		return TimeoutResult{}, err
	}
	defer tx.Rollback(ctx)
	var one int
	err = tx.QueryRow(ctx, `SELECT 1 FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_TIMEOUT'
 AND claim_token=$3 AND lease_until=$4 AND lease_until>clock_timestamp() FOR UPDATE`, c.WorkspaceID, c.jobID, c.token, c.until).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return TimeoutResult{}, ErrTimeoutLease
	}
	if err != nil {
		return TimeoutResult{}, err
	}

	var state string
	var rev int64
	err = tx.QueryRow(ctx, `SELECT state,revision FROM hp_processing.case_identity k WHERE workspace_id=$1
 AND case_id=$2 AND owner_user_id=$3 AND `+timeoutID+`=$4 FOR UPDATE`, c.WorkspaceID, c.CaseID, c.OwnerID, c.jobID).Scan(&state, &rev)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return TimeoutResult{}, err
	}
	var result TimeoutResult
	if err == nil && state == c.State && rev == c.Revision {
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1`+timerCases+` WHERE `+timerEligible+` AND k.case_id=$3)`,
			c.WorkspaceID, c.OwnerID, c.CaseID).Scan(&eligible)
		if err != nil {
			return TimeoutResult{}, err
		}
		if eligible {
			result, err = s.applyCurrent(ctx, tx, c)
			if err != nil {
				return TimeoutResult{}, err
			}
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_TIMEOUT'
 AND claim_token=$3 AND lease_until=$4 AND lease_until>clock_timestamp()`, c.WorkspaceID, c.jobID, c.token, c.until)
	if err != nil {
		return TimeoutResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return TimeoutResult{}, ErrTimeoutLease
	}
	if err = tx.Commit(ctx); err != nil {
		return TimeoutResult{}, err
	}
	return result, nil
}

func (s CaseTimeoutStore) applyCurrent(ctx context.Context, tx pgx.Tx, c TimeoutClaim) (TimeoutResult, error) {
	source, err := loadTimeoutSource(ctx, tx, c.WorkspaceID, c.CaseID, c.State)
	if err != nil {
		return TimeoutResult{}, err
	}
	now := s.now()
	if now.IsZero() || c.Revision < 1 || c.Revision >= 9007199254740991 {
		return TimeoutResult{}, preparecase.ErrHold
	}
	// An earlier clock cannot place a diagnostic behind the latest case history.
	var last *time.Time
	err = tx.QueryRow(ctx, `SELECT max(at) FROM hp_processing.safety_history WHERE workspace_id=$1 AND case_id=$2`,
		c.WorkspaceID, c.CaseID).Scan(&last)
	if err != nil {
		return TimeoutResult{}, err
	}
	if now.Before(source.Admitted) {
		return TimeoutResult{}, preparecase.ErrHold
	}
	problem := ""
	if _, p := source.Deadline(now); p == "" {
		problem = source.checkContent(ctx, s.History, c.WorkspaceID, c.CaseID, c.Revision, now)
	}
	next := casetimeouts.Decide(source.Source, now, problem)
	if next.Reason == "" {
		return TimeoutResult{}, nil
	}
	// A validated timeline must remain monotonic. Do not manufacture a
	// timestamp when the persisted record is ahead of the trusted clock.
	if last != nil && !now.After(*last) {
		return TimeoutResult{}, ErrTimeoutTimeline
	}
	d := preparecase.Decision{At: now, Reason: next.Reason, State: next.State}
	if next.Reason == casetimeouts.WorkingElapsed {
		d.EpisodeID = source.episodeID
	}
	// The HIT-95 history format reads this established decision envelope.
	b, err := json.Marshal(struct {
		CaseID   string
		Revision int64
		Decision preparecase.Decision
		Due      *time.Time
	}{c.CaseID, c.Revision, d, optionalDue(next.Due)})
	if err != nil {
		return TimeoutResult{}, err
	}
	d.Evidence, err = s.Files.Write(ctx, b, "HISTORY", preparecase.Origin{WorkspaceID: c.WorkspaceID, CaseID: c.CaseID, Revision: c.Revision},
		now.Add(90*24*time.Hour), now)
	if err != nil {
		return TimeoutResult{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE hp_processing.case_identity SET state=$3,revision=revision+1
 WHERE workspace_id=$1 AND case_id=$2 AND revision=$4 AND state=$5 AND owner_user_id=$6`,
		c.WorkspaceID, c.CaseID, d.State, c.Revision, c.State, c.OwnerID)
	if err != nil {
		return TimeoutResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return TimeoutResult{}, ErrTimeoutLease
	}
	if next.Reason == casetimeouts.WorkingElapsed {
		tag, err = tx.Exec(ctx, `UPDATE hp_processing.episodes SET completed_at=$4 WHERE workspace_id=$1 AND case_id=$2 AND episode_id=$3 AND completed_at IS NULL`,
			c.WorkspaceID, c.CaseID, source.episodeID, now)
		if err != nil {
			return TimeoutResult{}, err
		}
		if tag.RowsAffected() != 1 {
			return TimeoutResult{}, ErrTimeoutLease
		}
	}
	if c.State == "WORKING" && source.lineage != "" {
		// Pending transport loses authority to start. An ACK already persisted
		// is kept. Neither outcome represents a provider observation.
		_, err = tx.Exec(ctx, `UPDATE hp_processing.outbox SET state='HELD',claim_token=NULL,lease_until=NULL
 WHERE workspace_id=$1 AND message_id=$2 AND kind='DELIVERY_INTENT' AND state IN('PENDING','CLAIMED')`, c.WorkspaceID, source.lineage)
		if err != nil {
			return TimeoutResult{}, err
		}
		_, err = tx.Exec(ctx, `DELETE FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2 AND kind='DELIVERY_INTENT'`, c.WorkspaceID, source.lineage)
		if err != nil {
			return TimeoutResult{}, err
		}
	}
	if err = appendDecisionHistory(ctx, tx, c.WorkspaceID, c.CaseID, c.State, d); err != nil {
		return TimeoutResult{}, err
	}
	id, err := newID()
	if err != nil {
		return TimeoutResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.notifications(workspace_id,notification_id,case_id,reason_code,updated_at)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,case_id,reason_code) DO NOTHING`, c.WorkspaceID, id, c.CaseID, d.Reason, now)
	if err != nil {
		return TimeoutResult{}, err
	}
	return TimeoutResult{CaseID: c.CaseID, State: d.State, Reason: d.Reason}, nil
}

func optionalDue(due time.Time) *time.Time {
	if due.IsZero() {
		return nil
	}
	return &due
}
