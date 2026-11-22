package main

import (
	"strings"
	"testing"
)

// An absent boolean field is not proof that Caddy stripped the header. A
// partial or malformed observer must not give a passing verification.
func TestStripeObservationRequiresTransportAndExactBytes(t *testing.T) {
	route := fixtureRoute{
		name: "stripe_webhook", host: appHost, method: "POST",
		path: stripePath, role: "intake", stripe: true,
	}
	no, yes := false, true
	edge, signature, size := "edge", digest([]byte(stripeValue)), len(stripeBody)
	valid := observation{
		Role: "intake", Method: "POST", Path: stripePath, TLSVersion: "TLS1.3", PeerDNS: &edge,
		HitPipeHeaderPresent: &no, AuthorizationPresent: &no, CookiePresent: &no,
		StripeSignatureSHA256: &signature, BodySHA256: digest(stripeBody), BodyBytes: &size,
	}
	if err := compare(route, valid); err != "" {
		t.Fatalf("valid observer outcome rejected: %s", err)
	}
	for name, invalidate := range map[string]func(*observation){
		"deleted header not verified": func(o *observation) { o.HitPipeHeaderPresent = nil },
		"spoofed header survived":     func(o *observation) { o.HitPipeHeaderPresent = &yes },
		"authorization survived":      func(o *observation) { o.AuthorizationPresent = &yes },
		"cookie survived":             func(o *observation) { o.CookiePresent = &yes },
		"changed request bytes":       func(o *observation) { o.BodySHA256 = digest([]byte("other")) },
		"changed signature": func(o *observation) {
			x := digest([]byte("other"))
			o.StripeSignatureSHA256 = &x
		},
		"fallback sent to BFF":    func(o *observation) { o.Role = "bff" },
		"wrong upstream protocol": func(o *observation) { o.TLSVersion = "TLS1.2" },
	} {
		t.Run(name, func(t *testing.T) {
			got := valid
			invalidate(&got)
			if compare(route, got) == "" {
				t.Fatal("inadequate evidence was accepted")
			}
		})
	}
}

func TestFixtureCannotReturnRawContentOrTrailingData(t *testing.T) {
	for _, payload := range []string{
		`{"role":"intake","raw_body":"not allowed"}`,
		`{"role":"intake"}{"role":"bff"}`,
		strings.Repeat(" ", responseLimit+1),
	} {
		if _, err := readObservation(strings.NewReader(payload)); err == nil {
			t.Fatal("out of contract response accepted")
		}
	}
}
