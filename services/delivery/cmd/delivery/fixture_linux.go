//go:build delivery_fixture

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/deliveryrun"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/dispatch"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixtureManifest struct {
	Snapshot intake.Snapshot
	Token    string
	ClickUp  string
}

type fixtureAuthority struct {
	config fixtureManifest
	path   string
	digest [32]byte
	pool   *pgxpool.Pool
}

type dispatchAuthority struct{ *fixtureAuthority }

func launch(ctx context.Context, cfg deliveryrun.Config) error {
	path := os.Getenv("DELIVERY_FIXTURE_MANIFEST")
	raw, err := readManifest(path)
	if os.Getenv("DELIVERY_PROFILE") != "TEST" || err != nil ||
		!loopback(cfg.BusinessAddr) || !loopback(cfg.HealthAddr) {
		return errors.New("closed: verification requires explicit TEST profile, fixture manifest and loopback listeners")
	}
	u, err := url.Parse(cfg.ProcessingOrigin)
	if err != nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("closed: test Processing must use a loopback IP")
	}
	var manifest fixtureManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF ||
		manifest.Token != "test-only-delivery-token" || manifest.Snapshot.Workspace == uuid.Nil ||
		manifest.Snapshot.Case == uuid.Nil || manifest.Snapshot.Epoch < 1 {
		return errors.New("closed: invalid test authority manifest")
	}
	ca, err := roots(os.Getenv("DELIVERY_TLS_DIR"), "clickup-fixture-ca")
	if err != nil {
		return err
	}
	target, err := dispatch.FixtureClickUp(manifest.ClickUp, ca)
	if err != nil {
		return errors.New("closed: ClickUp test target must use HTTPS loopback")
	}
	pool, err := pgxpool.New(ctx, cfg.WorkerDSN)
	if err != nil {
		return errors.New("closed: test authority worker database unavailable")
	}
	defer pool.Close()
	f := &fixtureAuthority{config: manifest, path: path, digest: sha256.Sum256(raw), pool: pool}
	return deliveryrun.Run(ctx, cfg, deliveryrun.Adapters{
		Intake: f, Dispatch: dispatchAuthority{f}, Read: f, Gate: f, Credentials: f, Scopes: f.scopes,
	}, target)
}

func loopback(address string) bool {
	host, port, err := net.SplitHostPort(address)
	return err == nil && port != "" && net.ParseIP(host).IsLoopback()
}

func readManifest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 {
		return nil, intake.ErrUnavailable
	}
	return raw, nil
}

func (f *fixtureAuthority) available(ctx context.Context) error {
	raw, err := readManifest(f.path)
	if err != nil || sha256.Sum256(raw) != f.digest || ctx.Err() != nil {
		return intake.ErrUnavailable
	}
	return nil
}

func (f *fixtureAuthority) scopes(ctx context.Context) ([]deliveryrun.Scope, error) {
	if err := f.available(ctx); err != nil {
		return nil, err
	}
	s := f.config.Snapshot
	return []deliveryrun.Scope{{Workspace: s.Workspace, Epoch: s.Epoch}}, nil
}

func (f *fixtureAuthority) Current(ctx context.Context, ws, caseID uuid.UUID) (intake.Snapshot, error) {
	if err := f.available(ctx); err != nil {
		return intake.Snapshot{}, err
	}
	s := f.config.Snapshot
	if ws != s.Workspace || caseID != s.Case {
		return intake.Snapshot{}, intake.ErrDenied
	}
	return s, nil
}

func (f *fixtureAuthority) WithOpen(ctx context.Context, ws uuid.UUID, epoch int64, work func() error) error {
	if err := f.available(ctx); err != nil {
		return err
	}
	if ws != f.config.Snapshot.Workspace || epoch != f.config.Snapshot.Epoch {
		return intake.ErrDenied
	}
	return work()
}

func (f *fixtureAuthority) ForConnection(ctx context.Context, ws, connection uuid.UUID, epoch int64) (dispatch.Credential, error) {
	s := f.config.Snapshot
	if err := f.available(ctx); err != nil || ws != s.Workspace || connection != s.Connection || epoch != s.Epoch {
		return dispatch.Credential{}, intake.ErrDenied
	}
	return dispatch.Credential{Token: f.config.Token, Fingerprint: sha256.Sum256([]byte(f.config.Token))}, nil
}

func (f *fixtureAuthority) AuthorizeRead(ctx context.Context, q intake.Query) (dispatch.Authorization, error) {
	s := f.config.Snapshot
	if q.Workspace != s.Workspace || q.Case != s.Case || q.Lineage != s.Lineage || q.Epoch != s.Epoch {
		return dispatch.Authorization{}, intake.ErrDenied
	}
	return f.bound(ctx, uuid.Nil)
}

func (d dispatchAuthority) Current(ctx context.Context, job dispatch.Job) (dispatch.Authorization, error) {
	if job.Workspace != d.config.Snapshot.Workspace || job.Attempt == uuid.Nil {
		return dispatch.Authorization{}, intake.ErrDenied
	}
	return d.bound(ctx, job.Attempt)
}

func (f *fixtureAuthority) bound(ctx context.Context, expected uuid.UUID) (dispatch.Authorization, error) {
	if err := f.available(ctx); err != nil {
		return dispatch.Authorization{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	s := f.config.Snapshot
	tx, err := f.pool.BeginTx(bounded, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return dispatch.Authorization{}, intake.ErrUnavailable
	}
	defer tx.Rollback(bounded)
	if _, err := tx.Exec(bounded, "SELECT set_config('hp.workspace_id',$1,true)", s.Workspace.String()); err != nil {
		return dispatch.Authorization{}, intake.ErrUnavailable
	}
	var attempt uuid.UUID
	err = tx.QueryRow(bounded, `SELECT a.attempt_id FROM hp_delivery.attempts a
		JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id)
		WHERE a.workspace_id=$1 AND l.case_id=$2 AND a.lineage_id=$3 AND a.episode_id=$4 AND a.attempt_number=1`,
		s.Workspace, s.Case, s.Lineage, s.Episode).Scan(&attempt)
	if err != nil || expected != uuid.Nil && attempt != expected || tx.Commit(bounded) != nil {
		return dispatch.Authorization{}, intake.ErrDenied
	}
	return dispatch.Authorization{
		Workspace: s.Workspace, Attempt: attempt, Case: s.Case, Config: s.Config,
		Connection: s.Connection, Lineage: s.Lineage, Episode: s.Episode, Epoch: s.Epoch,
		ConfigDigest: s.ConfigDigest, FactsDigest: s.FactsDigest, FactsUntil: s.FactsUntil,
		Deadline: s.Deadline, TargetWorkspace: s.DestinationWorkspace, List: s.List, Assignee: s.Assignee,
	}, nil
}
