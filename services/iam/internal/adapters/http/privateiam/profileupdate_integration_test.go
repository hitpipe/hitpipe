//go:build integration

package privateiam_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wired "github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/content/profilefiles"
	wire "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionissuer"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

type profileLoginFixture struct {
	proof string
	login issuesession.VerifiedLogin
	used  atomic.Bool
}

func (f *profileLoginFixture) Consume(ctx context.Context, proof string) (issuesession.VerifiedLogin, error) {
	if ctx.Err() != nil || proof != f.proof || !f.used.CompareAndSwap(false, true) {
		return issuesession.VerifiedLogin{}, issuesession.ErrConfirmationDenied
	}
	return f.login, nil
}

type profileIssuerGuard struct{}

func (profileIssuerGuard) Check(ctx context.Context, identity issuesession.Identity) (issuesession.GuardDecision, error) {
	if ctx.Err() != nil || identity.Issuer != "https://profile-fixture.invalid" || !runtimeaccess.ValidUUID(identity.Subject) {
		return issuesession.GuardDecision{}, issuesession.ErrIssuanceDenied
	}
	return issuesession.GuardDecision{State: issuesession.IssuanceAllowed, Epoch: 3}, nil
}

func issueProfileSession(t *testing.T, db labDatabase, now time.Time) issuesession.Result {
	t.Helper()
	store, err := sessionissuer.New(db.app)
	if err != nil {
		t.Fatal("could not configure the existing HIT-83 issuer store")
	}
	// The one-use proof verifier and guard are fixture authorities. The
	// issuer below generates both raw values and commits their real digests.
	fixture := &profileLoginFixture{proof: labUUID(), login: issuesession.VerifiedLogin{
		Issuer: "https://profile-fixture.invalid", Subject: labUUID(), EmailVerified: true,
		AttemptCreatedAt: now.Add(-time.Second), AttemptExpiresAt: now.Add(time.Minute),
	}}
	issuer, err := issuesession.New(fixture, labClock{now}, profileIssuerGuard{}, store,
		issuesession.Policy{AbsoluteLifetime: time.Hour, IdleLifetime: time.Minute * 30})
	if err != nil {
		t.Fatal("could not configure the explicit synthetic issuance policy")
	}
	issued, err := issuer.Execute(t.Context(), fixture.proof)
	if err != nil || !issued.IsIssued() {
		t.Fatal("HIT-83 did not issue a durable test session")
	}
	t.Cleanup(func() {
		sqlExec(t, db.admin, `DELETE FROM hp_iam.revocations WHERE scope='SESSION' AND subject_id=$1`, issued.SessionID)
		sqlExec(t, db.admin, `DELETE FROM hp_iam.sessions WHERE session_id=$1`, issued.SessionID)
		sqlExec(t, db.admin, `DELETE FROM hp_iam.principals WHERE principal_id=$1`, issued.PrincipalID)
	})
	return issued
}

func serveProfileFixture(t *testing.T, service *wired.Server, peers labPeers, iam *http.Client,
	content app.Content, commands app.CommandAuthority, wantGate string) (string, func()) {
	t.Helper()
	const address = "127.0.0.1:8443"
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	var listenErr error
	go func() {
		defer close(stopped)
		listenErr = service.ListenProfile(ctx, address, peers.server, peers.ca.roots, content, commands)
	}()
	stop := sync.OnceFunc(func() {
		cancel()
		select {
		case <-stopped:
			if listenErr != nil {
				t.Error("the private profile listener failed")
			}
		case <-time.After(5 * time.Second):
			t.Error("the private profile listener failed to close")
		}
	})
	t.Cleanup(stop)
	base := "https://" + address
	ready, finish := context.WithTimeout(t.Context(), 4*time.Second)
	defer finish()
	for {
		registration := post(ready, iam, base+registerPath, peers.local.Registration, "")
		if registration.err == nil {
			snapshot := assertRegistration(t, registration, peers.local)
			if snapshot.Gate != wantGate {
				t.Fatal("the profile listener changed the established runtime gate")
			}
			return base, stop
		}
		select {
		case <-stopped:
			t.Fatal("profile listener exited during bootstrap")
		case <-ready.Done():
			t.Fatal("the private profile listener did not establish mTLS")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func profileCall(ctx context.Context, client *http.Client, base, method, session, csrf, id string, payload []byte, headers http.Header) result {
	request, err := http.NewRequestWithContext(ctx, method, base+wire.Path, bytes.NewReader(payload))
	if err != nil {
		return result{err: err}
	}
	if method == http.MethodPatch {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", id)
	}
	if session != "" {
		request.Header.Set("X-HitPipe-Session", session)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	for name, values := range headers {
		request.Header[name] = values
	}
	response, err := client.Do(request)
	if err != nil {
		return result{err: err}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<16))
	return result{status: response.StatusCode, body: body, err: err}
}

func updatePayload(id, name, zone string, revision int64) []byte {
	return fmt.Appendf(nil, `{"command":{"request_id":%q,"expected_revision":%d},"display_name":%q,"timezone":%q}`, id, revision, name, zone)
}

func assertProfileStatus(t *testing.T, got result, want int, credentials issuesession.Result) {
	t.Helper()
	if got.err != nil || got.status != want ||
		bytes.Contains(got.body, []byte(credentials.Session.Reveal())) ||
		bytes.Contains(got.body, []byte(credentials.CSRF.Reveal())) {
		t.Fatalf("profile request did not return expected status without credentials; got %d, want %d", got.status, want)
	}
	if want >= 400 {
		assertDenied(t, got, credentials.CSRF.Reveal())
	}
}

func assertReturnedProfile(t *testing.T, got result, want getprofile.Profile, credentials issuesession.Result) {
	t.Helper()
	assertProfileStatus(t, got, http.StatusOK, credentials)
	var profile getprofile.Profile
	var keys map[string]json.RawMessage
	if json.Unmarshal(got.body, &profile) != nil || json.Unmarshal(got.body, &keys) != nil || len(keys) != 4 || profile != want {
		t.Fatal("real HTTP response did not contain the four current Profile fields")
	}
}

func currentProfilePointer(t *testing.T, db labDatabase, principal string) app.Pointer {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	var current app.Pointer
	if err := db.app.QueryRow(ctx, `SELECT profile_content_ref::text,profile_digest
 FROM hp_iam.principals WHERE principal_id=$1`, principal).Scan(&current.Ref, &current.Digest); err != nil ||
		!runtimeaccess.ValidUUID(current.Ref) || !runtimeaccess.ValidFingerprint(current.Digest) {
		t.Fatal("actual application role could not verify the persisted profile pointer")
	}
	return current
}

type profileContentFixture struct {
	app.Content
	root        string
	refuseWrite atomic.Bool
}

func (f *profileContentFixture) WriteProfile(ctx context.Context, p getprofile.Profile) (app.Pointer, error) {
	if !f.refuseWrite.CompareAndSwap(true, false) {
		return f.Content.WriteProfile(ctx, p)
	}
	// Force the actual owner writer to reject its directory before the
	// pointer can be switched. Only this disposable synthetic root changes.
	if os.Chmod(f.root, 0755) != nil {
		return app.Pointer{}, app.ErrUnavailable
	}
	defer func() { _ = os.Chmod(f.root, 0700) }()
	return f.Content.WriteProfile(ctx, p)
}

type profileConfirmFixture struct {
	app.CommandAuthority
	afterSQL func(context.Context, app.Receipt) error
}

type profileConfirmRecord struct {
	app.CommandRecord
	afterSQL func(context.Context, app.Receipt) error
}

func (f profileConfirmFixture) WithCommand(ctx context.Context, principal, request string, use func(context.Context, app.CommandRecord) error) error {
	return f.CommandAuthority.WithCommand(ctx, principal, request, func(work context.Context, record app.CommandRecord) error {
		return use(work, profileConfirmRecord{record, f.afterSQL})
	})
}

func (f profileConfirmRecord) Confirm(ctx context.Context, receipt app.Receipt) error {
	if err := f.afterSQL(ctx, receipt); err != nil {
		return err
	}
	return f.CommandRecord.Confirm(ctx, receipt)
}

func TestPostgresProfileUpdatePrivateListener(t *testing.T) {
	db := openLabDatabase(t)
	lab := labData{now: time.Now().UTC().Truncate(time.Microsecond), localRuntime: labUUID(), callerRuntime: labUUID(),
		localBoot: labUUID(), callerBoot: labUUID()}
	peers := newLabPeers(t, lab)
	t.Cleanup(func() {
		sqlExec(t, db.admin, `DELETE FROM hp_iam.revocation_participants WHERE runtime_id IN ($1,$2)`, lab.localRuntime, lab.callerRuntime)
		sqlExec(t, db.admin, `DELETE FROM hp_iam.runtime_registry WHERE runtime_id IN ($1,$2)`, lab.localRuntime, lab.callerRuntime)
	})
	owner := issueProfileSession(t, db, lab.now)
	second := issueProfileSession(t, db, lab.now)
	ownerRoot, receiptRoot := t.TempDir(), t.TempDir()
	for _, root := range []string{ownerRoot, receiptRoot} {
		// A runner's temporary filesystem may pass its setgid bit to
		// newly created directories. Both test stores require exact 0700.
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal("could not set owner-only fixture directory permissions")
		}
	}
	openFiles := func() *profilefiles.Reader {
		files, err := profilefiles.New(ownerRoot)
		if err != nil {
			t.Fatal("could not open the dedicated profile owner directory")
		}
		t.Cleanup(func() { _ = files.Close() })
		return files
	}
	openCommands := func() *profileCommandFixture {
		records, err := newProfileCommandFixture(receiptRoot)
		if err != nil {
			t.Fatal("could not open the synthetic durable command authority")
		}
		return records
	}
	files, records := openFiles(), openCommands()
	current := getprofile.Profile{UserID: owner.PrincipalID, DisplayName: "Синтетический профиль", Timezone: "UTC", Revision: 7}
	original, err := files.WriteProfile(t.Context(), current)
	if err != nil {
		t.Fatal("could not publish the original synthetic profile")
	}
	sqlExec(t, db.app, `UPDATE hp_iam.principals SET profile_content_ref=$2,profile_digest=$3 WHERE principal_id=$1`,
		owner.PrincipalID, original.Ref, original.Digest)
	content := &profileContentFixture{Content: files, root: ownerRoot}
	var confirmMode atomic.Int32
	var sawCommitted atomic.Bool
	afterSQL := func(ctx context.Context, receipt app.Receipt) error {
		mode := confirmMode.Swap(0)
		if mode == 0 {
			return nil
		}
		var actual app.Pointer
		if err := db.admin.QueryRow(ctx, `SELECT profile_content_ref::text,profile_digest
 FROM hp_iam.principals WHERE principal_id=$1`, owner.PrincipalID).Scan(&actual.Ref, &actual.Digest); err != nil || actual != receipt.After {
			return app.ErrUnavailable
		}
		sawCommitted.Store(true)
		if mode == 1 {
			return app.ErrUnavailable
		}
		_, err := db.admin.Exec(ctx, `INSERT INTO hp_iam.revocations
 (revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 VALUES($1,'SESSION',$2,4,$3::timestamptz,$3::timestamptz+interval '5 seconds','PENDING','PROFILE_TEST_ONLY')`,
			labUUID(), owner.SessionID, lab.now)
		return err
	}
	commands := func() app.CommandAuthority { return profileConfirmFixture{records, afterSQL} }
	iam, bff := labClient(t, peers.ca, peers.iam), labClient(t, peers.ca, peers.bff)
	inventory := func() []runtimeaccess.Binding { return []runtimeaccess.Binding{peers.local, peers.remote} }
	fixtureTest := t
	service := newLabApp(t, db, lab, peers.local, inventory(), labOwner{})
	base, stop := serveProfileFixture(t, service, peers, iam, content, commands(), "SYNCING")
	closeListener := func() {
		stop()
		iam.CloseIdleConnections()
		bff.CloseIdleConnections()
	}
	get := func() result {
		return profileCall(t.Context(), bff, base, http.MethodGet, owner.Session.Reveal(), "", "", nil, nil)
	}
	patch := func(client *http.Client, session, csrf, id string, payload []byte, headers http.Header) result {
		return profileCall(t.Context(), client, base, http.MethodPatch, session, csrf, id, payload, headers)
	}
	save := func(id, name, zone string, expected int64) result {
		return patch(bff, owner.Session.Reveal(), owner.CSRF.Reveal(), id, updatePayload(id, name, zone, expected), nil)
	}
	establishOpen := func() {
		t.Helper()
		remote := assertRegistration(t, post(t.Context(), bff, base+registerPath, peers.remote.Registration, ""), peers.remote)
		if remote.Gate != "SYNCING" {
			t.Fatal("BFF registration alone granted privileged RPC")
		}
		// Production BFF boot acknowledgement is out of scope. The test
		// fixture explicitly provisions only its known BFF runtime as OPEN.
		sqlExec(t, db.admin, `UPDATE hp_iam.runtime_registry SET status='OPEN' WHERE runtime_id=$1 AND boot_id=$2`,
			peers.remote.RuntimeID, peers.remote.BootID)
	}
	establishOpen()
	assertDenied(t, save(labUUID(), "Фикстура", "UTC", 7), owner.CSRF.Reveal())
	if err := service.Start(t.Context()); err != nil {
		t.Fatal("could not apply the exact IAM denial set and open the listener gate")
	}
	assertReturnedProfile(t, get(), current, owner)

	t.Run("verified mTLS stored session CSRF and strict CAS input", func(t *testing.T) {
		id := labUUID()
		payload := updatePayload(id, "Синтетическое изменение", "Asia/Tokyo", 7)
		for _, certs := range [][]tls.Certificate{
			nil,
			{peers.ca.leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, true)},
			{peers.ca.leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, false)},
			{newLabCA(t).leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, false)},
		} {
			got := patch(labClient(t, peers.ca, certs...), owner.Session.Reveal(), owner.CSRF.Reveal(), id, payload, nil)
			if got.err == nil || got.status != 0 {
				t.Fatal("unqualified client certificate passed actual mTLS")
			}
		}
		tls12 := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: peers.ca.roots,
			Certificates: []tls.Certificate{peers.bff}, MaxVersion: tls.VersionTLS12}}
		t.Cleanup(tls12.CloseIdleConnections)
		oldClient := &http.Client{Transport: tls12, Timeout: time.Second}
		if got := patch(oldClient, owner.Session.Reveal(), owner.CSRF.Reveal(), id, payload, nil); got.err == nil || got.status != 0 {
			t.Fatal("the profile listener accepted TLS 1.2")
		}
		assertProfileStatus(t, patch(iam, owner.Session.Reveal(), owner.CSRF.Reveal(), id, payload, nil), 403, owner)
		assertProfileStatus(t, patch(bff, "", owner.CSRF.Reveal(), id, payload, nil), 401, owner)
		assertProfileStatus(t, patch(bff, strings.Repeat("x", 43), owner.CSRF.Reveal(), id, payload, nil), 401, owner)
		assertProfileStatus(t, patch(bff, second.Session.Reveal(), owner.CSRF.Reveal(), id, payload, nil), 403, owner)
		assertProfileStatus(t, patch(bff, owner.Session.Reveal(), second.CSRF.Reveal(), id, payload, nil), 403, owner)
		assertProfileStatus(t, patch(bff, owner.Session.Reveal(), owner.CSRF.Reveal(), labUUID(), payload, nil), 400, owner)
		assertProfileStatus(t, patch(bff, owner.Session.Reveal(), owner.CSRF.Reveal(), id, payload,
			http.Header{"If-Match": []string{`"6"`}}), 412, owner)
		assertProfileStatus(t, patch(bff, owner.Session.Reveal(), owner.CSRF.Reveal(), id, payload,
			http.Header{"X-User-Id": []string{second.PrincipalID}}), 403, owner)
		invalid := append(append([]byte(nil), payload[:len(payload)-1]...), []byte(`,"timezone":null}`)...)
		assertProfileStatus(t, patch(bff, owner.Session.Reveal(), owner.CSRF.Reveal(), id, invalid, nil), 400, owner)

		sqlExec(t, db.admin, `UPDATE hp_iam.sessions SET created_at=$2, expires_at=$3, idle_expires_at=$3 WHERE session_id=$1`,
			owner.SessionID, lab.now.Add(-time.Hour), lab.now.Add(-time.Second))
		assertProfileStatus(t, save(labUUID(), "Фикстура", "UTC", 7), 401, owner)
		sqlExec(t, db.admin, `UPDATE hp_iam.sessions SET created_at=$2,expires_at=$3,idle_expires_at=$4 WHERE session_id=$1`,
			owner.SessionID, owner.CreatedAt, owner.ExpiresAt, owner.IdleExpiresAt)
		if currentProfilePointer(t, db, owner.PrincipalID) != original {
			t.Fatal("denied HTTP requests switched the profile pointer")
		}
	})

	t.Run("content failure and bad digest keep the original profile", func(t *testing.T) {
		content.refuseWrite.Store(true)
		assertProfileStatus(t, save(labUUID(), "Не записано", "UTC", 7), 503, owner)
		assertReturnedProfile(t, get(), current, owner)
		sqlExec(t, db.app, `UPDATE hp_iam.principals SET profile_digest=$2 WHERE principal_id=$1`,
			owner.PrincipalID, "sha256:"+strings.Repeat("a", 64))
		assertProfileStatus(t, save(labUUID(), "Не записано", "UTC", 7), 503, owner)
		assertProfileStatus(t, get(), 503, owner)
		sqlExec(t, db.app, `UPDATE hp_iam.principals SET profile_digest=$2 WHERE principal_id=$1`, owner.PrincipalID, original.Digest)
		if currentProfilePointer(t, db, owner.PrincipalID) != original {
			t.Fatal("content validation failure changed the accepted pointer")
		}
	})

	t.Run("first write exact retry and concurrent stale revision", func(t *testing.T) {
		id := labUUID()
		current.DisplayName, current.Timezone, current.Revision = "Синтетическое имя 1", "Europe/Moscow", 8
		payload := updatePayload(id, current.DisplayName, current.Timezone, 7)
		assertReturnedProfile(t, patch(bff, owner.Session.Reveal(), owner.CSRF.Reveal(), id, payload,
			http.Header{"If-Match": []string{`"7"`}}), current, owner)
		assertReturnedProfile(t, get(), current, owner)
		published := currentProfilePointer(t, db, owner.PrincipalID)
		p, err := files.Read(t.Context(), published.Ref, published.Digest)
		if published == original || err != nil || p != current {
			t.Fatal("real PostgreSQL and durable owner file did not agree after PATCH")
		}
		assertReturnedProfile(t, save(id, current.DisplayName, current.Timezone, 7), current, owner)
		if currentProfilePointer(t, db, owner.PrincipalID) != published {
			t.Fatal("exact replay wrote another immutable version")
		}
		assertProfileStatus(t, save(id, "Другое содержимое", "UTC", 7), 409, owner)
		assertProfileStatus(t, save(labUUID(), "Старая версия", "UTC", 7), 412, owner)

		results := make(chan result, 2)
		go func() { results <- save(labUUID(), "Параллель A", "UTC", 8) }()
		go func() { results <- save(labUUID(), "Параллель B", "Europe/Moscow", 8) }()
		var next getprofile.Profile
		for _, got := range []result{<-results, <-results} {
			if got.status == http.StatusOK && next == (getprofile.Profile{}) {
				if json.Unmarshal(got.body, &next) != nil || next.UserID != owner.PrincipalID || next.Revision != 9 {
					t.Fatal("parallel winner did not advance the same owner by one revision")
				}
			} else {
				assertProfileStatus(t, got, http.StatusPreconditionFailed, owner)
			}
		}
		if next == (getprofile.Profile{}) {
			t.Fatal("two valid competing commands left no established result")
		}
		current = next
		assertReturnedProfile(t, get(), current, owner)
	})

	t.Run("lost confirmation reopens owner state and recovers exact command", func(t *testing.T) {
		id := labUUID()
		sawCommitted.Store(false)
		confirmMode.Store(1)
		assertProfileStatus(t, save(id, "Восстанавливаемое имя", "Etc/UTC", current.Revision), 503, owner)
		if !sawCommitted.Load() {
			t.Fatal("uncertainty did not occur after the real SQL pointer commit")
		}
		committed := currentProfilePointer(t, db, owner.PrincipalID)
		closeListener()
		if err := files.Close(); err != nil {
			t.Fatal("could not close the old owner file adapter")
		}
		// The closed test process is deliberately fenced before a new IAM
		// boot is admitted. The real registry checks that prior fence.
		sqlExec(t, db.admin, `UPDATE hp_iam.runtime_registry SET status='FENCED',fence_evidence_ref=$2 WHERE runtime_id=$1`,
			peers.local.RuntimeID, labUUID())
		peers.local.Generation++
		peers.local.BootID = labUUID()
		files, records = openFiles(), openCommands()
		content = &profileContentFixture{Content: files, root: ownerRoot}
		service = newLabApp(t, db, lab, peers.local, inventory(), labOwner{})
		base, stop = serveProfileFixture(fixtureTest, service, peers, iam, content, commands(), "SYNCING")
		establishOpen()
		if err := service.Start(t.Context()); err != nil {
			t.Fatal("could not open the newly registered IAM boot")
		}
		current.DisplayName, current.Timezone, current.Revision = "Восстанавливаемое имя", "Etc/UTC", current.Revision+1
		assertReturnedProfile(t, save(id, current.DisplayName, current.Timezone, current.Revision-1), current, owner)
		assertReturnedProfile(t, get(), current, owner)
		if currentProfilePointer(t, db, owner.PrincipalID) != committed {
			t.Fatal("reopened authority rewrote an already committed profile")
		}
	})

	t.Run("the missing command authority closes PATCH and leaves GET functional", func(t *testing.T) {
		closeListener()
		base, stop = serveProfileFixture(fixtureTest, service, peers, iam, content, nil, "OPEN")
		previous := currentProfilePointer(t, db, owner.PrincipalID)
		assertReturnedProfile(t, get(), current, owner)
		assertProfileStatus(t, save(labUUID(), "Незапущенный writer", "UTC", current.Revision), 503, owner)
		if currentProfilePointer(t, db, owner.PrincipalID) != previous {
			t.Fatal("PATCH wrote without an explicit global command authority")
		}
		closeListener()
		base, stop = serveProfileFixture(fixtureTest, service, peers, iam, content, commands(), "OPEN")
		assertReturnedProfile(t, get(), current, owner)
	})

	t.Run("a global revocation after SQL commit prevents response start", func(t *testing.T) {
		sawCommitted.Store(false)
		confirmMode.Store(2)
		id := labUUID()
		assertProfileStatus(t, save(id, "Отзыв после SQL", "UTC", current.Revision), 403, owner)
		if !sawCommitted.Load() {
			t.Fatal("the gate test did not exercise a genuine post-commit revocation")
		}
		assertDenied(t, get(), owner.Session.Reveal())
		assertDenied(t, save(id, "Отзыв после SQL", "UTC", current.Revision), owner.CSRF.Reveal())
	})
}
