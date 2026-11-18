package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

func takeCaseJob(t *testing.T, jobs CaseJobStore, workspace string) CaseClaim {
	t.Helper()
	if err := jobs.Index(ctx, workspace, 5); err != nil {
		t.Fatal(err)
	}
	c, err := jobs.Claim(ctx, workspace, 7)
	if err != nil || c.CaseID == "" {
		t.Fatalf("missing claim: %+v %v", c, err)
	}
	return c
}

func workerService(f preparedFixture, db Beginner, claim CaseClaim) preparecase.Service {
	s := f.service
	s.Repository = ClaimedDecisionStore{DecisionStore{DB: db, Content: f.files}, claim}
	return s
}

func expireCaseClaim(t *testing.T, db Beginner, c CaseClaim) {
	t.Helper()
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE hp_processing.job_index SET lease_until=clock_timestamp() WHERE workspace_id=$1 AND job_id=$2`, c.WorkspaceID, c.CaseID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCaseWorkerDoesNotIndexFailedAdmission(t *testing.T) {
	ws := workspace(t)
	db := connect(t)
	raw := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
	store := admissionStore(t, badCommitDB{db, ws})
	got, err := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, store)
	if err == nil || got.RecordID != "" {
		t.Fatalf("failed commit acknowledged: %v %v", got, err)
	}
	jobs := CaseJobStore{DB: dispatchTestDB{}}
	if err = jobs.Index(ctx, ws, 5); err != nil {
		t.Fatal(err)
	}
	claim, err := jobs.Claim(ctx, ws, 7)
	if err != nil || claim.CaseID != "" {
		t.Fatalf("rolled back work is visible: %+v %v", claim, err)
	}
	counts(t, ws, 0, 0, 0)
}

func TestCaseWorkerConcurrentLeaseAndRLS(t *testing.T) {
	f := prepared(t, []string{})
	jobs := CaseJobStore{DB: dispatchTestDB{}}
	ws := f.request.WorkspaceID
	if err := (CaseJobStore{DB: connect(t)}).Index(ctx, ws, 5); err == nil {
		t.Fatal("HTTP role gained access to job_index")
	}
	if err := jobs.Index(ctx, ws, 5); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type answer struct {
		c CaseClaim
		e error
	}
	results := make(chan answer, 2)
	for range 2 {
		go func() {
			<-start
			c, e := jobs.Claim(ctx, ws, 7)
			results <- answer{c, e}
		}()
	}
	close(start)
	var c CaseClaim
	for range 2 {
		got := <-results
		if got.e != nil {
			t.Fatal(got.e)
		}
		if got.c.CaseID != "" {
			if c.CaseID != "" {
				t.Fatal("two live claim owners")
			}
			c = got.c
		}
	}
	if c.CaseID != f.request.CaseID || c.ExpectedRevision != f.request.ExpectedRevision {
		t.Fatal(c)
	}
	foreign := c
	foreign.WorkspaceID = owner
	if err := jobs.CheckStart(ctx, foreign); !errors.Is(err, preparecase.ErrStale) {
		t.Fatalf("wrong workspace: %v", err)
	}
	if err := jobs.CheckStart(ctx, c); err != nil {
		t.Fatal(err)
	}

	stale := workerService(f, jobs.DB, c)
	stale.Readers = fixtureReaders{after: func() {
		tx := begin(t, connect(t), ws)
		if _, err := tx.Exec(ctx, `SELECT 1 FROM hp_processing.operations WHERE workspace_id=$1 FOR UPDATE NOWAIT`, ws); err != nil {
			t.Fatalf("SQL lock held during read: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		expireCaseClaim(t, jobs.DB, c)
	}}
	if _, err := stale.Prepare(ctx, c.Request); !errors.Is(err, preparecase.ErrStale) {
		t.Fatalf("stale lease committed: %v", err)
	}
	decisionCounts(t, ws, 0, 1, 0)
	restarted := CaseJobStore{DB: dispatchTestDB{}}
	fresh := takeCaseJob(t, restarted, ws)
	if fresh.CaseID != c.CaseID || fresh.ExpectedRevision != c.ExpectedRevision || fresh.token == c.token {
		t.Fatalf("wrong recovery: %+v", fresh)
	}
	if err := restarted.CheckStart(ctx, c); !errors.Is(err, preparecase.ErrStale) {
		t.Fatalf("old token could start: %v", err)
	}
	s := workerService(f, restarted.DB, fresh)
	if r, err := s.Prepare(ctx, fresh.Request); err != nil || r.State != "WORKING" {
		t.Fatalf("recovered decision: %+v %v", r, err)
	}
	decisionCounts(t, ws, 1, 2, 1)
}

func TestCaseWorkerChangesDuringReadDoNotCommit(t *testing.T) {
	for _, mode := range []string{"draining", "selected_config_removed", "expired_episode", "later_signal"} {
		t.Run(mode, func(t *testing.T) {
			f := prepared(t, []string{})
			ws := f.request.WorkspaceID
			jobs := CaseJobStore{DB: dispatchTestDB{}}
			claim := takeCaseJob(t, jobs, ws)
			s := workerService(f, jobs.DB, claim)
			s.Readers = fixtureReaders{after: func() {
				db := connect(t)
				if mode == "later_signal" {
					raw := wire(t, ws, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), "checkout.session.async_payment_succeeded")
					got, err := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, Store{DB: db, Content: f.files})
					if err != nil || got.RecordID != claim.CaseID {
						t.Fatalf("new event did not attach to same case: %v %v", got, err)
					}
					return
				}
				tx := begin(t, db, ws)
				sql := map[string]string{
					"draining":                `UPDATE hp_processing.operations SET state='DRAINING',revision=revision+1 WHERE workspace_id=$1`,
					"selected_config_removed": `UPDATE hp_processing.operations SET selected_config_id=NULL,revision=revision+1 WHERE workspace_id=$1`,
					"expired_episode": `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at)
 VALUES($1,gen_random_uuid(),(SELECT case_id FROM hp_processing.case_identity WHERE workspace_id=$1),now(),now())`,
				}[mode]
				if _, err := tx.Exec(ctx, sql, ws); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}}
			if _, err := s.Prepare(ctx, claim.Request); !errors.Is(err, preparecase.ErrStale) {
				t.Fatalf("stale work committed: %v", err)
			}
			if err := jobs.CheckStart(ctx, claim); !errors.Is(err, preparecase.ErrStale) {
				t.Fatalf("stale source start allowed: %v", err)
			}
			episodes := 0
			if mode == "expired_episode" {
				episodes = 1
			}
			decisionCounts(t, ws, 0, 1, episodes)
		})
	}
}

var errCaseCommitReplyLost = errors.New("controlled lost commit reply")

type caseCommitFault struct {
	Beginner
	workspace string
	loseReply bool
	loadSeen  bool
}

func (f *caseCommitFault) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, err := f.Beginner.BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	if !f.loadSeen {
		f.loadSeen = true
		return tx, nil
	}
	if f.loseReply {
		return lostCaseCommitReply{tx}, nil
	}
	return badCommitTx{tx, f.workspace}, nil
}

type lostCaseCommitReply struct{ pgx.Tx }

func (tx lostCaseCommitReply) Commit(ctx context.Context) error {
	if err := tx.Tx.Commit(ctx); err != nil {
		return err
	}
	return errCaseCommitReplyLost
}

func TestCaseWorkerRecoversCommitOutcomeWithoutSecondIntent(t *testing.T) {
	for _, mode := range []string{"rolled_back", "committed_reply_lost"} {
		t.Run(mode, func(t *testing.T) {
			f := prepared(t, []string{})
			ws := f.request.WorkspaceID
			jobs := CaseJobStore{DB: dispatchTestDB{}}
			claim := takeCaseJob(t, jobs, ws)
			loseReply := mode == "committed_reply_lost"
			s := workerService(f, &caseCommitFault{Beginner: jobs.DB, workspace: ws, loseReply: loseReply}, claim)
			if _, err := s.Prepare(ctx, claim.Request); err == nil || loseReply != errors.Is(err, errCaseCommitReplyLost) {
				t.Fatalf("unverified outcome: %v", err)
			}
			if !loseReply {
				decisionCounts(t, ws, 0, 1, 0)
				expireCaseClaim(t, jobs.DB, claim)
				newClaim := takeCaseJob(t, jobs, ws)
				s = workerService(f, jobs.DB, newClaim)
				if r, err := s.Prepare(ctx, newClaim.Request); err != nil || r.State != "WORKING" {
					t.Fatalf("retry after rollback: %+v %v", r, err)
				}
			}
			decisionCounts(t, ws, 1, 2, 1)
			restarted := CaseJobStore{DB: dispatchTestDB{}}
			if err := restarted.Index(ctx, ws, 5); err != nil {
				t.Fatal(err)
			}
			if next, err := restarted.Claim(ctx, ws, 7); err != nil || next.CaseID != "" {
				t.Fatalf("committed case was scheduled again: %+v %v", next, err)
			}
		})
	}
}
