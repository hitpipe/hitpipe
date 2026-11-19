package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/recheck"
	"github.com/jackc/pgx/v5"
)

const recheckAccepted = "RECHECK_ACCEPTED"

type RecheckStore struct {
	CaseReadStore
	Files preparecase.Content
}

type knownCase struct {
	observations.Snapshot
	operationState, digest string
}

func readKnownCase(ctx context.Context, tx pgx.Tx, workspace, id, owner string, lock bool) (knownCase, error) {
	c := knownCase{Snapshot: observations.Snapshot{WorkspaceID: workspace, CaseID: id, OwnerID: owner}}
	var lineage, episode *string
	var cfg []byte
	q := `SELECT k.operation_id::text,k.config_id::text,k.state,k.outcome_knowledge,k.revision,k.ever_dispatch_fenced,
 k.known_task_id,k.lineage_id::text,o.state,a.digest,a.canonical_config,
 (SELECT e.episode_id::text FROM hp_processing.episodes e WHERE e.workspace_id=k.workspace_id AND e.case_id=k.case_id
   ORDER BY e.started_at DESC,e.episode_id DESC LIMIT 1)
 FROM hp_processing.case_identity k
 JOIN hp_processing.operations o ON (o.workspace_id,o.operation_id)=(k.workspace_id,k.operation_id)
 JOIN hp_processing.accepted_configs a ON (a.workspace_id,a.operation_id,a.config_id)=(k.workspace_id,k.operation_id,k.config_id)
 WHERE k.workspace_id=$1 AND k.case_id=$2 AND k.owner_user_id=$3 AND o.owner_user_id=$3`
	if lock {
		q += ` FOR UPDATE OF k,o`
	}
	err := tx.QueryRow(ctx, q, workspace, id, owner).Scan(&c.OperationID, &c.ConfigID, &c.State, &c.Outcome, &c.Revision,
		&c.Fenced, &c.KnownTaskID, &lineage, &c.operationState, &c.digest, &cfg, &episode)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, drafts.ErrNotFound
	}
	if err != nil {
		return c, drafts.ErrUnavailable
	}
	if lineage != nil {
		c.LineageID = *lineage
	}
	if episode != nil {
		c.EpisodeID = *episode
	}
	var v struct {
		Destination string `json:"destination_connection_id"`
	}
	canon, err := jcs.Transform(cfg)
	if err != nil || intent.Digest(canon) != c.digest || json.Unmarshal(cfg, &v) != nil || !drafts.UUID(v.Destination) {
		return c, drafts.ErrUnavailable
	}
	c.ConnectionID = v.Destination
	return c, nil
}

func (c knownCase) eligible(connection string) bool {
	return c.Fenced && c.KnownTaskID != nil && *c.KnownTaskID != "" && drafts.UUID(c.LineageID) && drafts.UUID(c.EpisodeID) &&
		c.ConnectionID == connection && c.operationState != "ARCHIVED" && c.State != "EXCLUDED" && c.State != "CLOSED_WITHOUT_CONFIRMATION" &&
		(c.Outcome == "TARGET_ID_KNOWN" || c.Outcome == "RESULT_VERIFIED" || c.Outcome == "RESULT_MISMATCH")
}

func (c knownCase) checkLineage(ctx context.Context, tx pgx.Tx, content OwnedContent, connection string, now time.Time) (time.Time, error) {
	if !c.eligible(connection) {
		return time.Time{}, drafts.ErrConflict
	}
	o := observations.TaskObservation{LineageID: c.LineageID, EpisodeID: c.EpisodeID}
	return observationLineage(ctx, tx, content, o, &c.Snapshot, now)
}

func (s RecheckStore) Accept(ctx context.Context, r recheck.Request, in recheck.Input, g recheck.Grant) (readcases.CaseDetail, error) {
	var detail readcases.CaseDetail
	tx, err := scoped(ctx, s.DB, r.WorkspaceID)
	if err != nil {
		return detail, drafts.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	c, err := readKnownCase(ctx, tx, r.WorkspaceID, r.CaseID, g.OwnerID, true)
	if err != nil {
		return detail, err
	}
	if c.ConnectionID != g.ConnectionID {
		return detail, drafts.ErrDenied
	}

	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil || !now.Before(g.ExpiresAt) {
		return detail, drafts.ErrUnavailable
	}
	now = now.UTC()

	replay, err := recheckReplay(ctx, tx, r, in, g)
	if err != nil {
		return detail, err
	}
	if !replay {
		if c.Revision != *in.Command.Command.ExpectedRevision {
			return detail, drafts.ErrStale
		}
		proofUntil, e := c.checkLineage(ctx, tx, s.Content, g.ConnectionID, now)
		if e != nil {
			return detail, drafts.ErrConflict
		}
		if in.Command.NativeTaskID != nil && *in.Command.NativeTaskID != *c.KnownTaskID {
			return detail, drafts.ErrConflict
		}
		// The ledger is reserved before writing files. A different aggregate may
		// race for the same request id, so the unique key also covers that case.
		tag, e := tx.Exec(ctx, `INSERT INTO hp_processing.user_commands(workspace_id,request_id,actor_id,digest,aggregate_id,expected_revision,result_revision,disposition,recorded_at)
 VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8) ON CONFLICT(workspace_id,request_id) DO NOTHING`,
			r.WorkspaceID, r.RequestID, g.OwnerID, in.Digest, r.CaseID, c.Revision, recheckAccepted, now)
		if e != nil {
			return detail, drafts.ErrUnavailable
		}
		if tag.RowsAffected() != 1 {
			if replay, e = recheckReplay(ctx, tx, r, in, g); e != nil || !replay {
				return detail, drafts.ErrConflict
			}
		} else if e = s.enqueue(ctx, tx, r, in, g, c, now, proofUntil); e != nil {
			return detail, e
		}
	}
	view, err := s.readCases(ctx, tx, readcases.Request{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, Kind: readcases.Detail, Limit: 1},
		readcases.Page{Snapshot: now}, now, readcases.Grant{WorkspaceID: r.WorkspaceID, PrincipalID: g.OwnerID, Epoch: g.Epoch,
			ExpiresAt: g.ExpiresAt, RecoverConnection: g.ConnectionID, RecoverUntil: g.ExpiresAt})
	if err != nil {
		return detail, drafts.ErrUnavailable
	}
	detail = view.(readcases.CaseDetail)
	if ctx.Err() != nil || tx.Commit(ctx) != nil {
		return readcases.CaseDetail{}, drafts.ErrUnavailable
	}
	return detail, nil
}

func recheckReplay(ctx context.Context, tx pgx.Tx, r recheck.Request, in recheck.Input, g recheck.Grant) (bool, error) {
	var digest, owner, aggregate, disposition string
	err := tx.QueryRow(ctx, `SELECT digest,actor_id::text,aggregate_id::text,disposition FROM hp_processing.user_commands
 WHERE workspace_id=$1 AND request_id=$2`, r.WorkspaceID, r.RequestID).Scan(&digest, &owner, &aggregate, &disposition)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, drafts.ErrUnavailable
	}
	if digest != in.Digest || owner != g.OwnerID || aggregate != r.CaseID || disposition != recheckAccepted {
		return false, drafts.ErrConflict
	}
	return true, nil
}

func (s RecheckStore) enqueue(ctx context.Context, tx pgx.Tx, r recheck.Request, in recheck.Input, g recheck.Grant, c knownCase, now, proofUntil time.Time) error {
	// The read has a short fixed delivery window. It never extends the
	// original write episode, authorization, or proof retention.
	deadline := now.Add(30 * time.Second)
	if proofUntil.Before(deadline) {
		deadline = proofUntil
	}
	packet := recheck.Packet{Query: drafts.DeliveryQuery{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, LineageID: c.LineageID,
		NativeTaskID: c.KnownTaskID, ExpectedConnectionEpoch: g.Epoch, DeadlineAt: deadline}, EpisodeID: c.EpisodeID,
		OperationID: c.OperationID, ConfigID: c.ConfigID, ConfigDigest: c.digest, ConnectionID: c.ConnectionID, OwnerID: c.OwnerID, Revision: c.Revision}
	raw, err := json.Marshal(packet)
	if err != nil {
		return drafts.ErrUnavailable
	}
	origin := preparecase.Origin{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, Revision: c.Revision}
	ref, err := s.Files.Write(ctx, raw, recheck.Kind, origin, deadline, now)
	if err != nil {
		return drafts.ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,created_at,next_attempt_at,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$5,$7,$7,$8)`, r.WorkspaceID, r.RequestID, recheck.Kind, ref.ID, ref.Digest, ref.Bytes, now, ref.ExpiresAt)
	if err != nil {
		return drafts.ErrUnavailable
	}
	h, err := newID()
	if err != nil {
		return drafts.ErrUnavailable
	}
	history, err := json.Marshal(struct {
		CaseID     string
		Revision   int64
		RequestID  string
		ReasonText string
		Decision   preparecase.Decision
	}{r.CaseID, c.Revision, r.RequestID, in.Command.Reason, preparecase.Decision{State: c.State, Reason: recheckAccepted, At: now, EpisodeID: c.EpisodeID}})
	if err != nil {
		return drafts.ErrUnavailable
	}
	ref, err = s.Files.Write(ctx, history, "HISTORY", origin, now.Add(90*24*time.Hour), now)
	if err != nil {
		return drafts.ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,actor_id,at,reason_code,old_state,new_state)
 VALUES($1,$2,$3,'USER',$4,$5,$6,$7,$7)`, r.WorkspaceID, h, r.CaseID, g.OwnerID, now, recheckAccepted, c.State)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO hp_processing.history_content VALUES($1,$2,$3,$4,$5,$6,$7)`, r.WorkspaceID, h, ref.ID, ref.Digest, ref.Bytes, now, ref.ExpiresAt)
	}
	if err != nil {
		return drafts.ErrUnavailable
	}
	return nil
}
