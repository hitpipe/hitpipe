package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

var errNoBrowserPayment = errors.New("the operation has no browser payment to replay")
var errPendingIngress = errors.New("previous Stripe event was not acknowledged; replay that event before sending a different one")

func eventMarker(raw []byte) []byte {
	return []byte(fmt.Sprintf("sha256:%x\n", sha256.Sum256(raw)))
}

func matchesPendingIngress(pending, marker []byte, replayLegacyPending bool) bool {
	return bytes.Equal(pending, marker) || replayLegacyPending && string(pending) == "awaiting Intake acknowledgement\n"
}

type browserEventJob struct {
	OperationID string `json:"operation_id"`
	Kind        string `json:"kind"`
}

func (s *backend) browserJobPath() string {
	return filepath.Join(s.state, "runtime", "browser-event.json")
}

func (s *backend) browserJob() (browserEventJob, error) {
	var job browserEventJob
	data, err := os.ReadFile(s.browserJobPath())
	if err != nil {
		return job, err
	}
	if json.Unmarshal(data, &job) != nil || !drafts.UUID(job.OperationID) || (job.Kind != "paid" && job.Kind != "replay") {
		return browserEventJob{}, errors.New("invalid school browser event job")
	}
	return job, nil
}

func (s *backend) enqueueBrowserEvent(job browserEventJob) error {
	current, err := s.browserJob()
	if err == nil {
		if current.OperationID == job.OperationID {
			return nil
		}
		return errPendingIngress
	}
	if !os.IsNotExist(err) {
		return err
	}
	s.provider.mu.Lock()
	saved := append([]byte(nil), s.provider.state.BrowserPayments[job.OperationID]...)
	s.provider.mu.Unlock()
	if job.Kind == "replay" && len(saved) == 0 {
		return errNoBrowserPayment
	}
	if pending, err := os.ReadFile(s.possibleIngress()); err == nil {
		if len(saved) == 0 || !matchesPendingIngress(pending, eventMarker(saved), false) {
			return errPendingIngress
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := s.revokeQueueProof(); err != nil {
		return err
	}
	return writeJSON(s.browserJobPath(), job)
}

// This entry belongs to the 0600 Unix socket. The BFF authenticates the owner
// and supplies the server-bound workspace. Admission persists a job before 202.
// Delivery uses the backend lifecycle, independent of the browser request.
func (s *backend) browserStripeEvent(w http.ResponseWriter, r *http.Request) {
	var kind string
	switch r.URL.Path {
	case "/browser-events/paid":
		kind = "paid"
	case "/browser-events/replay":
		kind = "replay"
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.URL.RawQuery != "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var input struct {
		WorkspaceID string `json:"workspace_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256))
	dec.DisallowUnknownFields()
	if dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if input.WorkspaceID != s.workspaceID {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !s.queueMu.TryLock() {
		respond(w, http.StatusConflict, map[string]string{"code": "SCHOOL_EVENT_PENDING"})
		return
	}
	defer s.queueMu.Unlock()
	current, err := s.status(r.Context())
	if err == nil && (current.OperationID == "" || current.OperationState != "ACTIVE") {
		respond(w, http.StatusConflict, map[string]string{"code": "SCHOOL_OPERATION_NOT_ACTIVE"})
		return
	}
	if err == nil {
		err = s.enqueueBrowserEvent(browserEventJob{OperationID: current.OperationID, Kind: kind})
	}
	switch {
	case errors.Is(err, errNoBrowserPayment):
		respond(w, http.StatusConflict, map[string]string{"code": "SCHOOL_PAYMENT_REQUIRED"})
	case errors.Is(err, errPendingIngress):
		respond(w, http.StatusConflict, map[string]string{"code": "SCHOOL_EVENT_PENDING"})
	case err != nil:
		log.Printf("school browser Stripe event unavailable: %v", err)
		respond(w, http.StatusServiceUnavailable, map[string]string{"code": "SCHOOL_PAYMENT_UNAVAILABLE"})
	default:
		respond(w, http.StatusAccepted, map[string]string{"state": "SUBMITTED", "kind": kind})
	}
}

func (s *backend) workBrowserEvents(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastLog time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.queueMu.TryLock() {
				continue
			}
			err := s.deliverQueuedBrowserEvent(ctx)
			s.queueMu.Unlock()
			if err != nil && ctx.Err() == nil && time.Since(lastLog) >= 10*time.Second {
				log.Printf("school browser Stripe event still pending: %v", err)
				lastLog = time.Now()
			}
		}
	}
}

func (s *backend) deliverQueuedBrowserEvent(ctx context.Context) error {
	job, err := s.browserJob()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	current, err := s.status(ctx)
	if err != nil {
		return err
	}
	if current.OperationID != job.OperationID || current.OperationState == "ARCHIVED" {
		return errors.New("browser event operation changed while queued")
	}
	if job.Kind == "paid" {
		if err := waitForCheckoutTime(ctx, current.activation); err != nil {
			return err
		}
	}
	raw, err := s.provider.preparePayment(job.Kind, job.OperationID, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := s.deliverEvent(ctx, raw, false); err != nil {
		return err
	}
	return os.Remove(s.browserJobPath())
}
