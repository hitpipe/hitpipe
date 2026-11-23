package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/domain/readback"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
)

type ReadAuthority interface {
	AuthorizeRead(context.Context, intake.Query) (Authorization, error)
}

type ObservationFiles interface {
	ContentReader
	intake.ContentStore
}

type QueryService struct {
	db          *Postgres
	files       ObservationFiles
	authority   ReadAuthority
	gate        intake.StartGate
	credentials Credentials
	reads       *readPool
}

func NewQueryService(db *Postgres, files ObservationFiles, authority ReadAuthority, gate intake.StartGate,
	credentials Credentials, clickup *ClickUp) (*QueryService, error) {
	if db == nil || files == nil || authority == nil || gate == nil || credentials == nil || clickup == nil {
		return nil, ErrUnwired
	}
	return &QueryService{db, files, authority, gate, credentials, newReadPool(clickup)}, nil
}

func (s *QueryService) QueryKnownTask(ctx context.Context, q intake.Query) (intake.TaskObservation, error) {
	var zero intake.TaskObservation
	if !time.Now().Before(q.Deadline) {
		return zero, intake.ErrStale
	}
	ctx, cancel := context.WithDeadline(ctx, earlier(q.Deadline, time.Now().Add(8*time.Second)))
	defer cancel()
	a, err := s.authorize(ctx, q)
	if err != nil {
		return zero, err
	}
	var k known
	var expected readback.Expected
	var ex *readExchange
	start, stop := context.WithTimeout(ctx, 2*time.Second)
	err = s.gate.WithOpen(start, q.Workspace, q.Epoch, func() error {
		k, err = s.db.known(start, q, a)
		if err != nil {
			return err
		}
		body, err := s.files.Read(k.ref, time.Now())
		if err != nil {
			return intake.ErrUnavailable
		}
		var p payload
		if json.Unmarshal(body, &p) != nil || !matchesRead(k, p, a) {
			return intake.ErrDenied
		}
		block, caseID, ok := readback.ExtractBlock(p.Description)
		if !ok || caseID != q.Case.String() {
			return intake.ErrConflict
		}
		expected = readback.Expected{Workspace: p.TargetWorkspace, List: p.List, Title: p.Title, Assignee: p.Assignee, HandoffBlock: block}
		cred, err := s.credentials.ForConnection(start, q.Workspace, k.connection, q.Epoch)
		if err != nil || cred.Fingerprint != k.fingerprint {
			return intake.ErrDenied
		}
		req, err := s.reads.upstream.readRequest(k.task, cred.Token)
		if err != nil {
			return intake.ErrConflict
		}
		ex, err = s.reads.start(start, req, func() error { return s.db.chargeRead(start, k) })
		return err
	})
	stop()
	if ex != nil {
		defer ex.finish(false)
	}
	if errors.Is(err, errReadBudget) {
		return zero, intake.ErrQuota
	}
	if errors.Is(err, ErrHeld) || errors.Is(err, ErrUnwired) {
		return zero, intake.ErrUnavailable
	}
	if err != nil {
		return zero, err
	}
	if ex == nil {
		return zero, intake.ErrDenied
	}

	reader := &targetReader{ex: ex, id: k.task}
	readCtx, stopRead := context.WithTimeout(ctx, 4*time.Second)
	result := verifytarget.New(reader).VerifyTarget(readCtx, k.task, expected)
	stopRead()
	var o intake.TaskObservation
	recordCtx, stopRecord := context.WithTimeout(ctx, 2*time.Second)
	defer stopRecord()
	err = s.gate.WithOpen(recordCtx, q.Workspace, q.Epoch, func() error {
		current, err := s.authorize(recordCtx, q)
		if err != nil {
			return err
		}
		if !sameReadAuthorization(a, current) {
			return intake.ErrDenied
		}
		o, err = s.record(recordCtx, k, result, reader.code)
		return err
	})
	if errors.Is(err, ErrHeld) {
		return zero, intake.ErrDenied
	}
	if errors.Is(err, ErrUnwired) {
		return zero, intake.ErrUnavailable
	}
	if err != nil {
		return zero, err
	}
	return o, nil
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *QueryService) authorize(ctx context.Context, q intake.Query) (Authorization, error) {
	a, err := s.authority.AuthorizeRead(ctx, q)
	if err != nil {
		return Authorization{}, err
	}
	if a.Workspace != q.Workspace || a.Case != q.Case || a.Lineage != q.Lineage || a.Attempt == uuid.Nil || a.Epoch != q.Epoch {
		return Authorization{}, intake.ErrDenied
	}
	return a, nil
}

func matchesRead(k known, p payload, a Authorization) bool {
	return a.Connection == k.connection && a.Config == k.config && a.Episode == k.episode && a.TargetWorkspace == k.workspace &&
		a.TargetWorkspace == p.TargetWorkspace && a.List != "" && a.List == p.List && a.Assignee != "" && a.Assignee == p.Assignee &&
		a.ConfigDigest != "" && a.ConfigDigest == p.ConfigDigest
}

func sameReadAuthorization(a, b Authorization) bool {
	return a.Attempt == b.Attempt && a.Config == b.Config && a.Connection == b.Connection && a.Episode == b.Episode &&
		a.ConfigDigest == b.ConfigDigest && a.TargetWorkspace == b.TargetWorkspace && a.List == b.List && a.Assignee == b.Assignee
}

func (s *QueryService) record(ctx context.Context, k known, r verifytarget.Result, code string) (intake.TaskObservation, error) {
	created := time.Now().UTC().Truncate(time.Microsecond)
	o := intake.TaskObservation{MessageID: uuid.New(), SchemaVersion: "1.0", Workspace: k.job.Workspace, Case: k.caseID,
		Lineage: k.lineage, Episode: k.episode, Observation: uuid.New(), TaskID: &k.task, ObservedAt: r.Provenance.ObservedAt,
		Knowledge: "INCOMPLETE", ProofCode: code}
	if !created.Before(k.ref.Expires) || o.ObservedAt.IsZero() || r.Provenance.Source != observationSource {
		return o, intake.ErrUnavailable
	}
	m := r.Comparison
	o.WorkspaceMatches, o.ListMatches, o.AssigneeMatches = m.Workspace, m.List, m.Assignee
	o.TitleMatches, o.MarkerMatches, o.ContextMatches = m.Title, m.Marker, m.Context
	o.Complete = m.Workspace != nil && m.List != nil && m.Assignee != nil && m.Title != nil && m.Marker != nil && m.Context != nil
	switch m.Outcome {
	case readback.Match:
		o.Knowledge = "RESULT_VERIFIED"
	case readback.Mismatch:
		o.Knowledge = "RESULT_MISMATCH"
	}
	expires := earlier(k.ref.Expires, created.Add(90*24*time.Hour))
	var attrs *intake.ContentRef
	if r.Observed.Retrieved && r.Observed.List != "" && r.Observed.Assignees != nil {
		var workspace *string
		if r.Observed.Workspace != "" {
			workspace = &r.Observed.Workspace
		}
		data, err := json.Marshal(map[string]any{"task_id": r.Observed.TargetID, "external_workspace_id": workspace,
			"list_id": r.Observed.List, "assignee_ids": r.Observed.Assignees, "name": r.Observed.Title,
			"handoff_block": m.Block, "retrieved_at": r.Provenance.ObservedAt, "source": r.Provenance.Source})
		if err != nil {
			return o, intake.ErrUnavailable
		}
		ref, err := s.put(data, expires)
		if err != nil {
			return o, err
		}
		attrs = &ref
		o.EvidenceRef, o.EvidenceExpires, o.ProofDigest = &ref.ID, &ref.Expires, ref.SHA256
	} else {
		// This digest attests only the bounded local diagnostic, never absent
		// provider evidence or proof that no external effect occurred.
		o.ProofDigest = checksum([]byte(o.Observation.String() + ":" + o.ProofCode + ":" + o.ObservedAt.Format(time.RFC3339Nano)))
	}
	wire, err := intake.EncodeObservation(&o)
	if err != nil {
		return o, intake.ErrUnavailable
	}
	ref, err := s.put(wire, expires)
	if err != nil {
		return o, err
	}
	return o, s.db.saveReadback(ctx, k, o, attrs, ref, created)
}

func checksum(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }

func (s *QueryService) put(b []byte, expiry time.Time) (intake.ContentRef, error) {
	ref, err := s.files.Put(b, expiry)
	if err != nil || ref.ID == uuid.Nil || ref.SHA256 != checksum(b) || ref.Size != len(b) || !ref.Expires.Equal(expiry) {
		return intake.ContentRef{}, intake.ErrUnavailable
	}
	return ref, nil
}
