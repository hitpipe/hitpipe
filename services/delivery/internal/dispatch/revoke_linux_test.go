package dispatch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresRevocationAfterStart(t *testing.T) {
	f := setup(t)
	fp := f.budget(t, 3)
	job, auth := f.prepare(t, fp, false)
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	var revoked atomic.Bool
	var sends atomic.Int64
	gate := allowGate(func(ctx context.Context, _ uuid.UUID, _ int64, run func() error) error {
		select {
		case <-lock:
			defer func() { lock <- struct{}{} }()
		case <-ctx.Done():
			return ErrHeld
		}
		if revoked.Load() {
			return ErrHeld
		}
		return run()
	})
	client := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-lock:
			defer func() { lock <- struct{}{} }()
		case <-r.Context().Done():
			return
		}
		if _, err := f.admin.Exec(r.Context(), "UPDATE hp_delivery.destination_connections SET revoked_at=clock_timestamp(), security_epoch=2 WHERE workspace_id=$1", job.Workspace); err != nil {
			t.Error("fixture revocation failed:", err)
			return
		}
		revoked.Store(true)
		_, _ = io.WriteString(w, `{"id":"in-flight-task"}`)
	}))
	worker := f.newWorker(t, client, auth, fp)
	worker.gate = gate
	out, err := worker.Run(t.Context(), job)
	if err != nil || out.Disposition != TargetIDObserved || out.TaskID != "in-flight-task" || !revoked.Load() {
		t.Fatalf("the previously started request must be recorded after revocation: %+v %v", out, err)
	}
	if _, err := worker.Run(t.Context(), job); !errors.Is(err, ErrHeld) || sends.Load() != 1 {
		t.Fatal("new start was admitted after the gate closed")
	}
	t.Log("in-flight request returned its identity after local revocation; new starts=0")
}

func TestClickUpRefusesAmbiguousTargets(t *testing.T) {
	client := NewClickUp(nil)
	if _, err := NewWorker(nil, nil, nil, nil, nil, client); !errors.Is(err, ErrUnwired) {
		t.Fatal("unwired worker admitted")
	}
	for _, p := range []payload{
		{List: "https://other.example/list", Assignee: "123", Title: "name", Description: "description"},
		{List: "456/../999", Assignee: "123", Title: "name", Description: "description"},
		{List: "456", Assignee: "9223372036854775808", Title: "name", Description: "description"},
	} {
		if _, err := client.request(p, fixtureToken); !errors.Is(err, ErrHeld) {
			t.Fatal("unexpected acceptance of unsafe target")
		}
	}
	req, err := client.request(payload{List: "456", Assignee: "123", Title: "name", Description: "description"}, fixtureToken)
	if err != nil || req.URL.Host != "api.clickup.com" || req.URL.Scheme != "https" {
		t.Fatal("production origin changed")
	}
}
