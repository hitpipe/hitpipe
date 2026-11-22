// Package revocation persists IAM revocations and their frozen runtime roster in PostgreSQL.
package revocation

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	queryTimeout = 2 * time.Second
	maxRoster    = 64
)

// Store uses the existing IAM application login. No migration, provisioning,
// fencing, or other owner's runtime operation is performed by this adapter.
type Store struct{ pool *pgxpool.Pool }

var _ app.Store = (*Store)(nil)

func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 {
		return nil, app.ErrInvalid
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Begin(ctx context.Context, intent app.Intent, now time.Time) (app.Record, error) {
	if err := app.ValidateIntent(intent); err != nil {
		return app.Record{}, err
	}
	now = app.NormalizeTime(now)
	if !validTime(now) || !validTime(now.Add(app.Deadline)) {
		return app.Record{}, app.ErrInvalid
	}
	ctx, cancel, tx, err := s.transaction(ctx, false)
	if err != nil {
		return app.Record{}, err
	}
	defer cancel()
	defer rollback(tx)

	// Match HIT-115 registration's lock order: registry first, then IAM
	// revocations/target. This seals a generation and boot even when the
	// registry changes after commit and before delivery. No expiry heuristic
	// removes a participant; a missing ACK remains missing.
	if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.runtime_registry IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	existing, err := readRecord(ctx, tx, intent.ID, true)
	if err == nil {
		if !sameIntent(existing, intent) {
			return app.Record{}, app.ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, app.ErrNotFound) {
		return app.Record{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, existingSubject, string(intent.Subject.Scope), intent.Subject.SubjectID,
		intent.Subject.WorkspaceID).Scan(&exists); err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	if exists {
		return app.Record{}, app.ErrConflict
	}
	roster, err := freezeRoster(ctx, tx)
	if err != nil {
		return app.Record{}, err
	}
	var epoch uint64
	if intent.Subject.Scope == startgate.Session {
		err = tx.QueryRow(ctx, denySession, intent.Subject.SubjectID, now, startgate.MaxSafeInteger).Scan(&epoch)
	} else {
		if _, err := tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, *intent.Subject.WorkspaceID); err != nil {
			return app.Record{}, app.ErrUnavailable
		}
		err = tx.QueryRow(ctx, advanceWorkspace, intent.Subject.SubjectID, startgate.MaxSafeInteger).Scan(&epoch)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// An absent or already unusable target must not create a revocation
		// against an invented subject, or advance an already exhausted epoch.
		return app.Record{}, app.ErrConflict
	}
	if err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	deadline := now.Add(app.Deadline)
	if _, err := tx.Exec(ctx, insertRevocation, intent.Subject.WorkspaceID, intent.ID, string(intent.Subject.Scope),
		intent.Subject.SubjectID, epoch, now, deadline, intent.ReasonCode); err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	tag, err := tx.Exec(ctx, insertParticipants, intent.Subject.WorkspaceID, intent.ID)
	if err != nil || tag.RowsAffected() != int64(len(roster)) {
		return app.Record{}, app.ErrUnavailable
	}
	if err := commit(ctx, tx); err != nil {
		return app.Record{}, err
	}
	return app.Record{ID: intent.ID, Subject: intent.Subject, Epoch: epoch, AcceptedAt: now, DeadlineAt: deadline,
		ReasonCode: intent.ReasonCode, Status: app.Pending, Participants: roster}, nil
}

func (s *Store) Get(ctx context.Context, id string) (app.Record, error) {
	if !validUUID(id) {
		return app.Record{}, app.ErrInvalid
	}
	ctx, cancel, tx, err := s.transaction(ctx, true)
	if err != nil {
		return app.Record{}, err
	}
	defer cancel()
	defer rollback(tx)
	return readRecord(ctx, tx, id, false)
}

func (s *Store) Open(ctx context.Context, after string, limit int) ([]string, error) {
	if limit < 1 || limit > maxRoster || (after != "" && !validUUID(after)) {
		return nil, app.ErrInvalid
	}
	ctx, cancel, tx, err := s.transaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rollback(tx)
	rows, err := tx.Query(ctx, listOpen, after, limit)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil || !validUUID(id) {
			return nil, app.ErrUnavailable
		}
		out = append(out, id)
	}
	if rows.Err() != nil {
		return nil, app.ErrUnavailable
	}
	return out, nil
}

func (s *Store) Acknowledge(ctx context.Context, id string, ack startgate.BarrierAck) error {
	if !validUUID(id) {
		return app.ErrInvalid
	}
	ctx, cancel, tx, err := s.transaction(ctx, false)
	if err != nil {
		return err
	}
	defer cancel()
	defer rollback(tx)
	record, err := readRecord(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if err := app.ValidateAck(record, ack); err != nil {
		return err
	}
	at := app.NormalizeTime(ack.AcknowledgedAt)
	var member *app.Participant
	for i := range record.Participants {
		p := &record.Participants[i]
		if p.Identity == (startgate.Identity{RuntimeID: ack.RuntimeID, BootID: ack.BootID, Generation: ack.Generation}) {
			member = p
			break
		}
	}
	if member == nil {
		return app.ErrConflict
	}
	if member.AcknowledgedAt != nil {
		// An exact replay must never rewrite the persisted history.
		if !at.Equal(*member.AcknowledgedAt) || ack.AppliedEpoch != member.AppliedEpoch {
			return app.ErrConflict
		}
		return nil
	}
	if record.Status == app.Effective {
		return app.ErrUnavailable
	}
	tag, err := tx.Exec(ctx, storeAck, id, ack.RuntimeID, ack.Generation, ack.BootID, ack.AppliedEpoch, at)
	if err != nil || tag.RowsAffected() != 1 {
		return app.ErrUnavailable
	}
	return commit(ctx, tx)
}

func (s *Store) Finalize(ctx context.Context, id string, now time.Time) (app.Record, error) {
	started := time.Now()
	now = app.NormalizeTime(now)
	if !validUUID(id) || !validTime(now) {
		return app.Record{}, app.ErrInvalid
	}
	ctx, cancel, tx, err := s.transaction(ctx, false)
	if err != nil {
		return app.Record{}, err
	}
	defer cancel()
	defer rollback(tx)
	record, err := readRecord(ctx, tx, id, true)
	if err != nil {
		return app.Record{}, err
	}
	if now.Before(record.AcceptedAt) {
		return app.Record{}, app.ErrInvalid
	}
	// SQL lock/IO time consumes the original window even if Finalize was
	// entered just before its deadline. Advance the caller's trusted clock
	// by monotonic elapsed time; never extend the stored DeadlineAt.
	now = app.NormalizeTime(now.Add(time.Since(started)))
	if !validTime(now) {
		return app.Record{}, app.ErrUnavailable
	}
	if record.Status == app.Effective {
		return record, nil
	}
	complete := len(record.Participants) > 0
	for _, p := range record.Participants {
		if p.AcknowledgedAt == nil || p.AcknowledgedAt.After(now) {
			complete = false
		}
	}
	changed := false
	if record.DeadlineBreachedAt == nil && !now.Before(record.DeadlineAt) {
		// No complete durable ACK proof was established by the deadline.
		// Later recovery of an original ACK may prove the barrier, but it must
		// never erase this recorded deadline breach.
		incident, err := newUUID()
		if err != nil {
			return app.Record{}, app.ErrUnavailable
		}
		record.DeadlineBreachedAt, record.IncidentID = &now, &incident
		record.Status = app.Incident
		changed = true
	}
	if complete {
		record.Status, record.EffectiveAt = app.Effective, &now
		changed = true
	}
	if !changed {
		return record, nil
	}
	tag, err := tx.Exec(ctx, updateFinal, id, string(record.Status), record.EffectiveAt, record.DeadlineBreachedAt, record.IncidentID)
	if err != nil || tag.RowsAffected() != 1 {
		return app.Record{}, app.ErrUnavailable
	}
	if err := commit(ctx, tx); err != nil {
		return app.Record{}, err
	}
	return record, nil
}

func (s *Store) transaction(ctx context.Context, readOnly bool) (context.Context, context.CancelFunc, pgx.Tx, error) {
	if s == nil || s.pool == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, nil, app.ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	opts := pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	if readOnly {
		// Load a consistent pair of revocation and participant records.
		opts = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	}
	tx, err := s.pool.BeginTx(bounded, opts)
	if err != nil {
		cancel()
		return nil, nil, nil, app.ErrUnavailable
	}
	var allowed bool
	if err := tx.QueryRow(bounded, checkAuthority).Scan(&allowed); err != nil || !allowed {
		rollback(tx)
		cancel()
		return nil, nil, nil, app.ErrUnavailable
	}
	// Never expose a barrier command after an asynchronous COMMIT. The
	// pool's startup settings may differ from the server defaults.
	if !readOnly {
		if _, err := tx.Exec(bounded, "SET LOCAL synchronous_commit = on"); err != nil {
			rollback(tx)
			cancel()
			return nil, nil, nil, app.ErrUnavailable
		}
	}
	return bounded, cancel, tx, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func commit(ctx context.Context, tx pgx.Tx) error {
	// PostgreSQL explicitly reports a rollback when that outcome is known.
	// A network failure/cancellation during COMMIT is ambiguous; never give
	// the caller a committable delivery command from that invocation.
	err := tx.Commit(ctx)
	var rejection *pgconn.PgError
	if errors.Is(err, pgx.ErrTxCommitRollback) ||
		(errors.As(err, &rejection) && (strings.HasPrefix(rejection.Code, "23") || rejection.Code == "P0001")) {
		return app.ErrUnavailable
	}
	if err != nil || ctx.Err() != nil {
		return app.ErrUncertain
	}
	return nil
}

func freezeRoster(ctx context.Context, tx pgx.Tx) ([]app.Participant, error) {
	rows, err := tx.Query(ctx, readRoster, maxRoster+1)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	defer rows.Close()
	roster := make([]app.Participant, 0)
	for rows.Next() {
		var member app.Participant
		var service string
		i := &member.Identity
		if rows.Scan(&i.RuntimeID, &i.BootID, &i.Generation, &service, &member.CertificateFingerprint) != nil {
			return nil, app.ErrUnavailable
		}
		registration := runtimeaccess.Registration{RuntimeID: i.RuntimeID, BootID: i.BootID,
			Generation: int64(i.Generation), Service: service, CertificateFingerprint: member.CertificateFingerprint}
		if !registration.Valid() || len(roster) == maxRoster {
			return nil, app.ErrUnavailable
		}
		roster = append(roster, member)
	}
	if rows.Err() != nil || len(roster) == 0 {
		return nil, app.ErrUnavailable
	}
	return roster, nil
}

func readRecord(ctx context.Context, tx pgx.Tx, id string, lock bool) (app.Record, error) {
	stmt := getRevocation
	if lock {
		stmt += ` FOR UPDATE`
	}
	var r app.Record
	var scope, status string
	err := tx.QueryRow(ctx, stmt, id).Scan(&r.ID, &r.Subject.WorkspaceID, &scope, &r.Subject.SubjectID,
		&r.Epoch, &r.AcceptedAt, &r.DeadlineAt, &status, &r.EffectiveAt, &r.DeadlineBreachedAt, &r.IncidentID, &r.ReasonCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Record{}, app.ErrNotFound
	}
	if err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	r.Subject.Scope, r.Status = startgate.Scope(scope), app.Status(status)
	r.AcceptedAt, r.DeadlineAt = app.NormalizeTime(r.AcceptedAt), app.NormalizeTime(r.DeadlineAt)
	r.EffectiveAt, r.DeadlineBreachedAt = normalizePointer(r.EffectiveAt), normalizePointer(r.DeadlineBreachedAt)
	if !validRecord(r) {
		return app.Record{}, app.ErrUnavailable
	}
	rows, err := tx.Query(ctx, getParticipants, id, maxRoster+1)
	if err != nil {
		return app.Record{}, app.ErrUnavailable
	}
	defer rows.Close()
	runtimes := map[string]bool{}
	boots := map[string]bool{}
	var rowWorkspace *string
	for rows.Next() {
		var member app.Participant
		var epoch *uint64
		i := &member.Identity
		if rows.Scan(&rowWorkspace, &i.RuntimeID, &i.BootID, &i.Generation, &member.CertificateFingerprint,
			&epoch, &member.AcknowledgedAt) != nil || !validUUID(i.RuntimeID) ||
			!validUUID(i.BootID) || i.Generation == 0 || i.Generation > startgate.MaxSafeInteger ||
			len(r.Participants) == maxRoster || !sameOptionalUUID(rowWorkspace, r.Subject.WorkspaceID) ||
			runtimes[i.RuntimeID] || boots[i.BootID] || (epoch == nil) != (member.AcknowledgedAt == nil) ||
			(member.CertificateFingerprint != "" && !runtimeaccess.ValidFingerprint(member.CertificateFingerprint)) {
			return app.Record{}, app.ErrUnavailable
		}
		if epoch != nil {
			member.AppliedEpoch, member.AcknowledgedAt = *epoch, normalizePointer(member.AcknowledgedAt)
			if member.AppliedEpoch != r.Epoch || member.AcknowledgedAt.Before(r.AcceptedAt) ||
				!member.AcknowledgedAt.Before(r.DeadlineAt) ||
				(r.EffectiveAt != nil && member.AcknowledgedAt.After(*r.EffectiveAt)) {
				return app.Record{}, app.ErrUnavailable
			}
		}
		if r.Status == app.Effective && member.AcknowledgedAt == nil {
			return app.Record{}, app.ErrUnavailable
		}
		runtimes[i.RuntimeID], boots[i.BootID] = true, true
		r.Participants = append(r.Participants, member)
	}
	if rows.Err() != nil || len(r.Participants) == 0 {
		return app.Record{}, app.ErrUnavailable
	}
	return r, nil
}

func validRecord(r app.Record) bool {
	if app.ValidateIntent(app.Intent{ID: r.ID, Subject: r.Subject, ReasonCode: r.ReasonCode}) != nil ||
		r.Epoch < 1 || r.Epoch > startgate.MaxSafeInteger || !validTime(r.AcceptedAt) || !validTime(r.DeadlineAt) ||
		!r.DeadlineAt.Equal(r.AcceptedAt.Add(app.Deadline)) || (r.DeadlineBreachedAt == nil) != (r.IncidentID == nil) ||
		(r.DeadlineBreachedAt != nil && (!validTime(*r.DeadlineBreachedAt) || r.DeadlineBreachedAt.Before(r.DeadlineAt))) ||
		(r.IncidentID != nil && !validUUID(*r.IncidentID)) {
		return false
	}
	switch r.Status {
	case app.Pending:
		return r.EffectiveAt == nil && r.DeadlineBreachedAt == nil
	case app.Incident:
		return r.EffectiveAt == nil && r.DeadlineBreachedAt != nil
	case app.Effective:
		return r.EffectiveAt != nil && validTime(*r.EffectiveAt) && !r.EffectiveAt.Before(r.AcceptedAt) &&
			((r.DeadlineBreachedAt == nil && r.EffectiveAt.Before(r.DeadlineAt)) ||
				(r.DeadlineBreachedAt != nil && !r.EffectiveAt.Before(*r.DeadlineBreachedAt)))
	default:
		return false
	}
}

func validUUID(id string) bool {
	return runtimeaccess.ValidUUID(id) && strings.ToLower(id) == id
}

func validTime(t time.Time) bool {
	return !t.IsZero() && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
}

func normalizePointer(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := app.NormalizeTime(*t)
	return &v
}

func sameOptionalUUID(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func sameIntent(r app.Record, i app.Intent) bool {
	return r.ID == i.ID && r.Subject.Scope == i.Subject.Scope &&
		r.Subject.SubjectID == i.Subject.SubjectID &&
		sameOptionalUUID(r.Subject.WorkspaceID, i.Subject.WorkspaceID) && r.ReasonCode == i.ReasonCode
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
