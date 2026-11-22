// This client qualifies the pinned edge using isolated, synthetic fixtures.
// It never dials a URL from a response and never prints a header or body value.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"time"
)

const (
	appHost       = "app.edge.test"
	idpHost       = "idp.edge.test"
	endpoint      = "edge:443"
	responseLimit = 8 * 1024
	tcpTimeout    = 650 * time.Millisecond
	httpTimeout   = 5 * time.Second
)

const stripeValue = "t=1700000000,v1=06146fd8e65cb8149d1d5f09a6aa3a62183a103299add728af398d16a4df35f2"
const stripePath = "/webhooks/stripe/01900000-0000-7000-8000-000000000141"

var stripeBody = []byte("{\r\n  \"id\": \"synthetic-event\", \"text\": \"перевод\"\r\n}\r\n")

type observation struct {
	Role                  string  `json:"role"`
	Method                string  `json:"method"`
	Path                  string  `json:"path"`
	TLSVersion            string  `json:"tls_version"`
	PeerDNS               *string `json:"peer_dns"`
	HitPipeHeaderPresent  *bool   `json:"hitpipe_header_present"`
	AuthorizationPresent  *bool   `json:"authorization_present"`
	CookiePresent         *bool   `json:"cookie_present"`
	StripeSignatureSHA256 *string `json:"stripe_signature_sha256"`
	BodySHA256            string  `json:"body_sha256"`
	BodyBytes             *int    `json:"body_bytes"`
}

type check struct {
	Name            string `json:"name"`
	OK              bool   `json:"ok"`
	HTTPStatus      int    `json:"http_status,omitempty"`
	Upstream        string `json:"upstream,omitempty"`
	UpstreamTLS     string `json:"upstream_tls,omitempty"`
	Evidence        string `json:"evidence,omitempty"`
	BodySHA256      string `json:"body_sha256,omitempty"`
	SignatureSHA256 string `json:"stripe_signature_sha256,omitempty"`
}

type report struct {
	Phase  string  `json:"phase"`
	Checks []check `json:"checks"`
	Passed bool    `json:"passed"`
}

type fixtureRoute struct {
	name, host, method, path, role string
	stripe                         bool
}

func digest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func fixedClient(caPath string) (*http.Client, error) {
	raw, err := os.ReadFile(caPath)
	if err != nil {
		return nil, errors.New("public fixture CA unreadable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(raw) {
		return nil, errors.New("public fixture CA invalid")
	}
	dial := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxResponseHeaderBytes: responseLimit,
		TLSHandshakeTimeout:    2 * time.Second,
		TLSClientConfig:        &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			switch address {
			case appHost + ":443", idpHost + ":443":
				return dial.DialContext(ctx, network, endpoint)
			default:
				return nil, errors.New("not an isolated edge host")
			}
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   httpTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// query only accepts hard-coded fixture paths. Spoofed names are synthetic
// client inputs and must never survive the edge's trust boundary.
func query(ctx context.Context, client *http.Client, route fixtureRoute) (*http.Response, error) {
	var body io.Reader
	if route.stripe {
		body = bytes.NewReader(stripeBody)
	}
	req, err := http.NewRequestWithContext(ctx, route.method, "https://"+route.host+route.path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-HitPipe-Session", "synthetic-session")
	req.Header.Add("X-HitPipe-Workspace", "synthetic-workspace")
	req.Header.Add("x-hItPiPe-Additional", "synthetic-additional")
	if route.stripe {
		req.Header.Set("Authorization", "Bearer synthetic-not-a-credential")
		req.Header.Set("Cookie", "synthetic_cookie=discard")
		req.Header.Set("Stripe-Signature", stripeValue)
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

func readObservation(body io.Reader) (observation, error) {
	raw, err := io.ReadAll(io.LimitReader(body, responseLimit+1))
	if err != nil || len(raw) > responseLimit {
		return observation{}, errors.New("fixture observation unavailable")
	}
	var o observation
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return observation{}, errors.New("fixture observation invalid")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return observation{}, errors.New("fixture observation has trailing data")
	}
	return o, nil
}

func compare(route fixtureRoute, got observation) string {
	if got.Role != route.role || got.Method != route.method || got.Path != route.path {
		return "unexpected_fixture_role_method_or_path"
	}
	if got.TLSVersion != "TLS1.3" || got.PeerDNS == nil ||
		(route.role == "keycloak" && *got.PeerDNS != "") ||
		(route.role != "keycloak" && *got.PeerDNS != "edge") {
		return "unexpected_upstream_tls_or_client_identity"
	}
	if got.HitPipeHeaderPresent == nil || *got.HitPipeHeaderPresent {
		return "untrusted_header_not_proven_absent"
	}
	if got.AuthorizationPresent == nil || *got.AuthorizationPresent ||
		got.CookiePresent == nil || *got.CookiePresent {
		return "unexpected_authorization_or_cookie"
	}
	var expectedBody []byte
	expectedSignature := ""
	if route.stripe {
		expectedBody, expectedSignature = stripeBody, digest([]byte(stripeValue))
	}
	if got.StripeSignatureSHA256 == nil || *got.StripeSignatureSHA256 != expectedSignature {
		return "unexpected_stripe_signature_digest"
	}
	if got.BodyBytes == nil || *got.BodyBytes != len(expectedBody) || got.BodySHA256 != digest(expectedBody) {
		return "unexpected_body_digest_or_size"
	}
	return ""
}

func positive(ctx context.Context, client *http.Client, route fixtureRoute) check {
	out := check{Name: route.name}
	resp, err := query(ctx, client, route)
	if err != nil {
		out.Evidence = "public_https_request_failed"
		return out
	}
	defer resp.Body.Close()
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		out.Evidence = "unexpected_http_status"
		return out
	}
	got, err := readObservation(resp.Body)
	if err != nil {
		out.Evidence = "fixture_observation_invalid"
		return out
	}
	out.Evidence = compare(route, got)
	if out.Evidence != "" {
		return out
	}
	out.OK, out.Upstream, out.UpstreamTLS = true, route.role, "TLS1.3"
	out.Evidence = "exact_route_and_sanitized_headers"
	if route.stripe {
		out.Evidence = "byte_exact_body_and_signature; no_authorization_cookie_or_hitpipe_headers"
		out.BodySHA256, out.SignatureSHA256 = got.BodySHA256, *got.StripeSignatureSHA256
	} else if route.role == "bff" && route.name == "public_private_path_is_bff" {
		out.Evidence = "bff_catchall; not_direct_forwarding_to_internal_service"
	}
	return out
}

func negativeHTTP(ctx context.Context, client *http.Client, route fixtureRoute, expected int) check {
	out := check{Name: route.name}
	resp, err := query(ctx, client, route)
	if err != nil {
		out.Evidence = "public_https_request_failed"
		return out
	}
	defer resp.Body.Close()
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode != expected {
		out.Evidence = "unexpected_http_status"
		return out
	}
	// A catch-all returning the same HTTP error is not the public denial.
	// No recognizable test upstream response can be present.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil || len(raw) > responseLimit {
		out.Evidence = "invalid_error_response"
		return out
	}
	var received struct {
		Role string `json:"role"`
	}
	if json.Unmarshal(raw, &received) == nil && received.Role != "" {
		out.Evidence = "fixture_response_on_denied_request"
		return out
	}
	out.OK, out.Evidence = true, "edge_rejected; no_fixture_response"
	return out
}

func checkTCP(ctx context.Context, label, host string, port int, shouldConnect bool) check {
	c := check{Name: label + "_" + strconv.Itoa(port)}
	d := net.Dialer{Timeout: tcpTimeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		_ = conn.Close()
	}
	c.OK = (err == nil) == shouldConnect
	if err == nil {
		c.Evidence = "tcp_connected"
	} else {
		c.Evidence = "tcp_connection_not_established"
	}
	return c
}

func isolatedIP(value string) bool {
	ip, err := netip.ParseAddr(value)
	return err == nil && ip.Is4() && ip.IsPrivate()
}

func checkPrivate(ctx context.Context, private string, iam string, shouldConnect bool) []check {
	out := make([]check, 0, 5)
	for _, p := range []int{5432, 8443, 9443, 9444} {
		out = append(out, checkTCP(ctx, "fixture_private", private, p, shouldConnect))
	}
	if iam != "" {
		out = append(out, checkTCP(ctx, "iam_business_listener", iam, 8443, shouldConnect))
	}
	return out
}

func baseline(ctx context.Context, client *http.Client, private, iam string) []check {
	routes := []fixtureRoute{
		{name: "oidc_login", host: appHost, method: http.MethodGet, path: "/auth/login", role: "iam"},
		{name: "oidc_callback", host: appHost, method: http.MethodGet, path: "/auth/callback", role: "iam"},
		{name: "stripe_webhook", host: appHost, method: http.MethodPost, path: stripePath, role: "intake", stripe: true},
		{name: "ui", host: appHost, method: http.MethodGet, path: "/", role: "bff"},
		{name: "public_api", host: appHost, method: http.MethodGet, path: "/api/v1/session", role: "bff"},
		{name: "public_private_path_is_bff", host: appHost, method: http.MethodGet, path: "/internal/v1/runtime/barrier", role: "bff"},
		{name: "public_admin_path_is_bff", host: appHost, method: http.MethodGet, path: "/admin/realms", role: "bff"},
		{name: "public_metrics_path_is_bff", host: appHost, method: http.MethodGet, path: "/metrics", role: "bff"},
		{name: "idp_realms", host: idpHost, method: http.MethodGet, path: "/realms/synthetic/protocol/openid-connect/auth", role: "keycloak"},
		{name: "idp_resources", host: idpHost, method: http.MethodGet, path: "/resources/synthetic/style.css", role: "keycloak"},
		{name: "idp_well_known", host: idpHost, method: http.MethodGet, path: "/.well-known/openid-configuration", role: "keycloak"},
	}
	checks := make([]check, 0, len(routes)+14)
	for _, route := range routes {
		checks = append(checks, positive(ctx, client, route))
	}
	for name, path := range []string{"/admin/realms", "/metrics", "/health", "/internal/v1/runtime/barrier", "/"} {
		route := fixtureRoute{name: []string{"idp_admin", "idp_metrics", "idp_health", "idp_control", "idp_default"}[name], host: idpHost, method: http.MethodGet, path: path}
		checks = append(checks, negativeHTTP(ctx, client, route, http.StatusNotFound))
	}
	for _, port := range []int{80, 2019, 5432, 8443, 9443, 9444} {
		checks = append(checks, checkTCP(ctx, "public_edge", "edge", port, false))
	}
	return append(checks, checkPrivate(ctx, private, iam, false)...)
}

func execute(ctx context.Context, phase, ca, edge, private, iam string) ([]check, error) {
	switch phase {
	case "baseline", "wrong-san", "untrusted-ca", "private-positive", "ready":
	default:
		return nil, errors.New("unknown phase")
	}
	if edge != endpoint {
		return nil, errors.New("edge must be the isolated endpoint edge:443")
	}
	if phase == "baseline" || phase == "private-positive" {
		if !isolatedIP(private) || (iam != "" && (!isolatedIP(iam) || iam == private)) {
			return nil, errors.New("fixture private addresses must be distinct RFC1918 IPv4 addresses")
		}
	}
	if phase == "private-positive" {
		return checkPrivate(ctx, private, iam, true), nil
	}
	client, err := fixedClient(ca)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	if phase == "baseline" {
		return baseline(ctx, client, private, iam), nil
	}
	ui := fixtureRoute{name: "unrelated_bff_control", host: appHost, method: http.MethodGet, path: "/", role: "bff"}
	if phase == "ready" {
		ui.name = "edge_bff_ready"
		return []check{positive(ctx, client, ui)}, nil
	}
	iamRoute := fixtureRoute{name: "iam_with_invalid_upstream_certificate", host: appHost, method: http.MethodGet, path: "/auth/login"}
	return []check{negativeHTTP(ctx, client, iamRoute, http.StatusBadGateway), positive(ctx, client, ui)}, nil
}

func run() int {
	phase := flag.String("phase", "", "baseline, wrong-san, untrusted-ca, private-positive, or ready")
	ca := flag.String("ca", "/tls/public-ca.crt", "synthetic public certificate authority")
	edge := flag.String("edge", endpoint, "isolated edge endpoint")
	private := flag.String("private-ip", "", "private IPv4 address of isolated fixture")
	iam := flag.String("iam-ip", "", "optional private IPv4 address of real IAM fixture")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	checks, err := execute(ctx, *phase, *ca, *edge, *private, *iam)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	out := report{Phase: *phase, Checks: checks, Passed: len(checks) > 0}
	for _, c := range checks {
		out.Passed = out.Passed && c.OK
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return 1
	}
	if !out.Passed {
		return 1
	}
	return 0
}

func main() { os.Exit(run()) }
