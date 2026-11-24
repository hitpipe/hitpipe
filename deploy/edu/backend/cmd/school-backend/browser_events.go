package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var errNoBrowserPayment = errors.New("the operation has no browser payment to replay")
var errPendingIngress = errors.New("previous Stripe event was not acknowledged; replay that event before sending a different one")

func eventMarker(raw []byte) []byte {
	return []byte(fmt.Sprintf("sha256:%x\n", sha256.Sum256(raw)))
}

func matchesPendingIngress(pending, marker []byte, replayLegacyPending bool) bool {
	return bytes.Equal(pending, marker) || replayLegacyPending && string(pending) == "awaiting Intake acknowledgement\n"
}

// This entry belongs to the 0600 Unix socket. The BFF authenticates the owner
// and supplies the server-bound workspace. The producer verifies the running
// operation and persists one stable paid event per operation.
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
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	ctx := r.Context()
	current, err := s.status(ctx)
	if err == nil && (current.OperationID == "" || current.OperationState != "ACTIVE") {
		respond(w, http.StatusConflict, map[string]string{"code": "SCHOOL_OPERATION_NOT_ACTIVE"})
		return
	}
	if err == nil && kind == "paid" {
		err = waitForCheckoutTime(ctx, current.activation)
	}
	if err == nil {
		var raw []byte
		raw, err = s.provider.preparePayment(kind, current.OperationID, time.Now().UTC())
		if err == nil {
			err = s.revokeQueueProof()
		}
		if err == nil {
			err = s.deliverEvent(ctx, raw, false)
		}
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
