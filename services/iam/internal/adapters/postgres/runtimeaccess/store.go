// Package runtimeaccess stores the runtime tuple and reads current IAM denials.
// The registry tracks a boot; registration alone does not open its local gate.
package runtimeaccess

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	queryTimeout = 2 * time.Second
	maxDenials   = 10000
	closed       = "CLOSED"
	syncing      = "SYNCING"
	open         = "OPEN"
	fenced       = "FENCED"
)

// The login must be the dedicated IAM app and the relations must belong to the
// IAM migrator. No tenant GUC participates in these IAM-owned global reads.
const checkAuthority = `SELECT current_user=session_user
 AND (SELECT rolcanlogin AND NOT(rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication)
      FROM pg_catalog.pg_roles WHERE rolname=current_user)
 AND pg_has_role(current_user,'hp_iam_app','USAGE')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members
      WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user)
        AND roleid<>(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_app'))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members
      WHERE member=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname='hp_iam_app'))
 AND (SELECT count(*)=4 FROM pg_catalog.pg_roles
      WHERE rolname IN('hp_iam_app','hp_iam_worker','hp_iam_purge','hp_iam_migrator')
        AND NOT(rolcanlogin OR rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication))
 AND NOT has_database_privilege(current_user,current_database(),'CREATE')
 AND NOT has_schema_privilege(current_user,'hp_iam','CREATE')
 AND (SELECT count(*)=3 FROM pg_catalog.pg_class c
      JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
      JOIN pg_catalog.pg_roles owner ON owner.oid=c.relowner AND owner.oid=n.nspowner
      WHERE n.nspname='hp_iam' AND c.relkind='r' AND owner.rolname='hp_iam_migrator'
        AND c.relname IN('schema_version','runtime_registry','revocations'))
 AND (SELECT count(*)=1 AND min(version)=1 FROM hp_iam.schema_version)
 AND NOT has_table_privilege(current_user,'hp_iam.runtime_registry','DELETE')
 AND NOT has_table_privilege(current_user,'hp_iam.revocations','DELETE')`

const readRegistry = `SELECT service,runtime_id::text,boot_id::text,certificate_fingerprint,generation,
 status,last_observed_at,fence_evidence_ref::text FROM hp_iam.runtime_registry`

const insertRuntime = `INSERT INTO hp_iam.runtime_registry
 (runtime_id,service,boot_id,certificate_fingerprint,generation,status,registered_at,last_observed_at)
 VALUES($1,$2,$3,$4,$5,'SYNCING',statement_timestamp(),statement_timestamp())`

const replaceRuntime = `UPDATE hp_iam.runtime_registry
 SET boot_id=$2,certificate_fingerprint=$3,generation=$4,status='SYNCING',
     registered_at=statement_timestamp(),fence_evidence_ref=NULL
 WHERE runtime_id=$1`

const touchRuntime = `UPDATE hp_iam.runtime_registry
 SET last_observed_at=GREATEST(clock_timestamp(),last_observed_at+interval '1 microsecond'),
     status=$2 WHERE runtime_id=$1 RETURNING last_observed_at`

const readDenials = `SELECT workspace_id::text,scope,subject_id::text,revocation_id::text,epoch,
 accepted_at,effective_at,status FROM hp_iam.revocations ORDER BY revocation_id LIMIT $1`

type Store struct {
	pool    *pgxpool.Pool
	allowed map[string]app.Registration
}

// New borrows a bounded IAM application pool and copies the approved roster.
// Different runtimes may share a service certificate if explicitly provisioned.
func New(pool *pgxpool.Pool, allowed []app.Registration) (*Store, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 || len(allowed) == 0 {
		return nil, app.ErrInvalid
	}
	roster := make(map[string]app.Registration, len(allowed))
	boots := make(map[string]bool, len(allowed))
	for _, reg := range allowed {
		if !reg.Valid() {
			return nil, app.ErrInvalid
		}
		reg = reg.Normalized()
		if _, duplicate := roster[reg.RuntimeID]; duplicate || boots[reg.BootID] {
			return nil, app.ErrInvalid
		}
		roster[reg.RuntimeID] = reg
		boots[reg.BootID] = true
	}
	return &Store{pool: pool, allowed: roster}, nil
}

type operation int

const (
	register operation = iota
	read
	openLocal
)

func (s *Store) Register(ctx context.Context, reg app.Registration) (app.Snapshot, error) {
	return s.snapshot(ctx, reg, register, app.Snapshot{})
}

func (s *Store) Read(ctx context.Context, reg app.Registration) (app.Snapshot, error) {
	return s.snapshot(ctx, reg, read, app.Snapshot{})
}

// OpenLocal is only for trusted IAM composition after its local gate has applied
// a snapshot. It accepts neither another caller boot nor a replayed OPEN row.
func (s *Store) OpenLocal(ctx context.Context, reg app.Registration, applied app.Snapshot) (app.Snapshot, error) {
	if reg.Service != "IAM" {
		return app.Snapshot{}, app.ErrDenied
	}
	return s.snapshot(ctx, reg, openLocal, applied)
}

type row struct {
	app.Registration
	status   string
	observed time.Time
	fence    *string
}

func (r *row) scan(result interface{ Scan(...any) error }) error {
	return result.Scan(&r.Service, &r.RuntimeID, &r.BootID, &r.CertificateFingerprint,
		&r.Generation, &r.status, &r.observed, &r.fence)
}

func (r row) active() bool {
	return r.status == closed || r.status == syncing || r.status == open
}

func (s *Store) snapshot(ctx context.Context, reg app.Registration, op operation, applied app.Snapshot) (app.Snapshot, error) {
	if s == nil || s.pool == nil || ctx == nil || ctx.Err() != nil {
		return app.Snapshot{}, app.ErrUnavailable
	}
	if !reg.Valid() {
		return app.Snapshot{}, app.ErrInvalid
	}
	reg = reg.Normalized()
	if s.allowed[reg.RuntimeID] != reg {
		return app.Snapshot{}, app.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return app.Snapshot{}, app.ErrUnavailable
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var authorized bool
	if err := tx.QueryRow(ctx, checkAuthority).Scan(&authorized); err != nil || !authorized {
		return app.Snapshot{}, app.ErrUnavailable
	}
	// A bounded lock serializes registration, rotation and roster changes,
	// including initially missing keys. A repeat sees the committed first boot.
	if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.runtime_registry IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return app.Snapshot{}, app.ErrUnavailable
	}
	current, err := prepareRuntime(ctx, tx, reg, op)
	if err != nil {
		return app.Snapshot{}, err
	}
	complete, err := s.completeRoster(ctx, tx)
	if err != nil {
		return app.Snapshot{}, err
	}
	if op == openLocal {
		// Keep the checked denial set current through the durable transition.
		if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.revocations IN SHARE MODE`); err != nil {
			return app.Snapshot{}, app.ErrUnavailable
		}
	}
	denials, err := loadDenials(ctx, tx)
	if err != nil {
		return app.Snapshot{}, err
	}
	status := current.status
	if op == openLocal {
		if status != syncing {
			return app.Snapshot{}, app.ErrConflict
		}
		if !complete || !applied.Complete || applied.Gate != syncing ||
			applied.RuntimeID != reg.RuntimeID || applied.Generation != reg.Generation || applied.BootID != reg.BootID ||
			applied.Revision != current.observed.UnixMicro() || !reflect.DeepEqual(applied.Denials, denials) {
			return app.Snapshot{}, app.ErrStale
		}
		status = open
	}
	var observed time.Time
	if err := tx.QueryRow(ctx, touchRuntime, reg.RuntimeID, status).Scan(&observed); err != nil {
		return app.Snapshot{}, app.ErrUnavailable
	}
	revision := observed.UnixMicro()
	if !validDate(observed) || revision < 1 || revision > authorizeaction.MaxWireInteger || tx.Commit(ctx) != nil || ctx.Err() != nil {
		return app.Snapshot{}, app.ErrUnavailable
	}
	if status == open && (!complete || op == register) {
		status = syncing
	}
	return app.Snapshot{RuntimeID: reg.RuntimeID, BootID: reg.BootID, Generation: reg.Generation,
		Revision: revision, Denials: denials, Gate: status, Complete: complete}, nil
}

func prepareRuntime(ctx context.Context, tx pgx.Tx, reg app.Registration, op operation) (row, error) {
	var current row
	err := current.scan(tx.QueryRow(ctx, readRegistry+` WHERE runtime_id=$1`, reg.RuntimeID))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if op != register {
			return row{}, app.ErrStale
		}
		_, err = tx.Exec(ctx, insertRuntime, reg.RuntimeID, reg.Service, reg.BootID, reg.CertificateFingerprint, reg.Generation)
	case err != nil:
		return row{}, app.ErrUnavailable
	case current.Registration == reg:
		if !current.active() || current.fence != nil || !validDate(current.observed) {
			return row{}, app.ErrStale
		}
		return current, nil
	case op != register || current.Generation > reg.Generation:
		return row{}, app.ErrStale
	case current.Service != reg.Service || current.Generation == reg.Generation || current.BootID == reg.BootID:
		return row{}, app.ErrConflict
	case !current.Registration.Valid() || current.status != fenced || current.fence == nil || !app.ValidUUID(*current.fence):
		// A trusted durable fence must already exist for this exact prior
		// tuple. Time, generation increments and the request provide no fence.
		return row{}, app.ErrConflict
	default:
		_, err = tx.Exec(ctx, replaceRuntime, reg.RuntimeID, reg.BootID, reg.CertificateFingerprint, reg.Generation)
	}
	if err != nil {
		var sqlError interface{ SQLState() string }
		if errors.As(err, &sqlError) && sqlError.SQLState() == "23505" {
			return row{}, app.ErrConflict
		}
		return row{}, app.ErrUnavailable
	}
	return row{Registration: reg, status: syncing}, nil
}

func (s *Store) completeRoster(ctx context.Context, tx pgx.Tx) (bool, error) {
	rows, err := tx.Query(ctx, readRegistry+` ORDER BY runtime_id LIMIT $1`, len(s.allowed)+1)
	if err != nil {
		return false, app.ErrUnavailable
	}
	defer rows.Close()
	count := 0
	complete := true
	for rows.Next() {
		var member row
		if member.scan(rows) != nil {
			return false, app.ErrUnavailable
		}
		count++
		if s.allowed[member.RuntimeID] != member.Registration || !member.active() || member.fence != nil || !validDate(member.observed) {
			complete = false
		}
	}
	if rows.Err() != nil {
		return false, app.ErrUnavailable
	}
	return complete && count == len(s.allowed), nil
}

func loadDenials(ctx context.Context, tx pgx.Tx) ([]app.Denial, error) {
	rows, err := tx.Query(ctx, readDenials, maxDenials+1)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	defer rows.Close()
	denials := make([]app.Denial, 0)
	for rows.Next() {
		var d app.Denial
		var status string
		err := rows.Scan(&d.WorkspaceID, &d.Scope, &d.SubjectID, &d.RevocationID, &d.Epoch, &d.AcceptedAt, &d.EffectiveAt, &status)
		if err != nil || !validDenial(d, status) || len(denials) == maxDenials {
			return nil, app.ErrUnavailable
		}
		d.AcceptedAt = d.AcceptedAt.UTC()
		if d.EffectiveAt != nil {
			effective := d.EffectiveAt.UTC()
			d.EffectiveAt = &effective
		}
		denials = append(denials, d)
	}
	if rows.Err() != nil {
		return nil, app.ErrUnavailable
	}
	return denials, nil
}

func validDenial(d app.Denial, status string) bool {
	if (d.Scope != "SESSION" && d.Scope != "MEMBERSHIP" && d.Scope != "CONNECTION" && d.Scope != "WORKSPACE") ||
		(d.WorkspaceID != nil && (!app.ValidUUID(*d.WorkspaceID) || d.Scope == "WORKSPACE" && d.SubjectID != *d.WorkspaceID)) ||
		!app.ValidUUID(d.SubjectID) || !app.ValidUUID(d.RevocationID) ||
		d.Epoch < 1 || d.Epoch > authorizeaction.MaxWireInteger || !validDate(d.AcceptedAt) {
		return false
	}
	switch status {
	case "PENDING", "INCIDENT":
		return d.EffectiveAt == nil
	case "EFFECTIVE":
		return d.EffectiveAt != nil && validDate(*d.EffectiveAt) && !d.EffectiveAt.Before(d.AcceptedAt)
	default:
		return false
	}
}

func validDate(t time.Time) bool {
	year := t.UTC().Year()
	return !t.IsZero() && year >= 1 && year <= 9999
}
