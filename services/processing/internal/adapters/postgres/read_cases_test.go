package postgres

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

type readPool struct{ slots chan *pgx.Conn }
type readTx struct {
	pgx.Tx
	once    sync.Once
	release func()
}

func (t *readTx) Commit(ctx context.Context) error {
	e := t.Tx.Commit(ctx)
	t.once.Do(t.release)
	return e
}
func (t *readTx) Rollback(ctx context.Context) error {
	e := t.Tx.Rollback(ctx)
	t.once.Do(t.release)
	return e
}
func (p *readPool) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	if o.AccessMode != pgx.ReadOnly || o.IsoLevel != pgx.RepeatableRead {
		return nil, errors.New("not read-only repeatable read")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c := <-p.slots:
		tx, e := c.BeginTx(ctx, o)
		if e != nil {
			p.slots <- c
			return nil, e
		}
		return &readTx{Tx: tx, release: func() { p.slots <- c }}, nil
	}
}
func newReadPool(t *testing.T, n int) *readPool {
	t.Helper()
	p := &readPool{make(chan *pgx.Conn, n)}
	for i := 0; i < n; i++ {
		p.slots <- connect(t)
	}
	return p
}
func readStore(t *testing.T, f preparedFixture, p *readPool) CaseReadStore {
	t.Helper()
	key, e := readcases.NewCursorKey(bytes.Repeat([]byte{0x71}, 32))
	if e != nil {
		t.Fatal(e)
	}
	return CaseReadStore{DB: p, Content: f.files, Cursors: key}
}
func readGrant(ws string) readcases.Grant {
	return readcases.Grant{WorkspaceID: ws, PrincipalID: owner, Epoch: 7, ExpiresAt: time.Now().Add(time.Minute)}
}
func readRequest(f preparedFixture, k readcases.Kind) readcases.Request {
	return readcases.Request{Kind: k, WorkspaceID: f.request.WorkspaceID, CaseID: f.request.CaseID, Session: "synthetic-session", Limit: 100}
}

// These fixtures persist exact private HIT-79 envelopes with actual expiry refs.
// They do not repair the existing admission writer or stand in for missing
// order_label/next_action. No successful CaseDetail is claimed here.
func seedReadHistory(t *testing.T, f preparedFixture, n int, badDigest ...bool) []string {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	// Only this new test workspace is changed. Existing actual predecessor rows
	// remain untouched in the separate gap test below.
	if _, e := tx.Exec(ctx, `DELETE FROM hp_processing.safety_history WHERE workspace_id=$1 AND case_id=$2`, f.request.WorkspaceID, f.request.CaseID); e == nil {
		t.Fatal("application unexpectedly deleted safety history")
	}
	tx.Rollback(ctx)
	// Keep the admitted row; supply explicitly persisted, synthetic content for
	// that one known row, then append typed events for pagination.
	tx = begin(t, db, f.request.WorkspaceID)
	var initial string
	var admitted time.Time
	if e := tx.QueryRow(ctx, `SELECT entry_id::text,at FROM hp_processing.safety_history WHERE workspace_id=$1 AND case_id=$2`, f.request.WorkspaceID, f.request.CaseID).Scan(&initial, &admitted); e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	for i := 0; i < n; i++ {
		id := initial
		at := admitted
		reason := "SIGNAL_ADMITTED"
		if i > 0 {
			id, _ = newID()
			at = admitted.Add(time.Duration(i) * time.Microsecond)
			reason = "SYNTHETIC_HISTORY"
			if _, e := tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,new_state)VALUES($1,$2,$3,'SYSTEM',$4,$5,'RECEIVED')`, f.request.WorkspaceID, id, f.request.CaseID, at, reason); e != nil {
				t.Fatal(e)
			}
		}
		envelope := struct {
			CaseID   string
			Revision int64
			Decision preparecase.Decision
		}{f.request.CaseID, 1, preparecase.Decision{Reason: reason, State: "RECEIVED", At: at}}
		b, _ := json.Marshal(envelope)
		ref, e := f.files.Write(ctx, b, "HISTORY", preparecase.Origin{WorkspaceID: f.request.WorkspaceID, CaseID: f.request.CaseID, Revision: 1}, at.Add(time.Hour), at)
		if e != nil {
			t.Fatal(e)
		}
		storedDigest := ref.Digest
		if len(badDigest) > 0 && badDigest[0] {
			storedDigest = "sha256:" + string(bytes.Repeat([]byte{'f'}, 64))
		}
		if _, e = tx.Exec(ctx, `INSERT INTO hp_processing.history_content VALUES($1,$2,$3,$4,$5,$6,$7)`, f.request.WorkspaceID, id, ref.ID, storedDigest, ref.Bytes, at, ref.ExpiresAt); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return ids
}

func TestReadHistoryActualStoragePaginationAndGaps(t *testing.T) {
	f := prepared(t, []string{})
	p := newReadPool(t, 1)
	store := readStore(t, f, p)
	r := readRequest(f, readcases.History)
	g := readGrant(r.WorkspaceID)
	if _, e := store.Read(ctx, r, g); !errors.Is(e, readcases.ErrUnavailable) {
		t.Fatal("admission without stored expiry claimed complete", e)
	}
	seedReadHistory(t, f, 102)
	first, e := store.Read(ctx, r, g)
	if e != nil {
		t.Fatal(e)
	}
	page := first.(readcases.HistoryView)
	if len(page.Items) != 100 || page.NextCursor == nil || page.Items[0].ReasonCode != "SIGNAL_ADMITTED" || page.Items[0].OldState != nil || page.Items[0].ActorID != nil || page.Items[0].EpisodeID != nil || page.Items[0].EvidenceRef == nil {
		t.Fatalf("lost required/null/source data: %+v", page)
	}
	seen := map[string]bool{}
	for _, h := range page.Items {
		seen[h.EntryID] = true
		if h.CaseID != r.CaseID || h.At.Location() != time.UTC || !h.ExpiresAt.After(h.At) {
			t.Fatal(h)
		}
	}
	r.Cursor = *page.NextCursor
	next, e := store.Read(ctx, r, g)
	if e != nil {
		t.Fatal(e)
	}
	last := next.(readcases.HistoryView)
	if len(last.Items) != 2 || last.NextCursor != nil {
		t.Fatal(last)
	}
	for _, h := range last.Items {
		if seen[h.EntryID] {
			t.Fatal("duplicate pagination")
		}
		seen[h.EntryID] = true
	}
	if len(seen) != 102 {
		t.Fatal(len(seen))
	}
	other := prepared(t, []string{})
	foreign := r
	foreign.WorkspaceID = other.request.WorkspaceID
	foreign.CaseID = other.request.CaseID
	if _, e = store.Read(ctx, foreign, readGrant(foreign.WorkspaceID)); !errors.Is(e, readcases.ErrInvalid) {
		t.Fatal("foreign cursor accepted", e)
	}
	foreign.Cursor = ""
	foreign.CaseID = r.CaseID
	if _, e = store.Read(ctx, foreign, readGrant(foreign.WorkspaceID)); !errors.Is(e, readcases.ErrNotFound) {
		t.Fatal("foreign case disclosed", e)
	}
	// SET LOCAL cannot leak after commit or after failures when the same physical
	// connection is returned to this one-slot pool.
	c := <-p.slots
	var count int
	if e = c.QueryRow(ctx, `SELECT count(*) FROM hp_processing.case_identity`).Scan(&count); e != nil || count != 0 {
		t.Fatal("scope leaked", count, e)
	}
	p.slots <- c
	list := readRequest(f, readcases.List)
	list.CaseID = ""
	if _, e = store.Read(ctx, list, g); !errors.Is(e, readcases.ErrUnavailable) {
		t.Fatal("missing label/action manufactured", e)
	}
	if _, e = store.Read(ctx, readRequest(f, readcases.Detail), g); !errors.Is(e, readcases.ErrUnavailable) {
		t.Fatal("missing detail manufactured", e)
	}
	list.WorkspaceID, _ = newID()
	empty, e := store.Read(ctx, list, readGrant(list.WorkspaceID))
	if e != nil || len(empty.(readcases.Cases).Items) != 0 || empty.(readcases.Cases).NextCursor != nil {
		t.Fatal(empty, e)
	}
}

func TestReadHistoryParallelWorkspacesAndNoBusinessWrites(t *testing.T) {
	a := prepared(t, []string{})
	b := prepared(t, []string{})
	seedReadHistory(t, a, 2)
	seedReadHistory(t, b, 2)
	p := newReadPool(t, 2)
	one := readStore(t, a, p)
	two := readStore(t, b, p)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f, s := a, one
			if i%2 == 1 {
				f, s = b, two
			}
			r := readRequest(f, readcases.History)
			v, e := s.Read(ctx, r, readGrant(r.WorkspaceID))
			if e == nil {
				for _, entry := range v.(readcases.HistoryView).Items {
					if entry.CaseID != r.CaseID {
						e = errors.New("foreign history")
					}
				}
			}
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, f := range []preparedFixture{a, b} {
		decisionCounts(t, f.request.WorkspaceID, 0, 2, 0)
		c := connect(t)
		tx := begin(t, c, f.request.WorkspaceID)
		var rev int64
		if e := tx.QueryRow(ctx, `SELECT revision FROM hp_processing.case_identity WHERE case_id=$1`, f.request.CaseID).Scan(&rev); e != nil || rev != 1 {
			t.Fatal("read wrote case", rev, e)
		}
	}
}

func TestReadCaseSQLKeysetAndReadonlyOnActualRole(t *testing.T) {
	f := prepared(t, []string{})
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	var admitted time.Time
	if e := tx.QueryRow(ctx, `SELECT admitted_at FROM hp_processing.case_identity WHERE workspace_id=$1 AND case_id=$2`, f.request.WorkspaceID, f.request.CaseID).Scan(&admitted); e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 101; i++ {
		id, _ := newID()
		at := admitted.Add(time.Duration(i) * time.Microsecond)
		history, _ := newID()
		if _, e := tx.Exec(ctx, `INSERT INTO hp_processing.case_identity(workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge)VALUES($1,$2,$3,$4,'acct_synthetic',false,$5,$6,$7,'RECEIVED',1,'UNESTABLISHED')`, f.request.WorkspaceID, id, op, config, "pagination_"+id, at, owner); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,new_state)VALUES($1,$2,$3,'SYSTEM',$4,'SIGNAL_ADMITTED','RECEIVED')`, f.request.WorkspaceID, history, id, at); e != nil {
			t.Fatal(e)
		}
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	p := newReadPool(t, 1)
	readTx, e := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = readTx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, f.request.WorkspaceID); e != nil {
		t.Fatal(e)
	}
	var readonly string
	if e = readTx.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readonly); e != nil || readonly != "on" {
		t.Fatal(readonly, e)
	}
	var snapshot time.Time
	if e = readTx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&snapshot); e != nil {
		t.Fatal(e)
	}
	type position struct {
		at time.Time
		id string
	}
	collect := func(after *position, state string) []position {
		var at *time.Time
		var id *string
		if after != nil {
			at = &after.at
			id = &after.id
		}
		rows, e := readTx.Query(ctx, caseReadSQL, f.request.WorkspaceID, "", state, snapshot, at, id, 101)
		if e != nil {
			t.Fatal(e)
		}
		defer rows.Close()
		var all []position
		for rows.Next() {
			values, e := rows.Values()
			if e != nil {
				t.Fatal(e)
			}
			all = append(all, position{values[7].(time.Time), values[0].(string)})
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return all
	}
	first := collect(nil, "RECEIVED")
	if len(first) != 101 {
		t.Fatal(len(first))
	}
	last := first[99]
	second := collect(&last, "RECEIVED")
	if len(second) != 2 || second[0].id != first[100].id {
		t.Fatal("wrong keyset continuation")
	}
	if len(collect(nil, "CONFIRMED")) != 0 {
		t.Fatal("filter returned other state")
	}
	if _, e = readTx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code)VALUES($1,'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee',$2,'SYSTEM',now(),'SHOULD_NOT_WRITE')`, f.request.WorkspaceID, f.request.CaseID); e == nil {
		t.Fatal("read transaction allowed write")
	}
	readTx.Rollback(ctx)
	c := <-p.slots
	var count int
	if e = c.QueryRow(ctx, `SELECT count(*) FROM hp_processing.case_identity`).Scan(&count); e != nil || count != 0 {
		t.Fatal("rollback scope leak", count, e)
	}
	p.slots <- c
	store := readStore(t, f, p)
	r := readRequest(f, readcases.List)
	r.CaseID = ""
	if _, e = store.Read(ctx, r, readGrant(r.WorkspaceID)); !errors.Is(e, readcases.ErrUnavailable) {
		t.Fatal("unapproved display values fabricated", e)
	}
	c = <-p.slots
	if e = c.QueryRow(ctx, `SELECT count(*) FROM hp_processing.case_identity`).Scan(&count); e != nil || count != 0 {
		t.Fatal("failed projection scope leak", count, e)
	}
	p.slots <- c
}

func TestReadContentFaultsAndSQLCancellation(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "wrong_hash", "symlink", "expired", "wrong_owner", "timeout", "unavailable_db"} {
		t.Run(mode, func(t *testing.T) {
			f := prepared(t, []string{})
			seedReadHistory(t, f, 1, mode == "wrong_hash")
			p := newReadPool(t, 1)
			store := readStore(t, f, p)
			r := readRequest(f, readcases.History)
			db := connect(t)
			tx := begin(t, db, r.WorkspaceID)
			var id, hash string
			var expiry time.Time
			var size int
			if e := tx.QueryRow(ctx, `SELECT detail_ref::text,detail_sha256,detail_bytes,expires_at FROM hp_processing.history_content`).Scan(&id, &hash, &size, &expiry); e != nil {
				t.Fatal(e)
			}
			tx.Rollback(ctx)
			path := filepath.Join(f.path, id)
			switch mode {
			case "missing":
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
			case "corrupt":
				if e := os.WriteFile(path, []byte("corrupt"), 0600); e != nil {
					t.Fatal(e)
				}
			case "wrong_hash":
				// Wrong hash was inserted as a new synthetic ref. The application
				// role cannot UPDATE existing history_content; no grant is widened.
			case "symlink":
				original := filepath.Join(t.TempDir(), "foreign")
				if e := os.Rename(path, original); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(original, path); e != nil {
					t.Fatal(e)
				}
			case "expired":
				store.Content = expiredReadContent{f.files, expiry.Add(time.Second)}
			case "wrong_owner":
				other := prepared(t, []string{})
				store.Content = ownerSubstitute{f.files, other.request.WorkspaceID}
			case "unavailable_db":
				store.DB = failingReadDB{}
			}
			c := ctx
			var cancel context.CancelFunc
			if mode == "timeout" {
				c, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, e := store.Read(c, r, readGrant(r.WorkspaceID)); e == nil {
				t.Fatal("fault produced success")
			}
			decisionCounts(t, r.WorkspaceID, 0, 1, 0)
			if mode == "expired" {
				if _, e := os.Stat(path); !os.IsNotExist(e) {
					t.Fatal("expired file retained", e)
				}
			}
		})
	}
}

type failingReadDB struct{}

func (failingReadDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.New("synthetic database unavailable")
}

type expiredReadContent struct {
	OwnedContent
	at time.Time
}

func (c expiredReadContent) ReadOwned(ctx context.Context, r preparecase.Ref, o preparecase.Origin, now time.Time) ([]byte, error) {
	return c.OwnedContent.ReadOwned(ctx, r, o, c.at)
}

type ownerSubstitute struct {
	OwnedContent
	workspace string
}

func (c ownerSubstitute) ReadOwned(ctx context.Context, r preparecase.Ref, o preparecase.Origin, now time.Time) ([]byte, error) {
	o.WorkspaceID = c.workspace
	return c.OwnedContent.ReadOwned(ctx, r, o, now)
}

// Explicit controlled IAM and shared epoch fixture; no production default.
type readAuthority struct{ grant readcases.Grant }

func (a readAuthority) Authorize(context.Context, readcases.Request) (readcases.Grant, error) {
	return a.grant, nil
}

type readBoundary struct {
	mu        sync.Mutex
	epoch     int64
	denyPhase string
}

func (g *readBoundary) Start(ctx context.Context, a readcases.Grant, phase string, work func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a.Epoch != g.epoch || phase == g.denyPhase {
		return readcases.ErrDenied
	}
	return work(ctx)
}

func TestReadActualMTLSPostgresContentComposition(t *testing.T) {
	f := prepared(t, []string{})
	seedReadHistory(t, f, 2)
	p := newReadPool(t, 1)
	store := readStore(t, f, p)
	g := &readBoundary{epoch: 7}
	service := readcases.Service{Authority: readAuthority{readGrant(f.request.WorkspaceID)}, Boundary: g, Clock: testClock{time.Now}, Repository: store}
	roots, serverCert, clientCert := readCertificates(t)
	server, e := casereadhttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{serverCert}, ClientCAs: roots}, service, "bff.private")
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewUnstartedServer(server.Handler)
	ts.TLS = server.TLSConfig
	ts.StartTLS()
	defer ts.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{clientCert}, MinVersion: tls.VersionTLS13}}, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	captures := []map[string]any{}
	for _, suffix := range []string{"/history", "", "?unused=1"} {
		url := ts.URL + "/api/v1/workspaces/" + f.request.WorkspaceID + "/cases/" + f.request.CaseID + suffix
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("X-HitPipe-Session", "synthetic-session")
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		want := 200
		if suffix == "" {
			want = 503
		}
		if suffix == "?unused=1" {
			want = 400
		}
		if res.StatusCode != want {
			t.Fatalf("%s status=%d body=%s", suffix, res.StatusCode, body)
		}
		var payload any
		if json.Unmarshal(body, &payload) != nil {
			t.Fatal(string(body))
		}
		schema := "History"
		if want != 200 {
			schema = "Error"
		}
		captures = append(captures, map[string]any{"operation": "getCaseHistory", "status": want, "schema": schema, "body": payload, "local_actual_mtls_postgres_content": true})
	}
	if output := os.Getenv("HIT95_SCHEMA_CAPTURE"); output != "" {
		b, _ := json.MarshalIndent(captures, "", "  ")
		if e = os.WriteFile(output, append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	g.mu.Lock()
	g.denyPhase = "RESPONSE_START"
	g.mu.Unlock()
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/workspaces/"+f.request.WorkspaceID+"/cases/"+f.request.CaseID+"/history", nil)
	req.Header.Set("X-HitPipe-Session", "synthetic-session")
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("revoke allowed response", res.StatusCode)
	}
}

func readCertificates(t *testing.T) (*x509.CertPool, tls.Certificate, tls.Certificate) {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic HIT95 CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	parsed, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	makeCert := func(name string, server bool) tls.Certificate {
		private, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
		tmpl := &x509.Certificate{SerialNumber: serial, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		if server {
			tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
			tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}
		cert, e := x509.CreateCertificate(rand.Reader, tmpl, parsed, &private.PublicKey, key)
		if e != nil {
			t.Fatal(e)
		}
		return tls.Certificate{Certificate: [][]byte{cert}, PrivateKey: private}
	}
	return roots, makeCert("processing.private", true), makeCert("bff.private", false)
}
