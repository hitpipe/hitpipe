package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"strings"
	"testing"
	"time"
)

func (f activationFixture) checkPreview(t *testing.T) drafts.ReadinessCheck {
	t.Helper()
	rev := int64(1)
	status, body := f.call(t, drafts.Check, f.meta(&rev), nil)
	var c drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(body, &c) != nil || !c.Passed || c.ActivationPreview == nil {
		t.Fatal("public readiness preview", status, string(body))
	}
	raw, _ := json.Marshal(c.ActivationPreview.CanonicalConfig)
	raw, e := jcs.Transform(raw)
	if e != nil || draftjson.Hash(raw) != c.ActivationPreview.AcceptedConfigDigest || c.ConfigDigest == c.ActivationPreview.AcceptedConfigDigest {
		t.Fatal("public hashes are not exact distinct representations", e)
	}
	draftCapture(t, drafts.Check, status, nil, body, "approved_final_snapshot_visible_before_acceptance")
	return c
}
func (f activationFixture) rehearseCheck(t *testing.T, c drafts.ReadinessCheck) (drafts.RehearsalRequest, drafts.RehearsalResult) {
	t.Helper()
	rev := int64(1)
	req := drafts.RehearsalRequest{Command: f.meta(&rev), DraftID: f.id, DraftRevision: 1, ReadinessCheckID: c.CheckID, Examples: []drafts.ScenarioExample{{SyntheticSessionAlias: "example_paid", Paid: true, AmountMinor: 100, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_zero", Paid: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_unpaid", AmountMinor: 100, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_duplicate", Paid: true, AmountMinor: 100, Duplicate: true, MissingContextKeys: []string{}}}}
	f.bindings.mu.Lock()
	calls := f.bindings.calls
	f.bindings.mu.Unlock()
	s, b := f.call(t, drafts.Rehearse, req, nil)
	var rr drafts.RehearsalResult
	if s != 200 || json.Unmarshal(b, &rr) != nil || rr.ConfigDigest != c.ConfigDigest || rr.LiveRequestsMade != 0 || strings.Join(rr.Results, ",") != "WOULD_CONFIRM,WOULD_EXCLUDE,WOULD_WAIT,WOULD_HOLD_UNKNOWN" {
		t.Fatal("selected snapshot rehearsal", s, string(b))
	}
	f.bindings.mu.Lock()
	after := f.bindings.calls
	f.bindings.mu.Unlock()
	if after != calls {
		t.Fatal("rehearsal performed live owner lookup")
	}
	draftCapture(t, drafts.Rehearse, s, req, b, "explicit_selected_readiness_zero_live_requests")
	return req, rr
}
func (f activationFixture) acceptPreview(c drafts.ReadinessCheck, rr drafts.RehearsalResult) drafts.ActivationCommand {
	return drafts.ActivationCommand{Command: f.meta(nil), DraftID: f.id, DraftRevision: 1, ReadinessCheckID: c.CheckID, RehearsalID: rr.RehearsalID, AcceptedConfigDigest: c.ActivationPreview.AcceptedConfigDigest, AcceptanceTextVersion: "managed-operation-v1", ConfirmActivate: true}
}
func TestActivationCheckARehearsalACannotAuthorizeCheckB(t *testing.T) {
	for _, change := range []string{"price", "list", "assignee"} {
		t.Run(change, func(t *testing.T) {
			f := newActivationFixture(t)
			f.save(t)
			a := f.checkPreview(t)
			_, ra := f.rehearseCheck(t, a)
			f.assertCounts(t, 0, 3)
			f.bindings.mu.Lock()
			switch change {
			case "price":
				f.bindings.priceID = "price_changed"
			case "list":
				f.bindings.listID = "list_changed"
			case "assignee":
				f.bindings.assigneeID = "assignee_changed"
			}
			f.bindings.mu.Unlock()
			b := f.checkPreview(t)
			if a.ConfigDigest != b.ConfigDigest || a.ActivationPreview.AcceptedConfigDigest == b.ActivationPreview.AcceptedConfigDigest {
				t.Fatal("unchanged draft vs changed final binding not distinguished")
			}
			f.assertCounts(t, 0, 4)
			for _, c := range []drafts.ReadinessCheck{a, b} {
				in := f.acceptPreview(c, ra)
				s, out := f.activate(t, in, nil)
				if s != 409 {
					t.Fatal("stale/mixed acceptance authorized", change, s, string(out))
				}
				activationCapture(t, s, in, out, "check_pair_or_owner_changed_"+change)
			}
			f.assertCounts(t, 0, 4)
			_, rb := f.rehearseCheck(t, b)
			in := f.acceptPreview(b, rb)
			s, out := f.activate(t, in, nil)
			if s != 201 {
				t.Fatal("fresh matching pair rejected", s, string(out))
			}
			var op drafts.Operation
			json.Unmarshal(out, &op)
			if op.AcceptedConfigDigest == nil || *op.AcceptedConfigDigest != b.ActivationPreview.AcceptedConfigDigest {
				t.Fatal("different final hash returned")
			}
			activationCapture(t, s, in, out, "fresh_check_rehearsal_and_new_explicit_acceptance_"+change)
			tx := begin(t, connect(t), f.workspace)
			var raw []byte
			var digest string
			if e := tx.QueryRow(ctx, `SELECT canonical_config,digest FROM hp_processing.accepted_configs WHERE workspace_id=$1`, f.workspace).Scan(&raw, &digest); e != nil {
				t.Fatal(e)
			}
			raw, e := jcs.Transform(raw)
			shown, _ := json.Marshal(b.ActivationPreview.CanonicalConfig)
			shown, _ = jcs.Transform(shown)
			if e != nil || !bytes.Equal(raw, shown) || digest != draftjson.Hash(raw) || digest != in.AcceptedConfigDigest {
				t.Fatal("SQL replaced the shown canonical bytes or digest", e)
			}
			tx.Rollback(ctx)
			f.assertCounts(t, 1, 6)
		})
	}
}

// Rewrite only this fixture's selected check and rebuild the existing private
// ref chain, so semantic validation is exercised independently of file hashes.
func (f activationFixture) rewriteCheck(t *testing.T, id string, change func(map[string]json.RawMessage, *drafts.CommandNode)) {
	t.Helper()
	tx := begin(t, connect(t), f.workspace)
	var rootID, digest string
	var expiry time.Time
	if e := tx.QueryRow(ctx, `SELECT prose_ref::text,prose_sha256,expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&rootID, &digest, &expiry); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	raw, e := f.files.ReadDraftRoot(ctx, rootID, digest, expiry, origin(f.workspace, f.id, 1), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var root drafts.Root
	json.Unmarshal(raw, &root)
	var rewrite func(*drafts.Ref) *drafts.Ref
	found := false
	rewrite = func(ref *drafts.Ref) *drafts.Ref {
		if ref == nil {
			return nil
		}
		raw, e := f.files.ReadDraft(ctx, *ref, origin(f.workspace, f.id, ref.Revision), time.Now())
		if e != nil {
			t.Fatal(e)
		}
		var node drafts.CommandNode
		json.Unmarshal(raw, &node)
		hit := false
		if node.Kind == drafts.Check {
			body, e := f.files.ReadDraft(ctx, node.Response, origin(f.workspace, f.id, node.Response.Revision), time.Now())
			if e != nil {
				t.Fatal(e)
			}
			var c drafts.ReadinessCheck
			json.Unmarshal(body, &c)
			if c.CheckID == id {
				var m map[string]json.RawMessage
				json.Unmarshal(body, &m)
				change(m, &node)
				raw, _ = json.Marshal(m)
				node.Response, e = f.repo.writeFile(ctx, raw, f.workspace, f.id, ref.Revision, expiry, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				hit = true
				found = true
			}
		}
		if !hit {
			node.Previous = rewrite(node.Previous)
		}
		raw, _ = json.Marshal(node)
		newRef, e := f.repo.writeFile(ctx, raw, f.workspace, f.id, ref.Revision, expiry, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		return &newRef
	}
	root.Tail = rewrite(root.Tail)
	if !found {
		t.Fatal("chosen check missing")
	}
	raw, _ = json.Marshal(root)
	newRoot, e := f.repo.writeFile(ctx, raw, f.workspace, f.id, 1, expiry, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	tx = begin(t, connect(t), f.workspace)
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.drafts SET prose_ref=$2,prose_sha256=$3 WHERE workspace_id=$1`, f.workspace, newRoot.ID, newRoot.Digest); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestActivationPreviewTamperLegacyAndExpiryFailClosed(t *testing.T) {
	for _, kind := range []string{"true_null", "false_object", "wrong_hash", "changed_config_with_correct_hash", "old_missing_preview", "expired", "wrong_draft_digest", "wrong_binding_proof"} {
		t.Run(kind, func(t *testing.T) {
			f := newActivationFixture(t)
			f.save(t)
			check := f.checkPreview(t)
			_, rr := f.rehearseCheck(t, check)
			in := f.acceptPreview(check, rr)
			f.rewriteCheck(t, check.CheckID, func(m map[string]json.RawMessage, n *drafts.CommandNode) {
				switch kind {
				case "true_null":
					m["activation_preview"] = json.RawMessage(`null`)
				case "false_object":
					m["passed"] = json.RawMessage(`false`)
				case "old_missing_preview":
					delete(m, "activation_preview")
					n.AcceptedConfigDigest = ""
				case "expired":
					m["expires_at"], _ = json.Marshal(time.Now().Add(-time.Second))
				case "wrong_draft_digest":
					m["config_digest"], _ = json.Marshal("sha256:" + strings.Repeat("0", 64))
				case "wrong_binding_proof":
					n.BindingCheck.PriceID = "price_not_accepted"
				default:
					var p drafts.ActivationPreview
					json.Unmarshal(m["activation_preview"], &p)
					if kind == "wrong_hash" {
						p.AcceptedConfigDigest = "sha256:" + strings.Repeat("0", 64)
					} else {
						p.CanonicalConfig.PriceID = "price_not_shown"
						raw, _ := json.Marshal(p.CanonicalConfig)
						raw, _ = jcs.Transform(raw)
						p.AcceptedConfigDigest = draftjson.Hash(raw)
						n.AcceptedConfigDigest = p.AcceptedConfigDigest
					}
					m["activation_preview"], _ = json.Marshal(p)
				}
			})
			s, b := f.activate(t, in, nil)
			want := 503
			if kind == "old_missing_preview" || kind == "expired" || kind == "wrong_draft_digest" {
				want = 409
			}
			if s != want {
				t.Fatal(kind, s, want, string(b))
			}
			f.assertCounts(t, 0, 3)
			activationCapture(t, s, in, b, "durable_preview_"+kind)
		})
	}
}

type expiryAtActivationCommit struct {
	inner   activation.Repository
	fixture activationFixture
	checkID string
	test    *testing.T
}

func (r expiryAtActivationCommit) Load(c context.Context, q activation.Request, in activation.Input, g drafts.Grant) (activation.Snapshot, error) {
	return r.inner.Load(c, q, in, g)
}
func (r expiryAtActivationCommit) Commit(c context.Context, q activation.Request, in activation.Input, g drafts.Grant, s activation.Snapshot, p drafts.BindingProof) ([]byte, error) {
	r.fixture.rewriteCheck(r.test, r.checkID, func(m map[string]json.RawMessage, _ *drafts.CommandNode) {
		m["expires_at"], _ = json.Marshal(time.Now().Add(-time.Second))
	})
	return r.inner.Commit(c, q, in, g, s, p)
}
func TestActivationCommitRechecksSelectedDurableExpiry(t *testing.T) {
	f := newActivationFixture(t)
	f.save(t)
	check := f.checkPreview(t)
	_, rr := f.rehearseCheck(t, check)
	in := f.acceptPreview(check, rr)
	f.act.Repository = expiryAtActivationCommit{inner: f.store, fixture: f, checkID: check.CheckID, test: t}
	f.startActivation(t)
	status, body := f.activate(t, in, nil)
	if status != 409 {
		t.Fatal("expired after load but before commit accepted", status, string(body))
	}
	f.assertCounts(t, 0, 3)
	activationCapture(t, status, in, body, "selected_check_expired_between_load_and_actual_commit")
	f.act.Repository = f.store
	f.startActivation(t)
	fresh := f.checkPreview(t)
	_, rehearsed := f.rehearseCheck(t, fresh)
	newConsent := f.acceptPreview(fresh, rehearsed)
	status, body = f.activate(t, newConsent, nil)
	if status != 201 {
		t.Fatal("fresh explicit acceptance after expiry", status, string(body))
	}
	f.assertCounts(t, 1, 6)
}
func TestActivationLegacyRehearsalWithoutPairIsNotUpgraded(t *testing.T) {
	f := newActivationFixture(t)
	in := f.ready(t)
	f.rewriteRehearsal(t, func(_ *drafts.RehearsalResult, n *drafts.CommandNode) {
		n.ReadinessCheckID = ""
		n.AcceptedConfigDigest = ""
	})
	s, b := f.activate(t, in, nil)
	if s != 409 {
		t.Fatal("unbound historical rehearsal upgraded", s, string(b))
	}
	f.assertCounts(t, 0, 3)
	activationCapture(t, s, in, b, "historical_rehearsal_missing_pair_not_backfilled")
}

func TestActivationCommittedReplayAfterReadinessExpiry(t *testing.T) {
	f := newActivationFixture(t)
	in := f.ready(t)
	s, original := f.activate(t, in, nil)
	if s != 201 {
		t.Fatal(s, string(original))
	}
	tx := begin(t, connect(t), f.workspace)
	var before time.Time
	if e := tx.QueryRow(ctx, `SELECT expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&before); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	later := testClock{func() time.Time { return time.Now().Add(2 * time.Minute) }}
	f.store.Drafts.Clock = later
	f.act.Clock = later
	f.act.Repository = f.store
	f.bindings.unavailable = true
	f.startActivation(t)
	s, b := f.activate(t, in, nil)
	if s != 201 || !bytes.Equal(b, original) {
		t.Fatal("committed receipt incorrectly treated as new expired activation", s, string(b))
	}
	activationCapture(t, s, in, b, "exact_committed_receipt_after_readiness_expiry_without_renewal")
	changed := in
	changed.Command = f.meta(nil)
	s, b = f.activate(t, changed, nil)
	if s != 409 {
		t.Fatal("new activation used expired consent", s, string(b))
	}
	activationCapture(t, s, changed, b, "new_request_cannot_reuse_expired_pair")
	tx = begin(t, connect(t), f.workspace)
	var after time.Time
	if e := tx.QueryRow(ctx, `SELECT expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&after); e != nil || !after.Equal(before) {
		t.Fatal("technical replay renewed TTL", e)
	}
	tx.Rollback(ctx)
	f.assertCounts(t, 1, 4)
}
