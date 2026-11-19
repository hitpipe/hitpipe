package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/recheck"
	"github.com/jackc/pgx/v5"
)

func recheckCommand(t *testing.T, id string, revision int64) drafts.RecoveryCommand {
	t.Helper()
	return drafts.RecoveryCommand{Action: "RECHECK", Reason: "Synthetic follow-up", Command: drafts.CommandMeta{RequestID: id, ExpectedRevision: &revision}}
}

func knownFixture(t *testing.T) (observationFixture, int64) {
	t.Helper()
	f := newObservation(t)
	f.apply(t, f.observation(t, "TARGET_ID_OBSERVED"), 202, "known_for_recheck")
	state, outcome, revision, id, fenced, _ := f.stored(t)
	if state != "UNKNOWN" || outcome != "TARGET_ID_KNOWN" || id == nil || !fenced {
		t.Fatalf("known fixture: %s %s %v %v", state, outcome, id, fenced)
	}
	return f, revision
}

func recoveryStore(f observationFixture) RecheckStore {
	return RecheckStore{CaseReadStore: CaseReadStore{DB: f.repo.DB, Content: f.files}, Files: f.files}
}

func queueInput(t *testing.T, f observationFixture, rev int64) (recheck.Request, recheck.Input, recheck.Grant) {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	q := recheck.Request{WorkspaceID: f.request.WorkspaceID, CaseID: f.request.CaseID, RequestID: id}
	in, err := (recheckhttp.Codec{}).Decode(marshalProcess(t, recheckCommand(t, id, rev)))
	if err != nil {
		t.Fatal(err)
	}
	return q, in, recheck.Grant{WorkspaceID: q.WorkspaceID, OwnerID: owner, ConnectionID: destination, Epoch: 7, ExpiresAt: time.Now().Add(time.Minute)}
}

func recheckCounts(t *testing.T, ws string, n int) {
	t.Helper()
	tx := begin(t, connect(t), ws)
	defer tx.Rollback(ctx)
	for label, q := range map[string]string{
		"ledger":  `SELECT count(*) FROM hp_processing.user_commands WHERE disposition='RECHECK_ACCEPTED'`,
		"outbox":  `SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_QUERY'`,
		"history": `SELECT count(*) FROM hp_processing.safety_history WHERE reason_code='RECHECK_ACCEPTED'`,
		"create":  `SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_INTENT'`,
	} {
		want := n
		if label == "create" {
			want = 1
		}
		var actual int
		if err := tx.QueryRow(ctx, q).Scan(&actual); err != nil || actual != want {
			t.Fatalf("%s got %d wanted %d (%v)", label, actual, want, err)
		}
	}
}

func TestRecheckAtomicReplayAndLease(t *testing.T) {
	f, rev := knownFixture(t)
	store := recoveryStore(f)
	r, in, g := queueInput(t, f, rev)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			d, err := store.Accept(ctx, r, in, g)
			if err != nil || d.Case.State != "UNKNOWN" || d.Case.Revision != rev || d.Case.TargetURL != nil {
				t.Errorf("exact replay status: %+v %v", d.Case, err)
			}
		})
	}
	wg.Wait()
	recheckCounts(t, r.WorkspaceID, 1)
	in.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := store.Accept(ctx, r, in, g); !errors.Is(err, drafts.ErrConflict) {
		t.Fatal("different body", err)
	}
	worker := RecheckDispatch{DispatchStore{DB: dispatchTestDB{}}}
	request := dispatchintent.Request{WorkspaceID: r.WorkspaceID, Epoch: 7}
	if err := worker.Index(ctx, request, 5); err != nil {
		t.Fatal(err)
	}
	jobs, err := worker.Jobs(ctx, r.WorkspaceID, 5)
	if err != nil || len(jobs) != 1 || jobs[0].MessageID != r.RequestID {
		t.Fatal(jobs, err)
	}
	start := make(chan struct{})
	results := make(chan dispatchintent.Claim, 2)
	for range 2 {
		wg.Go(func() {
			<-start
			c, err := worker.Claim(ctx, request, r.RequestID, 10*time.Second)
			if err != nil && !errors.Is(err, dispatchintent.ErrStale) {
				t.Error(err)
			}
			results <- c
		})
	}
	close(start)
	wg.Wait()
	close(results)
	var c dispatchintent.Claim
	for x := range results {
		if x.MessageID != "" {
			if c.MessageID != "" {
				t.Fatal("second live claim")
			}
			c = x
		}
	}
	if c.MessageID != r.RequestID || worker.Check(ctx, c) != nil {
		t.Fatal(c)
	}
	raw, err := f.files.Read(ctx, c.Ref, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (deliveryhttp.QueryCodec{}).Decode(raw, c, time.Now()); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*dispatchintent.Claim){
		"workspace": func(x *dispatchintent.Claim) { x.WorkspaceID = owner },
		"epoch":     func(x *dispatchintent.Claim) { x.Epoch++ },
		"lineage":   func(x *dispatchintent.Claim) { x.LineageID = owner },
		"task":      func(x *dispatchintent.Claim) { x.NativeTaskID = "other" },
		"revision":  func(x *dispatchintent.Claim) { x.Revision++ },
	} {
		x := c
		change(&x)
		if _, err = (deliveryhttp.QueryCodec{}).Decode(raw, x, time.Now()); err == nil {
			t.Fatal("accepted altered " + name)
		}
	}
	if _, err = (deliveryhttp.QueryCodec{}).Decode(raw, c, c.DeadlineAt); err == nil {
		t.Fatal("accepted expired query")
	}
}

type recheckCommitFault struct {
	Beginner
	ws   string
	lost bool
}

func (f recheckCommitFault) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	tx, err := f.Beginner.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	if f.lost {
		return lostCaseCommitReply{tx}, nil
	}
	return badCommitTx{tx, f.ws}, nil
}

func TestRecheckRollbackOrLostCommitReply(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "rollback"
		if lost {
			name = "committed_reply_lost"
		}
		t.Run(name, func(t *testing.T) {
			f, rev := knownFixture(t)
			normal := recoveryStore(f)
			r, in, g := queueInput(t, f, rev)
			fault := normal
			fault.DB = recheckCommitFault{normal.DB, r.WorkspaceID, lost}
			if _, err := fault.Accept(ctx, r, in, g); !errors.Is(err, drafts.ErrUnavailable) {
				t.Fatalf("unconfirmed commit returned %v", err)
			}
			want := 0
			if lost {
				want = 1
			}
			recheckCounts(t, r.WorkspaceID, want)
			if _, err := normal.Accept(ctx, r, in, g); err != nil {
				t.Fatal(err)
			}
			recheckCounts(t, r.WorkspaceID, 1)
		})
	}
}

func TestRecheckRejectsUnprovedLineage(t *testing.T) {
	f, rev := knownFixture(t)
	s := recoveryStore(f)
	r, in, g := queueInput(t, f, rev)
	tx := begin(t, connect(t), r.WorkspaceID)
	if _, err := tx.Exec(ctx, `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at)
 VALUES($1,gen_random_uuid(),$2,statement_timestamp(),statement_timestamp())`, r.WorkspaceID, r.CaseID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Accept(ctx, r, in, g); !errors.Is(err, drafts.ErrConflict) {
		t.Fatal(err)
	}
	recheckCounts(t, r.WorkspaceID, 0)
}
