package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type checkout struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
	Paid    bool   `json:"paid"`
}

type clickUpTask struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type providerState struct {
	Checkouts       map[string]checkout    `json:"checkouts"`
	Tasks           map[string]clickUpTask `json:"tasks"`
	LastCheckout    string                 `json:"last_checkout"`
	LastUnpaid      string                 `json:"last_unpaid"`
	LastEvent       json.RawMessage        `json:"last_event"`
	BrowserPayments map[string][]byte      `json:"browser_payments,omitempty"`
	Posts           int                    `json:"posts"`
	Reads           int                    `json:"reads"`
}

type providers struct {
	path  string
	mu    sync.Mutex
	state providerState
	keys  *identity
}

func loadProviders(path string, keys *identity) (*providers, error) {
	p := &providers{path: path, keys: keys, state: providerState{Checkouts: map[string]checkout{}, Tasks: map[string]clickUpTask{}}}
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &p.state)
		if err == nil && (p.state.Checkouts == nil || p.state.Tasks == nil) {
			err = errors.New("invalid school provider state")
		}
		if err == nil && len(p.state.LastEvent) != 0 {
			var compact bytes.Buffer
			err = json.Compact(&compact, p.state.LastEvent)
			p.state.LastEvent = compact.Bytes()
		}
	} else if os.IsNotExist(err) {
		err = writeJSON(path, p.state)
	}
	return p, err
}

func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func (p *providers) stripe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !constantEqual(r.Header.Get("Authorization"), "Bearer "+p.keys.StripeKey) ||
		r.Header.Get("Stripe-Account") != stripeAccount || r.Header.Get("Stripe-Version") != stripeVersion {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Stripe-Version", stripeVersion)
	obj := p.stripeObject(r.URL.Path)
	if obj == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	respond(w, http.StatusOK, obj)
}

func stripeList(items ...map[string]any) map[string]any {
	all := make([]map[string]any, len(items))
	copy(all, items)
	return map[string]any{"object": "list", "data": all, "has_more": false}
}

func (p *providers) stripeObject(path string) map[string]any {
	switch path {
	case "/v1/account":
		return map[string]any{"id": stripeAccount, "object": "account"}
	case "/v1/prices/price_fixed":
		return map[string]any{"id": "price_fixed", "object": "price", "livemode": false, "currency": "usd", "type": "one_time", "billing_scheme": "per_unit", "custom_unit_amount": nil, "recurring": nil, "transform_quantity": nil, "unit_amount": int64(1200)}
	case "/v1/refunds", "/v1/disputes":
		return stripeList()
	}
	for id, item := range p.state.Checkouts {
		suffix := strings.TrimPrefix(id, "cs_")
		pi, ch := "pi_"+suffix, "ch_"+suffix
		paid, status, amount := "unpaid", "processing", int64(0)
		if item.Paid {
			paid, status, amount = "paid", "succeeded", int64(1200)
		}
		switch path {
		case "/v1/checkout/sessions/" + id:
			return map[string]any{"id": id, "object": "checkout.session", "livemode": false, "mode": "payment", "payment_status": paid, "currency": "usd", "amount_total": int64(1200), "created": item.Created, "payment_intent": pi}
		case "/v1/checkout/sessions/" + id + "/line_items":
			return stripeList(map[string]any{"id": "li_" + suffix, "object": "item", "currency": "usd", "quantity": int64(1), "price": map[string]any{"id": "price_fixed", "object": "price"}})
		case "/v1/payment_intents/" + pi:
			return map[string]any{"id": pi, "object": "payment_intent", "livemode": false, "status": status, "currency": "usd", "amount_received": amount, "latest_charge": ch}
		case "/v1/charges/" + ch:
			chargeStatus := "pending"
			if item.Paid {
				chargeStatus = "succeeded"
			}
			return map[string]any{"id": ch, "object": "charge", "livemode": false, "currency": "usd", "payment_intent": pi, "paid": item.Paid, "status": chargeStatus, "captured": item.Paid, "amount_captured": amount, "amount_refunded": int64(0), "disputed": false}
		}
	}
	return nil
}

func (p *providers) clickup(w http.ResponseWriter, r *http.Request) {
	if !constantEqual(r.Header.Get("Authorization"), clickUpToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v2/list/"+clickUpList+"/task":
		var body struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Assignees   []int64 `json:"assignees"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body) != nil ||
			body.Name == "" || body.Description == "" || len(body.Assignees) != 1 || body.Assignees[0] != 123402 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(p.state.Tasks) >= 100 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		id := "86" + strings.ReplaceAll(mustUUID(), "-", "")[:15]
		p.state.Tasks[id] = clickUpTask{ID: id, Name: body.Name, Description: body.Description}
		p.state.Posts++
		if err := writeJSON(p.path, p.state); err != nil {
			delete(p.state.Tasks, id)
			p.state.Posts--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		respond(w, http.StatusOK, map[string]string{"id": id})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/task/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v2/task/")
		task, ok := p.state.Tasks[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		p.state.Reads++
		if err := writeJSON(p.path, p.state); err != nil {
			p.state.Reads--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		respond(w, http.StatusOK, map[string]any{"id": id, "team_id": clickUpWorkspace, "list": map[string]string{"id": clickUpList},
			"assignees": []map[string]int64{{"id": 123402}}, "name": task.Name, "description": task.Description})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (p *providers) prepareEvent(kind string, now time.Time) ([]byte, error) {
	return p.preparePayment(kind, "", now)
}

func (p *providers) preparePayment(kind, operation string, now time.Time) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if operation != "" {
		if saved := p.state.BrowserPayments[operation]; len(saved) != 0 {
			return append([]byte(nil), saved...), nil
		}
		if kind == "replay" {
			return nil, errNoBrowserPayment
		}
		if kind != "paid" {
			return nil, errors.New("unsupported school browser payment")
		}
	}
	if kind == "replay" {
		if len(p.state.LastEvent) == 0 {
			return nil, errors.New("there is no signed event to replay")
		}
		return append([]byte(nil), p.state.LastEvent...), nil
	}
	last, lastUnpaid, lastEvent := p.state.LastCheckout, p.state.LastUnpaid, p.state.LastEvent
	var item, previous checkout
	eventType := "checkout.session.completed"
	switch kind {
	case "paid", "unpaid":
		if len(p.state.Checkouts) >= 100 {
			return nil, errors.New("school test checkout limit reached")
		}
		item = checkout{ID: "cs_school_" + strings.ReplaceAll(mustUUID(), "-", ""), Created: now.Unix(), Paid: kind == "paid"}
		p.state.LastCheckout = item.ID
		if !item.Paid {
			p.state.LastUnpaid = item.ID
		}
	case "async-paid":
		if p.state.LastUnpaid == "" {
			return nil, errors.New("send an unpaid test checkout first")
		}
		item = p.state.Checkouts[p.state.LastUnpaid]
		previous = item
		item.Paid = true
		p.state.LastUnpaid = ""
		p.state.LastCheckout = item.ID
		eventType = "checkout.session.async_payment_succeeded"
	default:
		return nil, errors.New("unknown school event type")
	}
	raw, err := json.Marshal(map[string]any{"id": "evt_" + strings.ReplaceAll(mustUUID(), "-", ""), "object": "event", "account": stripeAccount,
		"api_version": stripeVersion, "type": eventType, "created": now.Unix(), "livemode": false, "data": map[string]any{"object": map[string]any{"id": item.ID, "object": "checkout.session", "created": item.Created, "livemode": false}}})
	if err != nil {
		return nil, err
	}
	p.state.Checkouts[item.ID] = item
	if operation == "" {
		p.state.LastEvent = raw
	} else {
		p.state.LastCheckout = last
		if p.state.BrowserPayments == nil {
			p.state.BrowserPayments = make(map[string][]byte)
		}
		p.state.BrowserPayments[operation] = raw
	}
	if err := writeJSON(p.path, p.state); err != nil {
		delete(p.state.BrowserPayments, operation)
		if previous.ID == "" {
			delete(p.state.Checkouts, item.ID)
		} else {
			p.state.Checkouts[item.ID] = previous
		}
		p.state.LastCheckout, p.state.LastUnpaid, p.state.LastEvent = last, lastUnpaid, lastEvent
		return nil, err
	}
	return raw, nil
}

type proof struct {
	OperationID          string `json:"operationId"`
	AcceptedConfigDigest string `json:"acceptedConfigDigest"`
	CaseID               string `json:"caseId"`
	CaseState            string `json:"caseState"`
	TaskID               string `json:"taskId"`
	ProviderTitle        string `json:"providerTitle"`
	ProviderCreateCount  int    `json:"providerCreateCount"`
	ProviderReadCount    int    `json:"providerReadCount"`
	CaseCount            int    `json:"caseCount"`
	OperationState       string `json:"operationState"`
	activation           time.Time
}

func (s *backend) status(ctx context.Context) (proof, error) {
	var status proof
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	db, err := pgx.Connect(ctx, processingApp)
	if err != nil {
		return status, err
	}
	defer db.Close(context.Background())
	tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return status, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, s.workspaceID); err != nil {
		return status, err
	}
	err = tx.QueryRow(ctx, `SELECT o.operation_id::text,o.state,o.first_activation_at,a.digest
		FROM hp_processing.operations o
		JOIN hp_processing.accepted_configs a
		  ON (a.workspace_id,a.operation_id,a.config_id)=(o.workspace_id,o.operation_id,o.selected_config_id)
		WHERE o.workspace_id=$1 ORDER BY o.first_activation_at DESC LIMIT 1`, s.workspaceID).
		Scan(&status.OperationID, &status.OperationState, &status.activation, &status.AcceptedConfigDigest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return status, err
	}
	if status.OperationID != "" {
		err = tx.QueryRow(ctx, `SELECT case_id::text,state,coalesce(known_task_id,'')
		FROM hp_processing.case_identity WHERE workspace_id=$1 AND operation_id=$2 ORDER BY admitted_at DESC LIMIT 1`, s.workspaceID, status.OperationID).
			Scan(&status.CaseID, &status.CaseState, &status.TaskID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return status, err
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.case_identity WHERE workspace_id=$1 AND operation_id=$2`, s.workspaceID, status.OperationID).Scan(&status.CaseCount); err != nil {
			return status, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return status, err
	}
	s.provider.mu.Lock()
	status.ProviderTitle = s.provider.state.Tasks[status.TaskID].Name
	status.ProviderCreateCount = s.provider.state.Posts
	status.ProviderReadCount = s.provider.state.Reads
	s.provider.mu.Unlock()
	return status, nil
}

func (s *backend) emit(ctx context.Context, kind string) error {
	if _, err := os.Lstat(s.possibleIngress()); !os.IsNotExist(err) && kind != "replay" {
		return errPendingIngress
	}
	status, err := s.status(ctx)
	if err != nil {
		return err
	}
	if status.OperationID == "" {
		return errors.New("activate an operation before sending a Stripe event")
	}
	if kind != "replay" {
		if err := waitForCheckoutTime(ctx, status.activation); err != nil {
			return err
		}
	}
	raw, err := s.provider.prepareEvent(kind, time.Now().UTC())
	if err != nil {
		return err
	}
	return s.deliverEvent(ctx, raw, kind == "replay")
}

func waitForCheckoutTime(ctx context.Context, activation time.Time) error {
	wait := time.Until(activation.Truncate(time.Second).Add(time.Second))
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *backend) deliverEvent(ctx context.Context, raw []byte, replayLegacyPending bool) error {
	marker := eventMarker(raw)
	if pending, err := os.ReadFile(s.possibleIngress()); err == nil {
		if !matchesPendingIngress(pending, marker, replayLegacyPending) {
			return errPendingIngress
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	now := time.Now().UTC()
	mac := hmac.New(sha256.New, []byte(s.keys.Signing))
	_, _ = fmt.Fprintf(mac, "%d.", now.Unix())
	_, _ = mac.Write(raw)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+intakeAddress+"/webhooks/stripe/synthetic", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%x", now.Unix(), mac.Sum(nil)))
	req.Header.Set("Content-Type", "application/json")
	// The event bytes are already durable. If the network outcome is unknown,
	// preserve the queue fence and allow re-sending exactly those bytes.
	if err := writePrivate(s.possibleIngress(), marker); err != nil {
		return err
	}
	res, err := s.certs.client("", "intake.test").Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Intake webhook returned HTTP %d", res.StatusCode)
	}
	return os.Remove(s.possibleIngress())
}

func (s *backend) serveControl(ctx context.Context) error {
	path := filepath.Join(s.share, "provider.sock")
	if err := resetSocket(path); err != nil {
		return err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
		l.Close()
		return err
	}
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 4 * time.Second, WriteTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/browser-events/"):
			s.browserStripeEvent(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/status":
			status, err := s.status(r.Context())
			if err != nil {
				log.Printf("status unavailable: %v", err)
				respond(w, http.StatusServiceUnavailable, map[string]string{"error": "status unavailable"})
				return
			}
			respond(w, http.StatusOK, status)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/events/"):
			kind := strings.TrimPrefix(r.URL.Path, "/events/")
			switch kind {
			case "paid", "unpaid", "async-paid", "replay":
			default:
				respond(w, http.StatusNotFound, map[string]string{"error": "unsupported test event"})
				return
			}
			s.queueMu.Lock()
			defer s.queueMu.Unlock()
			if err := s.revokeQueueProof(); err != nil {
				respond(w, http.StatusConflict, map[string]string{"error": "queue snapshot is being consumed"})
				return
			}
			if err := s.emit(r.Context(), kind); err != nil {
				respond(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			respond(w, http.StatusAccepted, map[string]string{"event": kind, "state": "INGESTED"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})}
	go func() {
		if err := server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("private controls stopped: %v", err)
		}
	}()
	context.AfterFunc(ctx, func() { server.Close() })
	publish := func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		var last proof
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				status, err := s.status(ctx)
				s.refreshQueueWhenPaused(ctx, status, err)
				if err != nil || status == last || status.CaseState != "CONFIRMED" {
					continue
				}
				if err := writeJSON(filepath.Join(s.share, "result.json"), status); err != nil {
					log.Printf("publish status: %v", err)
				} else {
					last = status
				}
			}
		}
	}
	go publish()
	return nil
}
