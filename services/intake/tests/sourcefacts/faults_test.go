//go:build integration

package sourcefacts_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	stripe "github.com/hitpipe/hitpipe/services/intake/internal/adapters/stripe"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntegrationRefundsDisputesAndPartialLists(t *testing.T) {
	for _, name := range []string{"refund", "dispute", "pending-refund", "partial-refunds", "partial-disputes", "wrong-refund-link", "duplicate-list", "two-items-paginated", "malformed-list"} {
		t.Run(name, func(t *testing.T) {
			var page atomic.Int64
			f := newFixture(t, func(f *fixture) {
				if strings.HasPrefix(name, "partial") {
					f.pages = 1
				}
				f.mutate = func(path string, m map[string]any) {
					switch name {
					case "refund", "pending-refund", "wrong-refund-link":
						if path == "/v1/charges/ch_synthetic" && name == "refund" {
							m["amount_refunded"] = int64(400)
						}
						if path == "/v1/refunds" {
							charge, status := "ch_synthetic", "succeeded"
							if name == "wrong-refund-link" {
								charge = "ch_other"
							}
							if name == "pending-refund" {
								status = "pending"
							}
							m["data"] = []any{map[string]any{"id": "re_synthetic", "object": "refund", "charge": charge, "payment_intent": "pi_synthetic", "currency": "usd", "amount": int64(400), "status": status}}
						}
					case "dispute":
						if path == "/v1/charges/ch_synthetic" {
							m["disputed"] = true
						}
						if path == "/v1/disputes" {
							m["data"] = []any{map[string]any{"id": "du_synthetic", "object": "dispute", "charge": "ch_synthetic", "payment_intent": "pi_synthetic", "currency": "usd", "livemode": false}}
						}
					case "partial-refunds":
						if path == "/v1/refunds" {
							m["has_more"] = true
							m["data"] = []any{map[string]any{"id": "re_synthetic", "object": "refund", "charge": "ch_synthetic", "currency": "usd", "amount": int64(0), "status": "succeeded"}}
						}
					case "partial-disputes":
						if path == "/v1/disputes" {
							m["has_more"] = true
							m["data"] = []any{map[string]any{"id": "du_synthetic", "object": "dispute", "charge": "ch_synthetic", "currency": "usd", "livemode": false}}
						}
					case "duplicate-list":
						if strings.HasSuffix(path, "line_items") {
							v := m["data"].([]map[string]any)
							m["data"] = []any{v[0], v[0]}
						}
					case "two-items-paginated":
						if strings.HasSuffix(path, "line_items") {
							if page.Add(1) == 1 {
								m["has_more"] = true
							} else {
								m["data"] = []any{map[string]any{"id": "li_second", "object": "item", "currency": "usd", "quantity": int64(1), "price": "price_synthetic"}}
							}
						}
					case "malformed-list":
						if path == "/v1/refunds" {
							delete(m, "has_more")
						}
					}
				}
			})
			status, b := f.call(f.request())
			if name == "wrong-refund-link" || name == "duplicate-list" || name == "malformed-list" {
				if status == 200 {
					t.Fatal("invalid provider list accepted")
				}
				return
			}
			v, e := sf.Decode(b)
			if status != 200 || e != nil {
				t.Fatal("snapshot failed", status, e)
			}
			switch name {
			case "refund":
				if !v.Complete || v.AmountRefunded == nil || *v.AmountRefunded != 400 {
					t.Fatal("refund omitted")
				}
			case "dispute":
				if !v.Complete || v.Disputed == nil || !*v.Disputed {
					t.Fatal("dispute omitted")
				}
			case "pending-refund", "partial-refunds":
				if v.Complete || v.AmountRefunded != nil {
					t.Fatal("refund unknown became zero")
				}
			case "partial-disputes":
				if v.Complete || v.Disputed == nil || !*v.Disputed {
					t.Fatal("known dispute lost")
				}
			case "two-items-paginated":
				if v.Complete || !v.LineItemsComplete || v.LineItemCount == nil || *v.LineItemCount != 2 || v.PriceID != nil {
					t.Fatal("pagination lost")
				}
				f.mu.Lock()
				events := append([]string(nil), f.events...)
				f.mu.Unlock()
				found := false
				for _, event := range events {
					if strings.Contains(event, "starting_after=li_synthetic") {
						found = true
					}
				}
				if !found {
					t.Fatal("cursor absent")
				}
			}
		})
	}
}
func TestIntegrationTLSHostnameAndNoUnwiredDefaults(t *testing.T) {
	f := newFixture(t, nil)
	pool := x509.NewCertPool()
	pool.AddCert(f.provider.Certificate())
	if _, e := stripe.New("http://bad", &http.Transport{}, credentials{}, f.now, stripe.Bounds{ResponseBytes: 100, Pages: 1, Items: 1}); e == nil {
		t.Fatal("plaintext")
	}
	if _, e := app.New(nil, f.repo, f.content, noopProvider{}, f.now, time.Hour); e == nil {
		t.Fatal("unwired authority")
	}
	provider, e := stripe.New("https://wrong.test", &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", f.provider.Listener.Addr().String())
	}}, credentials{}, f.now, stripe.Bounds{ResponseBytes: 65536, Pages: 2, Items: 200})
	if e != nil {
		t.Fatal(e)
	}
	service, e := app.New(f.authority, f.repo, f.content, provider, f.now, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.Read(context.Background(), f.request()); e == nil || f.requests.Load() != 0 {
		t.Fatal("hostname bypassed")
	}
}

type failingContent struct {
	base              app.Content
	failPut, failRead bool
	onPut             func()
}

func (c failingContent) Put(ctx context.Context, b app.Binding, data []byte, expires time.Time) (contentstore.Ref, error) {
	if c.failPut {
		return contentstore.Ref{}, app.Fail("content")
	}
	ref, e := c.base.Put(ctx, b, data, expires)
	if c.onPut != nil {
		c.onPut()
	}
	return ref, e
}
func (c failingContent) Read(ctx context.Context, b app.Binding, r contentstore.Ref) ([]byte, error) {
	if c.failRead {
		return nil, app.Fail("content")
	}
	return c.base.Read(ctx, b, r)
}
func TestIntegrationFileDatabaseAndStoredIntegrityFailures(t *testing.T) {
	for _, name := range []string{"put", "read", "database"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, nil)
			p := failingContent{base: f.content, failPut: name == "put", failRead: name == "read"}
			if name == "database" {
				p.onPut = f.repo.Close
			} // close the real app pool after file durability, before SQL.
			// Reuse the existing fully real Stripe transport via the fixture's service is
			// intentionally unavailable externally; instantiate the same explicit adapter.
			pool := x509.NewCertPool()
			pool.AddCert(f.provider.Certificate())
			provider, e := stripe.New(f.provider.URL, &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool}}, credentials{}, f.now, stripe.Bounds{ResponseBytes: 65536, Pages: 4, Items: 300})
			if e != nil {
				t.Fatal(e)
			}
			service, e := app.New(f.authority, f.repo, p, provider, f.now, time.Hour)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = service.Read(context.Background(), f.request()); e == nil {
				t.Fatal("storage failure became success")
			}
		})
	}
	t.Run("persisted-content-readback", func(t *testing.T) {
		f := newFixture(t, nil)
		q := f.request()
		result, e := f.service.Read(context.Background(), q)
		if e != nil {
			t.Fatal(e)
		}
		c, e := pgx.Connect(context.Background(), os.Getenv("HIT117_APP_DSN"))
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close(context.Background())
		if _, e = c.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,false)`, ws); e != nil {
			t.Fatal(e)
		}
		var id, sqlBody string
		e = c.QueryRow(context.Background(), `SELECT facts_ref::text,row_to_json(source_facts)::text FROM hp_intake.source_facts WHERE facts_id=$1`, result.Facts.FactsID).Scan(&id, &sqlBody)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(sqlBody, secret) || strings.Contains(sqlBody, "RAW_CUSTOMER_MARKER") || strings.Contains(sqlBody, "amount_received") {
			t.Fatal("payload in SQL")
		}
		path := filepath.Join(f.root, ws, id)
		data, e := os.ReadFile(path)
		if e != nil || bytes.Contains(data, []byte(secret)) || bytes.Contains(data, []byte("RAW_CUSTOMER_MARKER")) {
			t.Fatal("provider payload/secret in content")
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(data, &fields)
		if len(fields) != 28 {
			t.Fatal("wire fields changed", len(fields))
		}
		if e = os.WriteFile(path, []byte("tampered"), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e = f.service.Load(context.Background(), q, result.Facts.FactsID); e == nil {
			t.Fatal("tampered file accepted")
		}
		if e = os.Remove(path); e != nil {
			t.Fatal(e)
		}
		if _, e = f.service.Load(context.Background(), q, result.Facts.FactsID); e == nil {
			t.Fatal("missing file accepted")
		}
	})
}
func TestIntegrationCancellationAndFactsExpiry(t *testing.T) {
	f := newFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := f.service.Read(ctx, f.request()); e == nil || f.requests.Load() != 0 {
		t.Fatal("cancelled start")
	}
	f.clockOffset.Store(int64(11 * time.Second))
	if status, _ := f.call(f.request()); status == 200 || f.requests.Load() != 0 {
		t.Fatal("deadline ignored")
	}
}
func TestIntegrationEpochDatabaseRevocationAndValidityBounds(t *testing.T) {
	for _, name := range []string{"epoch", "db-revocation", "oldest-expired", "validity-cap"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, func(f *fixture) {
				f.after = func(path string) {
					if path != "/v1/account" {
						return
					}
					switch name {
					case "epoch":
						f.authority.mu.Lock()
						b := f.authority.bindings[ws]
						b.Epoch++
						f.authority.bindings[ws] = b
						f.authority.mu.Unlock()
					case "db-revocation":
						c, e := pgx.Connect(context.Background(), os.Getenv("HIT117_MIGRATOR_DSN"))
						if e != nil {
							t.Error("connect")
							return
						}
						defer c.Close(context.Background())
						_, e = c.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,false)`, ws)
						if e == nil {
							_, e = c.Exec(context.Background(), `UPDATE hp_intake.source_connections SET health='REVOKED',revoked_at=now() WHERE workspace_id=$1 AND connection_id=$2`, ws, conn)
						}
						if e != nil {
							t.Error("revoke")
						}
					case "oldest-expired":
						f.clockOffset.Store(int64(11 * time.Second))
					}
				}
			})
			q := f.request()
			q.DeadlineAt = time.Now().Add(time.Minute)
			status, b := f.call(q)
			if name == "validity-cap" {
				v, e := sf.Decode(b)
				if status != 200 || e != nil || v.ValidUntil.After(v.OldestObservedAt.Add(10*time.Second)) || v.ValidUntil.Before(v.OldestObservedAt.Add(10*time.Second-time.Microsecond)) {
					t.Fatal("validity not oldest+10", status, e)
				}
			} else if status == 200 {
				t.Fatal("revocation/expiry accepted")
			}
			if (name == "epoch" || name == "db-revocation") && f.requests.Load() != 1 {
				t.Fatal("network after context changed")
			}
		})
	}
}
