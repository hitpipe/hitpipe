//go:build delivery_fixture

package dispatch

import (
	"crypto/x509"
	"net"
	"net/url"
)

// FixtureClickUp is only present in the verification build and cannot dial
// the public provider or a hostname subject to DNS.
func FixtureClickUp(origin string, trust *x509.CertPool) (*ClickUp, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.Port() == "" || trust == nil ||
		!net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, ErrUnwired
	}
	return clickUpAt(*u, trust), nil
}
