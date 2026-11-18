// Package stripe owns bounded, read-only HTTPS requests to one approved origin.
// It is deliberately not an SDK with automatic retries, redirects or cached facts.
package stripe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/gowebpki/jcs"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type Bounds struct {
	ResponseBytes int64
	Pages, Items  int
}
type Client struct {
	origin    string
	transport *http.Transport
	keys      app.Credentials
	now       func() time.Time
	bounds    Bounds
}

func unwired(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return r.IsNil()
	}
	return false
}
func New(origin string, t *http.Transport, keys app.Credentials, now func() time.Time, b Bounds) (*Client, error) {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || t == nil || unwired(keys) || now == nil || b.ResponseBytes <= 0 || b.ResponseBytes >= 1<<31 || b.Pages <= 0 || b.Items <= 0 {
		return nil, app.Fail("dependency")
	}
	cfg := t.TLSClientConfig
	if cfg == nil || cfg.InsecureSkipVerify || cfg.RootCAs == nil || cfg.MinVersion < tls.VersionTLS12 || t.Proxy != nil || t.DialTLS != nil || t.DialTLSContext != nil || (cfg.ServerName != "" && cfg.ServerName != u.Hostname()) {
		return nil, app.Fail("transport")
	}
	clone := t.Clone()
	clone.TLSClientConfig = cfg.Clone()
	clone.TLSClientConfig.RootCAs = cfg.RootCAs.Clone()
	clone.TLSClientConfig.ServerName = u.Hostname()
	clone.TLSClientConfig.NextProtos = []string{"http/1.1"}
	clone.DisableKeepAlives = true
	clone.DisableCompression = true
	clone.ForceAttemptHTTP2 = false
	clone.Protocols = new(http.Protocols)
	clone.Protocols.SetHTTP1(true)
	return &Client{u.Scheme + "://" + u.Host, clone, keys, now, b}, nil
}

type object map[string]json.RawMessage

func parse(b []byte) (object, error) {
	if !utf8.Valid(b) || !json.Valid(b) {
		return nil, app.Fail("provider_message")
	}
	if _, e := jcs.Transform(b); e != nil {
		return nil, app.Fail("provider_message")
	}
	var m object
	if json.Unmarshal(b, &m) != nil || m == nil {
		return nil, app.Fail("provider_message")
	}
	return m, nil
}
func text(m object, k string) *string {
	var s string
	b, ok := m[k]
	if !ok || bytes.Equal(bytes.TrimSpace(b), []byte("null")) || json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}
func boolean(m object, k string) *bool {
	var v bool
	b, ok := m[k]
	if !ok || bytes.Equal(bytes.TrimSpace(b), []byte("null")) || json.Unmarshal(b, &v) != nil {
		return nil
	}
	return &v
}
func number(m object, k string) *int64 {
	b, ok := m[k]
	if !ok {
		return nil
	}
	v, ok := sf.Integer(b)
	if !ok {
		return nil
	}
	return &v
}
func eq(m object, k, want string) bool { v := text(m, k); return v != nil && *v == want }
func mode(m object, b app.Binding) bool {
	v := boolean(m, "livemode")
	return v != nil && b.LiveMode != nil && *v == *b.LiveMode
}
func identity(m object, id, kind string, b app.Binding) bool {
	return eq(m, "id", id) && eq(m, "object", kind) && mode(m, b)
}
func ref(m object, k string) string {
	if s := text(m, k); s != nil && sf.Opaque(*s) && *s != "." && *s != ".." {
		return *s
	}
	var v object
	if json.Unmarshal(m[k], &v) == nil {
		if s := text(v, "id"); s != nil && sf.Opaque(*s) && *s != "." && *s != ".." {
			return *s
		}
	}
	return ""
}
func part(s string) string { return url.PathEscape(s) }
func (c *Client) get(ctx context.Context, b app.Binding, g app.Guard, path string, query url.Values) (object, error) {
	if e := g.Check(ctx); e != nil {
		return nil, e
	}
	key, e := c.keys.ReadKey(ctx, b.Clone())
	if e != nil || len(key) == 0 || len(key) > 4096 || strings.ContainsAny(string(key), "\r\n") {
		return nil, app.Fail("credential")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	address := c.origin + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if e != nil {
		return nil, app.Fail("provider_message")
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Stripe-Account", b.AccountID)
	req.Header.Set("Stripe-Version", b.APIVersion)
	req.Header.Set("Accept", "application/json")
	t := c.transport.Clone()
	t.DialContext = func(dctx context.Context, network, address string) (net.Conn, error) {
		var conn net.Conn
		e := g.Start(ctx, func() error {
			var e error
			dial := c.transport.DialContext
			if dial == nil {
				dial = (&net.Dialer{}).DialContext
			}
			conn, e = dial(dctx, network, address)
			return e
		})
		if e != nil {
			if conn != nil {
				conn.Close()
			}
			return nil, e
		}
		return &guardedConn{conn, ctx, g}, nil
	}
	defer t.CloseIdleConnections()
	resp, e := t.RoundTrip(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, app.Fail("cancelled")
		}
		if code := app.Code(e); code != "unavailable" {
			return nil, app.Fail(code)
		}
		return nil, app.Fail("provider_transport")
	}
	if resp.Body == nil {
		return nil, app.Fail("provider_message")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return nil, app.Fail("rate_limited")
	}
	if resp.StatusCode != 200 {
		return nil, app.Fail("provider_status")
	}
	media, _, e := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if e != nil || media != "application/json" || resp.Header.Get("Content-Encoding") != "" || resp.ContentLength > c.bounds.ResponseBytes {
		return nil, app.Fail("provider_message")
	}
	if v := resp.Header.Get("Stripe-Version"); v != "" && v != b.APIVersion {
		return nil, app.Fail("provider_message")
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, c.bounds.ResponseBytes+1))
	if e != nil || int64(len(raw)) > c.bounds.ResponseBytes || ctx.Err() != nil {
		return nil, app.Fail("provider_message")
	}
	return parse(raw)
}

type guardedConn struct {
	net.Conn
	ctx  context.Context
	gate app.Guard
}

func (c *guardedConn) Write(b []byte) (int, error) {
	n := 0
	e := c.gate.Start(c.ctx, func() error {
		deadline := time.Now().Add(2 * time.Second)
		if d, ok := c.ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if e := c.Conn.SetWriteDeadline(deadline); e != nil {
			return e
		}
		var e error
		n, e = c.Conn.Write(b)
		return e
	})
	return n, e
}

// list never trusts a provider-supplied next URL. It constructs the same path
// and filter with starting_after, rejects loops/duplicate IDs, and stops at the
// explicitly supplied local resource bound. Reaching a bound is not completeness.
func (c *Client) list(ctx context.Context, b app.Binding, g app.Guard, path, kind string, query url.Values) ([]object, bool, error) {
	var all []object
	seen := map[string]bool{}
	for page := 0; page < c.bounds.Pages; page++ {
		v := url.Values{}
		for k, values := range query {
			v[k] = append([]string(nil), values...)
		}
		v.Set("limit", "100")
		if len(all) > 0 {
			v.Set("starting_after", ref(all[len(all)-1], "id"))
		}
		m, e := c.get(ctx, b, g, path, v)
		if e != nil {
			return nil, false, e
		}
		more := boolean(m, "has_more")
		if !eq(m, "object", "list") || more == nil {
			return nil, false, app.Fail("provider_message")
		}
		var data []json.RawMessage
		if string(m["data"]) == "null" || json.Unmarshal(m["data"], &data) != nil {
			return nil, false, app.Fail("provider_message")
		}
		if len(data) > 100 || (*more && len(data) == 0) {
			return nil, false, app.Fail("provider_message")
		}
		for _, raw := range data {
			o, e := parse(raw)
			id := ref(o, "id")
			if e != nil || !eq(o, "object", kind) || id == "" || seen[id] {
				return nil, false, app.Fail("provider_message")
			}
			if len(all) >= c.bounds.Items {
				return all, false, nil
			}
			seen[id] = true
			all = append(all, o)
		}
		if !*more {
			return all, true, nil
		}
	}
	return all, false, nil
}
func supported(v *string, values ...string) *string {
	if v == nil {
		return nil
	}
	for _, s := range values {
		if *v == s {
			return v
		}
	}
	u := "UNKNOWN"
	return &u
}
func currency(m object, k string) *string {
	v := text(m, k)
	if v == nil || len(*v) != 3 {
		return nil
	}
	for _, r := range *v {
		if r < 'a' || r > 'z' {
			return nil
		}
	}
	return v
}
func complete(f sf.Facts) bool {
	return f.SessionCreatedAt != nil && f.Mode != nil && *f.Mode != "UNKNOWN" && f.PaymentStatus != nil && *f.PaymentStatus != "UNKNOWN" && f.Currency != nil && f.AmountTotal != nil && f.LineItemsComplete && f.LineItemCount != nil && f.PriceID != nil && f.Quantity != nil && f.PaymentIntentStatus != "UNKNOWN" && f.AmountReceived != nil && f.ChargeSucceeded != nil && f.ChargeCaptured != nil && f.AmountCaptured != nil && f.AmountRefunded != nil && f.Disputed != nil && f.AllCurrenciesEqual != nil
}
func (c *Client) Read(ctx context.Context, q sf.Request, b app.Binding, g app.Guard) (sf.Facts, error) {
	if unwired(g) || b.LiveMode == nil || !b.Ready {
		return sf.Facts{}, app.Fail("dependency")
	}
	f := sf.Facts{PaymentIntentStatus: "UNKNOWN", OldestObservedAt: c.now().UTC().Truncate(time.Microsecond)}
	var currencies []*string
	linked := true
	finish := func() (sf.Facts, error) {
		all := true
		known := len(currencies) > 0
		for _, v := range currencies {
			if v == nil {
				known = false
			} else if f.Currency != nil && *v != *f.Currency {
				all = false
			}
		}
		if known {
			f.AllCurrenciesEqual = &all
		}
		f.Complete = linked && complete(f)
		return f, nil
	}
	account, e := c.get(ctx, b, g, "/v1/account", nil)
	if e != nil {
		return f, e
	}
	if !eq(account, "id", b.AccountID) || !eq(account, "object", "account") {
		return f, app.Fail("provider_context")
	}
	session, e := c.get(ctx, b, g, "/v1/checkout/sessions/"+part(q.SessionID), nil)
	if e != nil {
		return f, e
	}
	if !identity(session, q.SessionID, "checkout.session", b) {
		return f, app.Fail("provider_context")
	}
	f.Mode = supported(text(session, "mode"), "payment", "subscription", "setup")
	f.PaymentStatus = supported(text(session, "payment_status"), "paid", "unpaid", "no_payment_required")
	f.Currency = currency(session, "currency")
	currencies = append(currencies, f.Currency)
	f.AmountTotal = number(session, "amount_total")
	if created := number(session, "created"); created != nil && *created > 0 && *created <= 253402300799 {
		t := time.Unix(*created, 0).UTC()
		f.SessionCreatedAt = &t
	}
	items, full, e := c.list(ctx, b, g, "/v1/checkout/sessions/"+part(q.SessionID)+"/line_items", "item", nil)
	if e != nil {
		return f, e
	}
	f.LineItemsComplete = full
	if full {
		count := int64(len(items))
		f.LineItemCount = &count
	}
	for _, item := range items {
		currencies = append(currencies, currency(item, "currency"))
	}
	if full && len(items) == 1 {
		f.Quantity = number(items[0], "quantity")
		priceID := ref(items[0], "price")
		if priceID != "" {
			price, e := c.get(ctx, b, g, "/v1/prices/"+part(priceID), nil)
			if e != nil {
				return f, e
			}
			if !identity(price, priceID, "price", b) {
				return f, app.Fail("provider_context")
			}
			currencies = append(currencies, currency(price, "currency"))
			_, hasCustom := price["custom_unit_amount"]
			_, hasRecurring := price["recurring"]
			_, hasTransform := price["transform_quantity"]
			if eq(price, "type", "one_time") && eq(price, "billing_scheme", "per_unit") && hasCustom && string(price["custom_unit_amount"]) == "null" && hasRecurring && string(price["recurring"]) == "null" && hasTransform && string(price["transform_quantity"]) == "null" && number(price, "unit_amount") != nil {
				f.PriceID = &priceID
			}
		}
	}
	piID := ref(session, "payment_intent")
	if piID == "" {
		return finish()
	}
	pi, e := c.get(ctx, b, g, "/v1/payment_intents/"+part(piID), nil)
	if e != nil {
		return f, e
	}
	if !identity(pi, piID, "payment_intent", b) {
		return f, app.Fail("provider_context")
	}
	currencies = append(currencies, currency(pi, "currency"))
	if status := text(pi, "status"); status != nil {
		switch *status {
		case "succeeded":
			f.PaymentIntentStatus = "succeeded"
		case "requires_payment_method", "requires_confirmation", "requires_action", "processing", "requires_capture", "canceled":
			f.PaymentIntentStatus = "other"
		}
	}
	f.AmountReceived = number(pi, "amount_received")
	chID := ref(pi, "latest_charge")
	if chID == "" {
		return finish()
	}
	charge, e := c.get(ctx, b, g, "/v1/charges/"+part(chID), nil)
	if e != nil {
		return f, e
	}
	if !identity(charge, chID, "charge", b) || ref(charge, "payment_intent") != piID {
		return f, app.Fail("provider_context")
	}
	currencies = append(currencies, currency(charge, "currency"))
	paid, status := boolean(charge, "paid"), text(charge, "status")
	if paid != nil && status != nil {
		switch *status {
		case "succeeded", "pending", "failed":
			v := *paid && *status == "succeeded"
			f.ChargeSucceeded = &v
		}
	}
	f.ChargeCaptured = boolean(charge, "captured")
	f.AmountCaptured = number(charge, "amount_captured")
	refunded := number(charge, "amount_refunded")
	disputed := boolean(charge, "disputed")
	refunds, refundFull, e := c.list(ctx, b, g, "/v1/refunds", "refund", url.Values{"charge": {chID}})
	if e != nil {
		return f, e
	}
	sum := int64(0)
	refundKnown := refundFull
	for _, r := range refunds {
		if ref(r, "charge") != chID || (ref(r, "payment_intent") != "" && ref(r, "payment_intent") != piID) {
			return f, app.Fail("provider_context")
		}
		currencies = append(currencies, currency(r, "currency"))
		amount, status := number(r, "amount"), text(r, "status")
		if amount == nil || status == nil {
			refundKnown = false
			continue
		}
		switch *status {
		case "succeeded":
			if sum > 9007199254740991-*amount {
				refundKnown = false
			} else {
				sum += *amount
			}
		case "failed", "canceled":
		case "pending", "requires_action":
			refundKnown = false
		default:
			refundKnown = false
		}
	}
	if refundKnown && refunded != nil && sum == *refunded {
		f.AmountRefunded = refunded
	} else {
		linked = false
	}
	disputes, disputeFull, e := c.list(ctx, b, g, "/v1/disputes", "dispute", url.Values{"charge": {chID}})
	if e != nil {
		return f, e
	}
	for _, d := range disputes {
		if ref(d, "charge") != chID || (ref(d, "payment_intent") != "" && ref(d, "payment_intent") != piID) || !mode(d, b) {
			return f, app.Fail("provider_context")
		}
		currencies = append(currencies, currency(d, "currency"))
	}
	if disputed != nil && *disputed {
		f.Disputed = disputed
		if disputeFull && len(disputes) == 0 {
			linked = false
		}
	} else if len(disputes) > 0 {
		yes := true
		f.Disputed = &yes
		if disputed == nil || !*disputed {
			linked = false
		}
	} else if disputeFull && disputed != nil {
		no := false
		f.Disputed = &no
	}
	return finish()
}
