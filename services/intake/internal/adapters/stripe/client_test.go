package stripe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testKeys struct{}

func (*testKeys) ReadKey(context.Context, app.Binding) ([]byte, error) {
	return []byte("synthetic-only"), nil
}

// Explicit test gate, not a production default or runtime qualification.
type testGate struct{ starts atomic.Int64 }

func (g *testGate) Check(context.Context) error { return nil }
func (g *testGate) Start(ctx context.Context, f func() error) error {
	g.starts.Add(1)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return f()
}
func TestHTTPSRejectsMalformedProviderBodies(t *testing.T) {
	for _, x := range []struct{ name, body, content, encoding string }{{"json", "RAW_PROVIDER_MARKER:{", "application/json", ""}, {"duplicate", `{"id":"a","id":"b"}`, "application/json", ""}, {"utf8", string([]byte{255}), "application/json", ""}, {"content-type", `{}`, "text/plain", ""}, {"compression", `{}`, "application/json", "gzip"}, {"size", strings.Repeat("x", 1025), "application/json", ""}} {
		t.Run(x.name, func(t *testing.T) {
			var requests atomic.Int64
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", x.content)
				if x.encoding != "" {
					w.Header().Set("Content-Encoding", x.encoding)
				}
				w.Write([]byte(x.body))
			}))
			defer s.Close()
			roots := x509.NewCertPool()
			roots.AddCert(s.Certificate())
			transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}
			c, e := New(s.URL, transport, &testKeys{}, time.Now, Bounds{ResponseBytes: 1024, Pages: 1, Items: 100})
			if e != nil {
				t.Fatal(e)
			}
			g := &testGate{}
			b := app.Binding{AccountID: "acct_synthetic", APIVersion: "synthetic-version", Ready: true}
			_, e = c.get(context.Background(), b, g, "/v1/account", nil)
			if e == nil || strings.Contains(e.Error(), "RAW_PROVIDER_MARKER") || requests.Load() != 1 || g.starts.Load() < 2 {
				t.Fatal("malformed data, hidden replay or socket gate failure")
			}
			var keys *testKeys
			if _, e = New(s.URL, transport, keys, time.Now, Bounds{ResponseBytes: 1024, Pages: 1, Items: 100}); e == nil {
				t.Fatal("typed nil credential accepted")
			}
		})
	}
}
