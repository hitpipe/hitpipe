package postgres

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDraftHTTPAuthoritySessionCSRFAndActualStart(t *testing.T) {
	for _, mode := range []string{"missing_session", "invalid_session", "missing_csrf", "wrong_csrf", "forged_role_header", "reader_write", "operator_write", "denied", "expired_grant", "stale_epoch", "closed_gate", "non_invoking_gate", "response_gate"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			headers := map[string]string{}
			want := 403
			switch mode {
			case "missing_session":
				headers["X-HitPipe-Session"] = ""
				want = 401
			case "invalid_session":
				headers["X-HitPipe-Session"] = "foreign_session"
				want = 401
			case "missing_csrf":
				headers["X-CSRF-Token"] = ""
				want = 400
			case "wrong_csrf":
				headers["X-CSRF-Token"] = strings.Repeat("x", 32)
			case "forged_role_header":
				headers["X-HitPipe-Role"] = "OWNER"
				want = 400
			case "reader_write":
				f.authority.role = "READER"
			case "operator_write":
				f.authority.role = "OPERATOR"
			case "denied":
				f.authority.denied = true
			case "expired_grant":
				f.authority.expiry = time.Now().Add(-time.Second)
			case "stale_epoch":
				f.gate.epoch++
			case "closed_gate":
				f.gate.closed = true
			case "non_invoking_gate":
				f.gate.noCall = true
			case "response_gate":
				f.gate.denyPhase = "DRAFT_RESPONSE_START"
			}
			in := drafts.DraftWrite{Command: f.meta(nil), Config: f.config()}
			status, body := f.call(t, drafts.Save, in, headers)
			if status != want {
				t.Fatal("authorization", mode, status, string(body))
			}
			if bytes.Contains(body, []byte(in.Config.Name)) {
				t.Fatal("private response leaked before response gate")
			}
			draftCapture(t, drafts.Save, status, in, body, "security_"+mode)
			if mode == "response_gate" {
				f.counts(t, 1)
				f.gate.mu.Lock()
				f.gate.denyPhase = ""
				f.gate.mu.Unlock()
				status, body = f.call(t, drafts.Save, in, nil)
				if status != 201 {
					t.Fatal("committed response replay after grant recovery", status, string(body))
				}
				f.counts(t, 1)
			} else {
				db := connect(t)
				tx := begin(t, db, f.workspace)
				var n int
				if e := tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&n); e != nil || n != 0 {
					t.Fatal("denied request wrote draft", n, e)
				}
			}
		})
	}
}
func TestDraftTLSCallerCertificateAndMissingDependencies(t *testing.T) {
	for _, mode := range []string{"wrong_service", "expired_client", "foreign_ca", "missing_client", "wrong_server_name"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			tr := f.pki.Transport(t)
			switch mode {
			case "wrong_service":
				tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "iam.test", false, false)}
			case "expired_client":
				tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "bff.test", false, true)}
			case "foreign_ca":
				tr = testfixtures.NewPKI(t).Transport(t)
			case "missing_client":
				tr.TLSClientConfig.Certificates = nil
			case "wrong_server_name":
				tr.TLSClientConfig.ServerName = "other.test"
			}
			client := &http.Client{Transport: tr, Timeout: time.Second}
			r, e := http.NewRequest("GET", f.url+f.pathFor(drafts.Get), nil)
			if e != nil {
				t.Fatal(e)
			}
			r.Header.Set("X-HitPipe-Session", "controlled-session")
			response, e := client.Do(r)
			if mode == "wrong_service" {
				if e != nil || response.StatusCode != 403 {
					t.Fatal("non-BFF caller", response, e)
				}
				response.Body.Close()
			} else {
				if e == nil {
					response.Body.Close()
					t.Fatal("invalid certificate accepted")
				}
			}
		})
	}
}
func TestDraftReadRolesAndForeignDraftScope(t *testing.T) {
	f := newDraftFixture(t)
	f.save(t)
	for _, role := range []string{"READER", "OPERATOR"} {
		f.authority.mu.Lock()
		f.authority.role = role
		f.authority.mu.Unlock()
		status, b := f.call(t, drafts.Get, nil, nil)
		if status != 200 {
			t.Fatal("read role", role, status, string(b))
		}
	}
	foreign := newDraftFixture(t)
	foreign.save(t)
	f.id = foreign.id
	status, b := f.call(t, drafts.Get, nil, nil)
	if status != 404 || bytes.Contains(b, []byte(foreign.config().Name)) {
		t.Fatal("foreign draft existence/content exposed", status, string(b))
	}
	draftCapture(t, drafts.Get, status, nil, b, "foreign_draft_not_found")
}
func TestDraftContextChangeKeepsHistoricalProofAndRequiresFreshCheck(t *testing.T) {
	f := newDraftFixture(t)
	f.save(t)
	rev := int64(1)
	original := f.meta(&rev)
	status, body := f.call(t, drafts.Check, original, nil)
	if status != 200 {
		t.Fatal(status, string(body))
	}
	f.authority.mu.Lock()
	f.authority.epoch = 8
	f.authority.mu.Unlock()
	f.gate.mu.Lock()
	f.gate.epoch = 8
	f.gate.mu.Unlock()
	f.bindings.mu.Lock()
	f.bindings.denied = true
	f.bindings.mu.Unlock()
	fresh := f.meta(&rev)
	status, negative := f.call(t, drafts.Check, fresh, nil)
	var check drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(negative, &check) != nil || check.Passed {
		t.Fatal("changed binding claimed ready", status, string(negative))
	}
	status, replay := f.call(t, drafts.Check, original, nil)
	if status != 200 || !bytes.Equal(body, replay) {
		t.Fatal("historical proof was reissued", status, string(replay))
	}
	snap, e := f.repo.Load(ctx, drafts.Request{Kind: drafts.Get, WorkspaceID: f.workspace, DraftID: f.id}, drafts.Input{}, drafts.Grant{PrincipalID: owner})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := f.files.ReadDraft(ctx, *snap.Root.Tail, origin(f.workspace, f.id, 1), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var node drafts.CommandNode
	json.Unmarshal(raw, &node)
	if node.AuthorityEpoch != 8 {
		t.Fatal("new command context not retained")
	}
	raw, e = f.files.ReadDraft(ctx, *node.Previous, origin(f.workspace, f.id, 1), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(raw, &node)
	if node.AuthorityEpoch != 7 || node.BindingCheck == nil || node.BindingCheck.Epoch != 7 {
		t.Fatal("original positive binding context lost")
	}
	f.counts(t, 3)
	draftCapture(t, drafts.Check, 200, fresh, negative, "changed_epoch_binding_honest_negative")
}
func TestDraftContentExpiryPurgeAndReopenCannotFabricateDraft(t *testing.T) {
	f := newDraftFixture(t)
	policy := drafts.Policy{ContentTTL: 500 * time.Millisecond, CheckTTL: 100 * time.Millisecond, ReplayNodes: 256}
	f.service.Policy = policy
	f.repo.Policy = policy
	f.service.Repository = f.repo
	f.start(t)
	saved, _ := f.save(t)
	time.Sleep(600 * time.Millisecond)
	status, b := f.call(t, drafts.Get, nil, nil)
	if status != 503 {
		t.Fatal("expired draft returned", status, string(b))
	}
	if e := f.files.PurgeExpired(time.Now()); e != nil {
		t.Fatal(e)
	}
	fresh, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	f.service.Repository = DraftStore{draftTestDB{}, fresh, draftjson.Codec{}, testClock{time.Now}, policy}
	f.start(t)
	status, b = f.call(t, drafts.Save, saved, nil)
	if status != 503 {
		t.Fatal("expired replay manufactured replacement", status, string(b))
	}
	f.counts(t, 1)
	draftCapture(t, drafts.Save, status, saved, b, "expired_purged_reopened_no_fabricated_response")
}

func TestDraftRehearsalHTTPArrayBoundsAndCanonicalZeroFalse(t *testing.T) {
	f := newDraftFixture(t)
	f.save(t)
	rev := int64(1)
	for _, n := range []int{1, 10, 0, 11} {
		examples := make([]drafts.ScenarioExample, n)
		for i := range examples {
			examples[i] = drafts.ScenarioExample{SyntheticSessionAlias: "example_zero", AmountMinor: 0, Paid: false, Duplicate: false, MissingContextKeys: []string{}}
		}
		input := drafts.RehearsalRequest{Command: f.meta(&rev), DraftID: f.id, DraftRevision: 1, Examples: examples}
		status, b := f.call(t, drafts.Rehearse, input, nil)
		if n == 1 || n == 10 {
			var out drafts.RehearsalResult
			if status != 200 || json.Unmarshal(b, &out) != nil || len(out.Results) != n || out.LiveRequestsMade != 0 {
				t.Fatal(n, status, string(b))
			}
			for _, v := range out.Results {
				if v != "WOULD_WAIT" {
					t.Fatal("known zero unpaid", v)
				}
			}
		} else if status != 400 {
			t.Fatal("array bound", n, status, string(b))
		}
		draftCapture(t, drafts.Rehearse, status, input, b, "rehearsal_array_"+strconv.Itoa(n))
	}
}
