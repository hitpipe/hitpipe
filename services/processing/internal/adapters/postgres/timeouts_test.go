package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

func timerStore(f preparedFixture, now *time.Time) CaseTimeoutStore {
	return CaseTimeoutStore{DB: dispatchTestDB{}, Files: f.files, History: f.files, Clock: testClock{func() time.Time { return *now }}}
}

func timerClaim(t *testing.T, s CaseTimeoutStore, ws, id string) TimeoutClaim {
	t.Helper()
	if err := s.Index(ctx, ws, owner, 5); err != nil {
		t.Fatal(err)
	}
	c, err := s.Claim(ctx, ws, owner)
	if err != nil || c.CaseID != id {
		t.Fatalf("unexpected claim %+v %v", c, err)
	}
	return c
}

func timerSourceTimes(t *testing.T, ws, id, state string) (time.Time, time.Time) {
	t.Helper()
	tx := begin(t, connect(t), ws)
	defer tx.Rollback(ctx)
	src, err := loadTimeoutSource(ctx, tx, ws, id, state)
	if err != nil {
		t.Fatal(err)
	}
	due, code := src.Deadline(time.Now())
	if code != "" {
		t.Fatal(code)
	}
	return src.Entered, due
}

type timerSnapshot struct {
	state, knowledge, reason, lineage, task, outbox string
	revision                                        int64
	fenced                                          bool
	at                                              time.Time
	closed                                          *time.Time
	intents, notice, entries                        int
}

func timerCase(t *testing.T, ws, id string) timerSnapshot {
	t.Helper()
	tx := begin(t, connect(t), ws)
	defer tx.Rollback(ctx)
	var s timerSnapshot
	err := tx.QueryRow(ctx, `SELECT k.state,k.outcome_knowledge,k.revision,k.ever_dispatch_fenced,COALESCE(k.lineage_id::text,''),
 COALESCE(k.known_task_id,''),h.reason_code,h.at,COALESCE(o.state,''),
 (SELECT completed_at FROM hp_processing.episodes WHERE workspace_id=k.workspace_id AND case_id=k.case_id ORDER BY started_at DESC LIMIT 1),
 (SELECT count(*) FROM hp_processing.outbox WHERE workspace_id=k.workspace_id AND message_id=k.lineage_id),
 (SELECT count(*) FROM hp_processing.notifications WHERE workspace_id=k.workspace_id AND case_id=k.case_id),
 (SELECT count(*) FROM hp_processing.safety_history WHERE workspace_id=k.workspace_id AND case_id=k.case_id)
 FROM hp_processing.case_identity k JOIN LATERAL (
 SELECT reason_code,at FROM hp_processing.safety_history WHERE workspace_id=k.workspace_id AND case_id=k.case_id
 ORDER BY at DESC,entry_id DESC LIMIT 1) h ON true
 LEFT JOIN hp_processing.outbox o ON (o.workspace_id,o.message_id)=(k.workspace_id,k.lineage_id)
 WHERE k.workspace_id=$1 AND k.case_id=$2`, ws, id).Scan(&s.state, &s.knowledge, &s.revision, &s.fenced, &s.lineage,
		&s.task, &s.reason, &s.at, &s.outbox, &s.closed, &s.intents, &s.notice, &s.entries)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func notDue(t *testing.T, s CaseTimeoutStore, ws string) {
	t.Helper()
	if err := s.Index(ctx, ws, owner, 5); err != nil {
		t.Fatal(err)
	}
	if c, err := s.Claim(ctx, ws, owner); err != nil || c.CaseID != "" {
		t.Fatalf("not due %+v %v", c, err)
	}
}

func waitCase(t *testing.T) preparedFixture {
	t.Helper()
	f := prepared(t, []string{})
	s := f.service
	s.Readers = fixtureReaders{change: func(m map[string]any) { m["payment_status"] = "unpaid" }}
	if r, e := s.Prepare(ctx, f.request); e != nil || r.State != "WAITING_DATA" {
		t.Fatal(r, e)
	}
	return f
}

func TestCaseTimeoutWorkingExactDeadlineAndFate(t *testing.T) {
	f := prepared(t, []string{})
	ws, id := f.request.WorkspaceID, f.request.CaseID
	r, err := f.service.Prepare(ctx, f.request)
	if err != nil || r.State != "WORKING" {
		t.Fatal(r, err)
	}
	tx := begin(t, connect(t), ws)
	if _, err = tx.Exec(ctx, `UPDATE hp_processing.case_identity SET outcome_knowledge='POSSIBLE_EFFECT',ever_dispatch_fenced=true WHERE workspace_id=$1 AND case_id=$2`, ws, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	start, due := timerSourceTimes(t, ws, id, "WORKING")
	if due.Sub(start) != 300*time.Second {
		t.Fatalf("bad fixture %s %s", start, due)
	}
	now := due.Add(-time.Microsecond)
	s := timerStore(f, &now)
	notDue(t, s, ws)
	delivery := DispatchStore{DB: dispatchTestDB{}}
	req := dispatchintent.Request{WorkspaceID: ws, Epoch: 7}
	if err = delivery.Index(ctx, req, 5); err != nil {
		t.Fatal(err)
	}
	sent, err := delivery.Claim(ctx, req, r.MessageID, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now = due
	c := timerClaim(t, s, ws, id)
	got, err := s.Apply(ctx, c)
	if err != nil || got.State != "UNKNOWN" || got.Reason != casetimeouts.WorkingElapsed {
		t.Fatalf("%+v %v", got, err)
	}
	d := timerCase(t, ws, id)
	if d.state != "UNKNOWN" || d.knowledge != "POSSIBLE_EFFECT" || !d.fenced || d.lineage != r.MessageID ||
		d.outbox != "HELD" || d.intents != 1 || d.notice != 1 || d.entries != 3 || d.closed == nil || !d.closed.Equal(due) || !d.at.Equal(due) {
		t.Fatalf("wrong durable outcome %+v", d)
	}
	if delivery.Check(ctx, sent) == nil {
		t.Fatal("stale delivery lease can still start")
	}
	if jobs, err := delivery.Jobs(ctx, ws, 5); err != nil || len(jobs) != 0 {
		t.Fatal(jobs, err)
	}
	if _, err = s.Apply(ctx, c); !errors.Is(err, ErrTimeoutLease) {
		t.Fatal(err)
	}
	notDue(t, s, ws)
}

func TestCaseTimeoutWaitingContinuousNotExtended(t *testing.T) {
	f := waitCase(t)
	ws, id := f.request.WorkspaceID, f.request.CaseID
	first, due := timerSourceTimes(t, ws, id, "WAITING_DATA")
	if due.Sub(first) != 24*time.Hour {
		t.Fatalf("%s %s", first, due)
	}
	// A distinct signal plus its exact replay and a fresh unpaid decision
	// leave the same continuous run. The replacement worker has no memory.
	raw := wire(t, ws, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), "checkout.session.async_payment_succeeded")
	db := connect(t)
	accept := Store{DB: db, Content: f.files}
	for range 2 {
		r, e := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, accept)
		if e != nil || r.RecordID != id {
			t.Fatal(r, e)
		}
	}
	req := f.request
	req.ExpectedRevision = 2
	again := f.service
	again.Readers = fixtureReaders{change: func(m map[string]any) { m["payment_status"] = "unpaid" }}
	if r, e := again.Prepare(ctx, req); e != nil || r.State != "WAITING_DATA" {
		t.Fatal(r, e)
	}
	if a, b := timerSourceTimes(t, ws, id, "WAITING_DATA"); !a.Equal(first) || !b.Equal(due) {
		t.Fatal(a, b)
	}
	now := due.Add(-time.Microsecond)
	notDue(t, timerStore(f, &now), ws)
	now = due.Add(4*time.Minute + 59*time.Second)
	restarted := timerStore(f, &now)
	c := timerClaim(t, restarted, ws, id)
	r, e := restarted.Apply(ctx, c)
	if e != nil || r.State != "ATTENTION" || r.Reason != casetimeouts.WaitingElapsed {
		t.Fatal(r, e)
	}
	v := timerCase(t, ws, id)
	if v.state != "ATTENTION" || v.knowledge != "UNESTABLISHED" || v.revision != 4 || v.intents != 0 || v.notice != 1 ||
		!v.at.Equal(now) || v.at.Sub(due) >= 5*time.Minute {
		t.Fatalf("%+v", v)
	}
	notDue(t, restarted, ws)
}

func TestCaseTimeoutRacesPreserveCurrentDecision(t *testing.T) {
	t.Run("fresh_financial_decision_precedes_timer", func(t *testing.T) {
		f := waitCase(t)
		ws, id := f.request.WorkspaceID, f.request.CaseID
		_, now := timerSourceTimes(t, ws, id, "WAITING_DATA")
		s := timerStore(f, &now)
		c := timerClaim(t, s, ws, id)
		raw := wire(t, ws, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), "checkout.session.async_payment_succeeded")
		receipt, e := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, Store{DB: connect(t), Content: f.files})
		if e != nil || receipt.RecordID != id {
			t.Fatal(receipt, e)
		}
		financial := takeCaseJob(t, CaseJobStore{DB: s.DB}, ws)
		r, e := workerService(f, s.DB, financial).Prepare(ctx, financial.Request)
		if e != nil || r.State != "WORKING" {
			t.Fatal(r, e)
		}
		if r, e := s.Apply(ctx, c); e != nil || r.CaseID != "" {
			t.Fatal(r, e)
		}
		v := timerCase(t, ws, id)
		if v.state != "WORKING" || v.notice != 0 || v.intents != 1 || v.lineage != r.MessageID {
			t.Fatal(v)
		}
	})
	t.Run("timer_precedes_in_flight_financial_read", func(t *testing.T) {
		f := waitCase(t)
		ws, id := f.request.WorkspaceID, f.request.CaseID
		_, now := timerSourceTimes(t, ws, id, "WAITING_DATA")
		s := timerStore(f, &now)
		read, release := make(chan struct{}), make(chan struct{})
		service := f.service
		service.Readers = fixtureReaders{after: func() { close(read); <-release }}
		req := f.request
		req.ExpectedRevision = 2
		out := make(chan error, 1)
		go func() { _, err := service.Prepare(ctx, req); out <- err }()
		select {
		case <-read:
		case <-time.After(3 * time.Second):
			close(release)
			t.Fatal("read did not start")
		}
		r, e := s.Apply(ctx, timerClaim(t, s, ws, id))
		close(release)
		if e != nil || r.State != "ATTENTION" {
			t.Fatal(r, e)
		}
		if e = <-out; !errors.Is(e, preparecase.ErrStale) {
			t.Fatalf("old decision could commit: %v", e)
		}
		if v := timerCase(t, ws, id); v.intents != 0 || v.notice != 1 || v.state != "ATTENTION" {
			t.Fatal(v)
		}
	})
	t.Run("confirmed_observation_precedes_timer", func(t *testing.T) {
		f := newObservation(t)
		ws, id := f.request.WorkspaceID, f.request.CaseID
		_, now := timerSourceTimes(t, ws, id, "WORKING")
		s := timerStore(f.preparedFixture, &now)
		c := timerClaim(t, s, ws, id)
		f.apply(t, f.observation(t, "RESULT_VERIFIED"), 202, "confirmed_before_stale_timer")
		if r, e := s.Apply(ctx, c); e != nil || r.CaseID != "" {
			t.Fatal(r, e)
		}
		v := timerCase(t, ws, id)
		if v.state != "CONFIRMED" || v.knowledge != "RESULT_VERIFIED" || !v.fenced || v.task != "86synthetic99" ||
			v.notice != 0 || v.intents != 1 || v.lineage != f.lineage {
			t.Fatal(v)
		}
	})
}

// seedTimerPeriod puts a labelled aged period into the actual owned database.
// Expiring contents use the same format and writer as the real producer.
func seedTimerPeriod(t *testing.T, f preparedFixture, state string, entered time.Time, mode string) string {
	t.Helper()
	ws := f.request.WorkspaceID
	id, _ := newID()
	tx := begin(t, connect(t), ws)
	entered = entered.UTC().Truncate(time.Microsecond)
	admitted := entered.Add(-time.Minute)
	if mode == "future" {
		// Keep the admitted case visible. The transition timestamp is the
		// only invalid input in this scenario.
		admitted = entered.Add(-2 * time.Hour)
	}
	var lineage any
	if state == "WORKING" {
		lineage, _ = newID()
	}
	_, e := tx.Exec(ctx, `INSERT INTO hp_processing.case_identity(workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge,lineage_id)
 VALUES($1,$2,$3,$4,'acct_synthetic',false,$2::uuid::text,$5,$6,$7,2,'UNESTABLISHED',$8)`, ws, id, op, config, admitted, owner, state, lineage)
	if e != nil {
		t.Fatal(e)
	}
	write := func(old string, d preparecase.Decision, expiry time.Time) preparecase.Ref {
		t.Helper()
		raw, e := json.Marshal(struct {
			CaseID   string
			Revision int64
			Decision preparecase.Decision
		}{id, 1, d})
		if e != nil {
			t.Fatal(e)
		}
		d.Evidence, e = f.files.Write(ctx, raw, "HISTORY", preparecase.Origin{WorkspaceID: ws, CaseID: id, Revision: 1}, expiry, d.At)
		if e == nil {
			e = appendDecisionHistory(ctx, tx, ws, id, old, d)
		}
		if e != nil {
			t.Fatal(e)
		}
		return d.Evidence
	}
	write("", preparecase.Decision{At: admitted, Reason: "SIGNAL_ADMITTED", State: "RECEIVED"}, admitted.Add(90*24*time.Hour))
	if mode != "missing_transition" {
		d := preparecase.Decision{At: entered, State: state, Reason: "PAYMENT_STATUS"}
		if state == "WORKING" {
			d.Reason = "FINANCIAL_ELIGIBILITY_ONLY"
			d.DeadlineAt = entered.Add(300 * time.Second)
			d.LineageID = lineage.(string)
			d.MessageID = d.LineageID
			d.Intent = &preparecase.Ref{ID: id}
			if mode != "missing_episode" {
				d.EpisodeID, _ = newID()
				_, e = tx.Exec(ctx, `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at)
 VALUES($1,$2,$3,$4,$5)`, ws, d.EpisodeID, id, entered, d.DeadlineAt)
				if e != nil {
					t.Fatal(e)
				}
			}
		}
		expiry := entered.Add(90 * 24 * time.Hour)
		if mode == "expired" {
			expiry = entered.Add(time.Minute)
		}
		ref := write("RECEIVED", d, expiry)
		if mode == "corrupt" {
			if e = os.WriteFile(filepath.Join(f.path, ref.ID), []byte("test corruption"), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return id
}

type timerFault struct {
	Beginner
	ws string
}

func (f timerFault) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := f.Beginner.BeginTx(ctx, o)
	if e != nil {
		return nil, e
	}
	return badCommitTx{tx, f.ws}, nil
}

func TestCaseTimeoutJobsBoundedLeaseCancelRollback(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := preparedSince(t, []string{}, now.Add(-72*time.Hour))
	ws := f.request.WorkspaceID
	for range 3 {
		seedTimerPeriod(t, f, "WAITING_DATA", now.Add(-25*time.Hour), "")
	}
	s := timerStore(f, &now)
	if e := s.Index(ctx, ws, owner, 2); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan TimeoutClaim, 3)
	for range 3 {
		wg.Go(func() {
			c, e := s.Claim(ctx, ws, owner)
			if e != nil {
				t.Error(e)
			}
			results <- c
		})
	}
	wg.Wait()
	close(results)
	var claims []TimeoutClaim
	for c := range results {
		if c.CaseID != "" {
			claims = append(claims, c)
		}
	}
	if len(claims) != 2 || claims[0].CaseID == claims[1].CaseID {
		t.Fatal(claims)
	}
	c := claims[0]
	wrong := c
	wrong.WorkspaceID = owner
	if _, e := s.Apply(ctx, wrong); !errors.Is(e, ErrTimeoutLease) {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := s.Apply(cancelled, c); e == nil {
		t.Fatal("cancelled attempt committed")
	}
	fault := s
	fault.DB = timerFault{s.DB, ws}
	if _, e := fault.Apply(ctx, c); e == nil {
		t.Fatal("failed commit reported success")
	}
	if v := timerCase(t, ws, c.CaseID); v.revision != 2 || v.notice != 0 || v.entries != 2 {
		t.Fatal(v)
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.job_index SET lease_until=clock_timestamp() WHERE workspace_id=$1 AND job_id=$2`, ws, c.jobID); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, c); !errors.Is(e, ErrTimeoutLease) {
		t.Fatal(e)
	}
	restarted := timerStore(f, &now)
	fresh, e := restarted.Claim(ctx, ws, owner)
	if e != nil || fresh.CaseID != c.CaseID || fresh.token == c.token {
		t.Fatal(fresh, e)
	}
	if r, e := restarted.Apply(ctx, fresh); e != nil || r.State != "ATTENTION" {
		t.Fatal(r, e)
	}
	if v := timerCase(t, ws, c.CaseID); v.notice != 1 || v.revision != 3 {
		t.Fatal(v)
	}
}
