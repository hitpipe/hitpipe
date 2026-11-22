package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
)

var (
	ErrHeld    = errors.New("delivery attempt held")
	ErrFenced  = errors.New("delivery attempt already fenced")
	ErrUnwired = errors.New("delivery dispatch dependency unavailable")
)

type Job struct {
	Workspace, Attempt uuid.UUID
}

// Authorization is the current decision for a saved attempt. The authority
// owner supplies it independently of the worker's job or its content.
type Authorization struct {
	Workspace, Attempt, Case, Config, Connection, Lineage, Episode uuid.UUID
	Epoch                                                          int64
	ConfigDigest, FactsDigest                                      string
	FactsUntil, Deadline                                           time.Time
	TargetWorkspace, List, Assignee                                string
}

type Authority interface {
	Current(context.Context, Job) (Authorization, error)
}

type Credential struct {
	Token       string
	Fingerprint [32]byte
}

type Credentials interface {
	ForConnection(context.Context, uuid.UUID, uuid.UUID, int64) (Credential, error)
}

type ContentReader interface {
	Read(intake.ContentRef, time.Time) ([]byte, error)
}

type saved struct {
	auth        Authorization
	ref         intake.ContentRef
	fingerprint [32]byte
}

type payload struct {
	Title           string `json:"title"`
	Description     string `json:"rendered_handoff_block"`
	TargetWorkspace string `json:"destination_workspace_id"`
	List            string `json:"target_list_id"`
	Assignee        string `json:"assignee_id"`
	ConfigDigest    string `json:"config_digest"`
}

type disposition string

const (
	TargetIDObserved disposition = "TARGET_ID_OBSERVED"
	Ambiguous        disposition = "AMBIGUOUS"
)

type Result struct {
	Disposition disposition
	TaskID      string
}

type Worker struct {
	ledger      *Postgres
	content     ContentReader
	authority   Authority
	gate        intake.StartGate
	credentials Credentials
	clickup     *ClickUp
}

func NewWorker(ledger *Postgres, content ContentReader, authority Authority, gate intake.StartGate, credentials Credentials, clickup *ClickUp) (*Worker, error) {
	if ledger == nil || content == nil || authority == nil || gate == nil || credentials == nil || clickup == nil {
		return nil, ErrUnwired
	}
	return &Worker{ledger, content, authority, gate, credentials, clickup}, nil
}

func (w *Worker) Run(ctx context.Context, job Job) (Result, error) {
	if job.Workspace == uuid.Nil || job.Attempt == uuid.Nil {
		return Result{}, ErrHeld
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	auth, err := w.current(ctx, job)
	if err != nil {
		return Result{}, err
	}
	var rec saved
	var target payload
	readCtx, stopReading := context.WithTimeout(ctx, 2*time.Second)
	err = w.gate.WithOpen(readCtx, job.Workspace, auth.Epoch, func() error {
		var err error
		rec, err = w.ledger.load(readCtx, job)
		if err != nil {
			return err
		}
		body, err := w.content.Read(rec.ref, time.Now())
		if err != nil || json.Unmarshal(body, &target) != nil {
			return ErrHeld
		}
		return checkBinding(rec, target, auth)
	})
	stopReading()
	if err != nil {
		return Result{}, err
	}

	var ex *exchange
	fenced := false
	deadline := time.Now().Add(2 * time.Second)
	for _, limit := range []time.Time{rec.auth.FactsUntil, rec.auth.Deadline, rec.ref.Expires} {
		if limit.Before(deadline) {
			deadline = limit
		}
	}
	startCtx, stop := context.WithDeadline(ctx, deadline)
	defer stop()
	err = w.gate.WithOpen(startCtx, job.Workspace, auth.Epoch, func() error {
		current, err := w.current(startCtx, job)
		if err != nil {
			return err
		}
		if err = checkBinding(rec, target, current); err != nil {
			return err
		}
		cred, err := w.credentials.ForConnection(startCtx, job.Workspace, auth.Connection, auth.Epoch)
		if err != nil || cred.Fingerprint != rec.fingerprint {
			return ErrHeld
		}
		request, err := w.clickup.request(target, cred.Token)
		if err != nil {
			return err
		}
		if err = w.ledger.fence(startCtx, rec); err != nil {
			return err
		}
		fenced = true
		ex, err = w.clickup.start(startCtx, request, func() error {
			if !time.Now().Before(rec.ref.Expires) {
				return ErrHeld
			}
			return w.ledger.markStart(startCtx, rec)
		})
		return err
	})
	if ex != nil {
		defer ex.conn.Close()
	}
	if !fenced {
		if err == nil {
			return Result{}, ErrHeld
		}
		return Result{}, err
	}
	out := outcome{result: Result{Disposition: Ambiguous}, code: "DISPATCH_OUTCOME_UNKNOWN"}
	if err == nil && ex != nil {
		out = ex.receive(ctx)
	}

	// Request cancellation cannot reset the durable fence or suppress a known
	// result. Use a distinct, bounded context for recording the observation.
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer persistCancel()
	if e := w.ledger.finish(persistCtx, rec, out); e != nil {
		return Result{Disposition: Ambiguous}, ErrUnwired
	}
	return out.result, nil
}

func (w *Worker) current(ctx context.Context, job Job) (Authorization, error) {
	if ctx.Err() != nil {
		return Authorization{}, ErrHeld
	}
	a, err := w.authority.Current(ctx, job)
	if err != nil || a.Workspace != job.Workspace || a.Attempt != job.Attempt || a.Epoch <= 0 ||
		!time.Now().Before(a.FactsUntil) || !time.Now().Before(a.Deadline) {
		return Authorization{}, ErrHeld
	}
	return a, nil
}

func checkBinding(r saved, p payload, a Authorization) error {
	i := r.auth
	if i.Workspace != a.Workspace || i.Attempt != a.Attempt || i.Case != a.Case ||
		i.Connection != a.Connection || i.Config != a.Config || i.Lineage != a.Lineage || i.Episode != a.Episode ||
		i.Epoch != a.Epoch || i.FactsDigest != a.FactsDigest ||
		!i.FactsUntil.Equal(a.FactsUntil.UTC().Truncate(time.Microsecond)) ||
		!i.Deadline.Equal(a.Deadline.UTC().Truncate(time.Microsecond)) ||
		p.TargetWorkspace != i.TargetWorkspace || p.TargetWorkspace != a.TargetWorkspace ||
		p.List == "" || p.List != a.List || p.Assignee == "" || p.Assignee != a.Assignee ||
		p.ConfigDigest == "" || p.ConfigDigest != a.ConfigDigest {
		return ErrHeld
	}
	return nil
}
