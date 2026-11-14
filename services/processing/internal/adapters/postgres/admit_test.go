package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"context"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/jackc/pgx/v5"
)

func wire(t *testing.T, ws, created, event string) []byte {
	t.Helper()
	id, _ := newID()
	// Signed Event.data.object.created is represented by created. Event.created
	// is deliberately absent: it must never substitute Session.created.
	m := map[string]any{"message_id": id, "workspace_id": ws, "connection_id": config, "receipt_id": id, "schema_version": "1.0", "stripe_account_id": "acct_synthetic", "livemode": false, "session_id": "cs_synthetic", "event_id": "evt_" + id, "event_type": event, "source_created_at": created, "first_received_at": "2026-11-14T02:00:00Z", "expires_at": "2026-11-15T00:00:00Z"}
	b, _ := json.Marshal(m)
	b, e := jcs.Transform(b)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	m["message_digest"] = "sha256:" + hex.EncodeToString(sum[:])
	b, _ = json.Marshal(m)
	return b
}
func established(ws string) admission.EstablishedContext {
	return admission.EstablishedContext{WorkspaceID: ws, ConnectionID: config, AccountID: "acct_synthetic", OperationID: op, AuthorityVerified: true, ExpectedRevision: 1}
}
func TestAdmissionActivationBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, created, event string
		ok                   bool
	}{
		{"before", "2026-11-13T23:59:59Z", "checkout.session.completed", false},
		{"exact", "2026-11-14T00:00:00Z", "checkout.session.completed", true},
		{"after", "2026-11-14T00:00:01Z", "checkout.session.completed", true},
		{"old_session_late_payment", "2026-11-13T23:59:59Z", "checkout.session.async_payment_succeeded", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := workspace(t)
			raw := wire(t, ws, tc.created, tc.event)
			r, e := admission.Admit(ctx, raw, established(ws), Store{connect(t)})
			if tc.ok {
				if e != nil || r.Disposition != "APPLIED" {
					t.Fatalf("%v %v", r, e)
				}
				counts(t, ws, 1, 1, 1)
			} else {
				if e == nil || r != (admission.Receipt{}) {
					t.Fatal("old Session admitted")
				}
				counts(t, ws, 0, 0, 0)
			}
		})
	}
}
func TestAdmissionRejectsContextAndLocalState(t *testing.T) {
	for _, mode := range []string{"authority", "binding", "connection", "account", "livemode", "workspace", "missing_operation", "paused", "draft", "missing_config", "missing_activation", "stale_revision", "invalid_wire", "missing_time", "invalid_time"} {
		t.Run(mode, func(t *testing.T) {
			ws := workspace(t)
			raw := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
			c := established(ws)
			db := connect(t)
			switch mode {
			case "authority":
				c.AuthorityVerified = false
			case "binding":
				c.ConnectionID = ""
			case "connection":
				c.ConnectionID = op
			case "account":
				c.AccountID = "other"
			case "livemode":
				c.Live = true
			case "workspace":
				c.WorkspaceID = owner
			case "missing_operation":
				c.OperationID = owner
			case "stale_revision":
				c.ExpectedRevision = 2
			case "invalid_wire":
				raw = append(raw, []byte(` {}`)...)
			case "missing_time", "invalid_time":
				var m map[string]any
				json.Unmarshal(raw, &m)
				if mode == "missing_time" {
					delete(m, "source_created_at")
				} else {
					m["source_created_at"] = "invalid"
				}
				raw, _ = json.Marshal(m)
			default:
				tx := begin(t, db, ws)
				q := map[string]string{"paused": "state='PAUSED'", "draft": "state='DRAFT'", "missing_config": "selected_config_id=NULL", "missing_activation": "first_activation_at=NULL"}[mode]
				// Immutable activation cannot be reset. Create the missing-boundary fixture
				// using a new operation rather than bypassing its trigger.
				if mode == "missing_activation" {
					_, e := tx.Exec(ctx, `UPDATE hp_processing.operations SET state='PAUSED' WHERE workspace_id=$1`, ws)
					if e != nil {
						t.Fatal(e)
					}
					_, e = tx.Exec(ctx, `INSERT INTO hp_processing.operations(workspace_id,operation_id,owner_user_id,revision,state) VALUES($1,$2,$3,1,'ACTIVE')`, ws, owner, owner)
					if e != nil {
						t.Fatal(e)
					}

					_, e = tx.Exec(ctx, `INSERT INTO hp_processing.accepted_configs VALUES($1,$2,$2,'1.0','synthetic','{"pack_id":"synthetic"}','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',$2,now(),'synthetic')`, ws, owner)
					if e != nil {
						t.Fatal(e)
					}
					_, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id=$2 WHERE workspace_id=$1 AND operation_id=$2`, ws, owner)
					if e != nil {
						t.Fatal(e)
					}
					c.OperationID = owner
				} else {
					if _, e := tx.Exec(ctx, `UPDATE hp_processing.operations SET `+q+` WHERE workspace_id=$1`, ws); e != nil {
						t.Fatal(e)
					}
				}
				if e := tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
			}
			r, e := admission.Admit(ctx, raw, c, Store{db})
			if e == nil || r != (admission.Receipt{}) {
				t.Fatalf("rejected input acknowledged: %v %v", r, e)
			}
			counts(t, ws, 0, 0, 0)
		})
	}
}
func TestAdmissionReplayAfterLifecycleAndNewConnection(t *testing.T) {
	ws := workspace(t)
	raw := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
	c := established(ws)
	first, e := admission.Admit(ctx, raw, c, Store{connect(t)})
	if e != nil {
		t.Fatal(e)
	}
	db := connect(t)
	tx := begin(t, db, ws)
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET state='PAUSED',revision=2 WHERE workspace_id=$1`, ws); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	again, e := admission.Admit(ctx, raw, c, Store{connect(t)})
	if e != nil || first != again {
		t.Fatalf("replay: %v %v", again, e)
	}
	counts(t, ws, 1, 1, 1)
}

// Real PostgreSQL deferred constraint failure in Commit, after all three writes.
// This wraps only the transaction to inject a real commit failure, never a fake DB.
type badCommitDB struct {
	c  *pgx.Conn
	ws string
}

func (b badCommitDB) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := b.c.BeginTx(ctx, o)
	return badCommitTx{tx, b.ws}, e
}

type badCommitTx struct {
	pgx.Tx
	ws string
}

func (b badCommitTx) Commit(ctx context.Context) error {
	_, e := b.Tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id=$2 WHERE workspace_id=$1`, b.ws, owner)
	if e != nil {
		return e
	}
	return b.Tx.Commit(ctx)
}
func TestAdmissionCommitFailureNeverAcknowledges(t *testing.T) {
	ws := workspace(t)
	raw := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
	r, e := admission.Admit(ctx, raw, established(ws), Store{badCommitDB{connect(t), ws}})
	if e == nil || r != (admission.Receipt{}) {
		t.Fatal("failed commit acknowledged")
	}
	counts(t, ws, 0, 0, 0)
}
func TestAdmissionTwoSignalsOnePinnedCase(t *testing.T) {
	ws := workspace(t)
	c := established(ws)
	raw := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
	first, e := admission.Admit(ctx, raw, c, Store{connect(t)})
	if e != nil {
		t.Fatal(e)
	}
	db := connect(t)
	tx := begin(t, db, ws)
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET owner_user_id=$2,revision=2 WHERE workspace_id=$1`, ws, op); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	c.ExpectedRevision = 2
	second, e := admission.Admit(ctx, wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.async_payment_succeeded"), c, Store{connect(t)})
	if e != nil || second.RecordID != first.RecordID {
		t.Fatalf("identity split: %v", e)
	}
	tx = begin(t, db, ws)
	var actualOwner string
	var revision int
	if e = tx.QueryRow(ctx, `SELECT owner_user_id::text,revision FROM hp_processing.case_identity`).Scan(&actualOwner, &revision); e != nil {
		t.Fatal(e)
	}
	if actualOwner != owner || revision != 1 {
		t.Fatal("case pin or revision overwritten")
	}
	tx.Rollback(ctx)
	counts(t, ws, 2, 1, 1)
}

func TestAdmissionConcurrentAndDigestConflict(t *testing.T) {
	ws := workspace(t)
	c := established(ws)
	raw1 := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
	raw2 := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.async_payment_succeeded")
	db1, db2 := connect(t), connect(t)
	var wg sync.WaitGroup
	results := make(chan admission.Receipt, 2)
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i, raw := range [][]byte{raw1, raw2} {
		wg.Add(1)
		go func(raw []byte, db *pgx.Conn) {
			defer wg.Done()
			<-start
			r, e := admission.Admit(ctx, raw, c, Store{db})
			results <- r
			errs <- e
		}(raw, []*pgx.Conn{db1, db2}[i])
	}
	close(start)
	wg.Wait()
	for range 2 {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	a, b := <-results, <-results
	if a.RecordID != b.RecordID {
		t.Fatal("concurrent identities split")
	}
	counts(t, ws, 2, 1, 1)
	var m map[string]any
	json.Unmarshal(raw1, &m)
	delete(m, "message_digest")
	m["event_id"] = "evt_conflicting"
	payload, _ := json.Marshal(m)
	canonical, e := jcs.Transform(payload)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(canonical)
	m["message_digest"] = "sha256:" + hex.EncodeToString(sum[:])
	conflict, _ := json.Marshal(m)
	r, e := admission.Admit(ctx, conflict, c, Store{connect(t)})
	if !errors.Is(e, admission.ErrConflict) || r != (admission.Receipt{}) {
		t.Fatalf("conflict acknowledged %v %v", r, e)
	}
	counts(t, ws, 2, 1, 1)
}
