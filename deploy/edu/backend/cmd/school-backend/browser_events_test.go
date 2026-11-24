package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBrowserPaymentIsStableAcrossConcurrentClicksAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	p, err := loadProviders(path, &identity{})
	if err != nil {
		t.Fatal(err)
	}
	const operation = "sample-operation"
	now := time.Now().UTC()
	if _, err := p.preparePayment("replay", operation, now); !errors.Is(err, errNoBrowserPayment) {
		t.Fatalf("replay without a payment must fail: %v", err)
	}
	type result struct {
		body []byte
		err  error
	}
	results := make(chan result, 2)
	var callers sync.WaitGroup
	for i := range 2 {
		callers.Go(func() {
			body, err := p.preparePayment("paid", operation, now.Add(time.Duration(i)*time.Second))
			results <- result{body, err}
		})
	}
	callers.Wait()
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || !bytes.Equal(first.body, second.body) || len(p.state.Checkouts) != 1 {
		t.Fatal("the same browser operation must persist one event")
	}
	legacy, err := p.prepareEvent("unpaid", now)
	if err != nil {
		t.Fatal(err)
	}
	p, err = loadProviders(path, &identity{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"paid", "replay"} {
		replay, err := p.preparePayment(kind, operation, now.Add(time.Hour))
		if err != nil || !bytes.Equal(replay, first.body) {
			t.Fatalf("%s changed the persisted browser event", kind)
		}
	}
	if replay, err := p.prepareEvent("replay", now); err != nil || !bytes.Equal(replay, legacy) {
		t.Fatal("browser payments must not replace the retained operator event")
	}
	if len(p.state.Checkouts) != 2 {
		t.Fatal("replaying requests must not allocate another checkout")
	}
}

func TestPendingIngressOnlyAcceptsSameBrowserEvent(t *testing.T) {
	first := eventMarker([]byte(`{"id":"first"}`))
	other := eventMarker([]byte(`{"id":"other"}`))
	if !matchesPendingIngress(first, first, false) || matchesPendingIngress(first, other, false) ||
		matchesPendingIngress(first, other, true) || matchesPendingIngress([]byte("awaiting Intake acknowledgement\n"), other, false) {
		t.Fatal("an unconfirmed event must not be replaced or recovered by a different browser event")
	}
}

func TestBrowserEventsRejectForeignWorkspaceBeforeBackendLookup(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/browser-events/paid", strings.NewReader(`{"workspace_id":"other"}`))
	reply := httptest.NewRecorder()
	(&backend{}).browserStripeEvent(reply, request)
	if reply.Code != http.StatusForbidden {
		t.Fatalf("wanted forbidden for foreign workspace, got %d", reply.Code)
	}
}
