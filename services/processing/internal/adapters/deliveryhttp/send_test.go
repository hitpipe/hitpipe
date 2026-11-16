package deliveryhttp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestActualTLSExactBytesAndNoRedirect(t *testing.T) {
	p := testfixtures.NewPKI(t)
	body, c := valid(t)
	var requests atomic.Int32
	s := p.Server(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		b, _ := io.ReadAll(r.Body)
		if string(b) != string(body) || r.Method != "POST" || r.URL.Path != "/internal/v1/delivery-intents" || r.Header.Get("X-Request-ID") != c.MessageID {
			t.Error("request changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]string{"message_id": c.MessageID, "disposition": "APPLIED"})
	}, "delivery.test")
	sender, e := New(s.URL, p.Transport(t), 4096, testfixtures.Clock{})
	if e != nil {
		t.Fatal(e)
	}
	m := dispatchintent.Message{Body: body}
	gate := &testfixtures.Gate{Workspace: c.WorkspaceID, Epoch: c.Epoch}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, e := sender.Send(ctx, c, m, gate)
	if e != nil || r.Disposition != "APPLIED" || requests.Load() != 1 {
		t.Fatal(r, e)
	}
	redirect := p.Server(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Location", s.URL); w.WriteHeader(307) }, "delivery.test")
	sender, e = New(redirect.URL, p.Transport(t), 4096, testfixtures.Clock{})
	if e != nil {
		t.Fatal(e)
	}
	r, e = sender.Send(ctx, c, m, gate)
	if e != nil || r.HTTPStatus != 307 || requests.Load() != 1 {
		t.Fatal("redirect followed", r, e)
	}
}
func TestTLSAndGateRejectionBeforeHandler(t *testing.T) {
	for _, mode := range []string{"foreign_ca", "wrong_service", "expired_client", "missing_client", "wrong_server", "closed_gate", "no_gate_call"} {
		t.Run(mode, func(t *testing.T) {
			p := testfixtures.NewPKI(t)
			var n atomic.Int32
			serverName := "delivery.test"
			if mode == "wrong_server" {
				serverName = "other.test"
			}
			s := p.Server(t, func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.WriteHeader(202) }, serverName)
			tr := p.Transport(t)
			switch mode {
			case "foreign_ca":
				tr = testfixtures.NewPKI(t).Transport(t)
			case "wrong_service":
				tr.TLSClientConfig.Certificates = []tls.Certificate{p.Leaf(t, "iam.test", false, false)}
			case "expired_client":
				tr.TLSClientConfig.Certificates = []tls.Certificate{p.Leaf(t, "processing.test", false, true)}
			case "missing_client":
				tr.TLSClientConfig.Certificates = nil
			}
			sender, e := New(s.URL, tr, 4096, testfixtures.Clock{})
			if mode == "missing_client" {
				if e == nil {
					t.Fatal("missing cert config accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			b, c := valid(t)
			g := &testfixtures.Gate{Workspace: c.WorkspaceID, Epoch: c.Epoch, Closed: mode == "closed_gate", NoInvoke: mode == "no_gate_call"}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, e = sender.Send(ctx, c, dispatchintent.Message{Body: b}, g); e == nil {
				t.Fatal("invalid transport/gate accepted")
			}
			if n.Load() != 0 {
				t.Fatal("handler invoked")
			}
		})
	}
}
func TestReceiptStatusContentAndRetryAfter(t *testing.T) {
	for _, mode := range []string{"empty202", "bad_id", "bad_json", "oversized", "wrong_type", "retry_seconds", "retry_date"} {
		t.Run(mode, func(t *testing.T) {
			p := testfixtures.NewPKI(t)
			s := p.Server(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if mode == "retry_seconds" {
					w.Header().Set("Retry-After", "2")
					w.WriteHeader(429)
					return
				}
				if mode == "retry_date" {
					w.Header().Set("Retry-After", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat))
					w.WriteHeader(503)
					return
				}
				if mode == "wrong_type" {
					w.Header().Set("Content-Type", "text/plain")
				}
				w.WriteHeader(202)
				switch mode {
				case "empty202":
					return
				case "bad_id":
					io.WriteString(w, `{"message_id":"22222222-2222-4222-8222-222222222222","disposition":"APPLIED"}`)
				case "oversized":
					for range 1000 {
						io.WriteString(w, "xxxxxxxx")
					}
				case "bad_json":
					io.WriteString(w, `{"message_id":`)
				default:
					io.WriteString(w, `{"message_id":"`+testID+`","disposition":"APPLIED"}`)
				}
			}, "delivery.test")
			sender, e := New(s.URL, p.Transport(t), 4096, testfixtures.Clock{})
			if e != nil {
				t.Fatal(e)
			}
			b, c := valid(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, e := sender.Send(ctx, c, dispatchintent.Message{Body: b}, &testfixtures.Gate{Workspace: c.WorkspaceID, Epoch: 7})
			if mode == "retry_seconds" || mode == "retry_date" {
				if e != nil || !r.RetryAfter.After(time.Now()) {
					t.Fatal(r, e)
				}
			} else if e == nil {
				t.Fatal("invalid202 accepted", r)
			}
		})
	}
}
