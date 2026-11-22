package privateiam

import (
	"crypto/x509"

	control "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

// ControlHandler serves the barrier on the same gate as IAM authorization.
// Mount it on the separate private control listener with control.ServerTLS.
func (s *Server) ControlHandler(iamClientCAs *x509.CertPool, iamClientFingerprint string) (*control.Handler, error) {
	if s == nil {
		return nil, runtimeaccess.ErrInvalid
	}
	return control.NewHandler(s.gate, iamClientCAs, iamClientFingerprint)
}
