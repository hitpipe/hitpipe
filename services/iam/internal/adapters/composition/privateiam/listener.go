package privateiam

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"time"

	transport "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

// Listen serves the private business methods on an explicit private address.
// The caller controls startup with Start; the listener can receive bootstrap
// registration while protected authorization is still closed.
func (s *Server) Listen(ctx context.Context, address string, certificate tls.Certificate, clientCAs *x509.CertPool) error {
	handler, err := transport.New(s)
	if err != nil {
		return err
	}
	return s.listen(ctx, address, certificate, clientCAs, handler)
}

func (s *Server) listen(ctx context.Context, address string, certificate tls.Certificate, clientCAs *x509.CertPool, handler http.Handler) error {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if err != nil || port != "8443" || ip == nil || !(ip.IsPrivate() || ip.IsLoopback()) {
		return runtimeaccess.ErrInvalid
	}
	peers := make([]runtimeaccess.Peer, 0, len(s.peers))
	for peer := range s.peers {
		peers = append(peers, peer)
	}
	config, err := transport.TLSConfig(certificate, clientCAs, peers)
	if err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return runtimeaccess.ErrUnavailable
	}
	defer listener.Close()
	server := &http.Server{
		Handler: handler, TLSConfig: config,
		ReadHeaderTimeout: startBudget, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second,
		MaxHeaderBytes: 8 * 1024,
	}
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	err = server.ServeTLS(listener, "", "")
	if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return runtimeaccess.ErrUnavailable
	}
	return nil
}
