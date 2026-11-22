package revocation

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

var errExchange = errors.New("revocation control: exact acknowledgement not established")

type Client struct {
	certificate tls.Certificate
	serverCAs   *x509.CertPool
	targets     map[string]string
}

// NewClient builds the trusted target inventory out of band from any request.
// Each configured target must be an HTTPS origin with an explicit port. The
// runtime registry separately supplies the exact certificate pin for each send.
func NewClient(iamClientCertificate tls.Certificate, runtimeServerCAs *x509.CertPool, targets map[string]string) (*Client, error) {
	if !validCertificateConfig(iamClientCertificate) || !validRoots(runtimeServerCAs) || len(targets) == 0 {
		return nil, errConfiguration
	}
	configured := make(map[string]string, len(targets))
	for id, origin := range targets {
		endpoint, err := url.Parse(origin)
		if !runtimeaccess.ValidUUID(id) || err != nil || endpoint.Scheme != "https" ||
			endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || strings.Contains(origin, "#") ||
			endpoint.Opaque != "" || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawPath != "" {
			return nil, errConfiguration
		}
		host := endpoint.Hostname()
		port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
		if err != nil || port == 0 || (net.ParseIP(host) == nil && !runtimeaccess.ValidDNSName(host)) {
			return nil, errConfiguration
		}
		id = strings.ToLower(id)
		if _, duplicate := configured[id]; duplicate {
			return nil, errConfiguration
		}
		endpoint.Path = Path
		configured[id] = endpoint.String()
	}
	return &Client{certificate: iamClientCertificate, serverCAs: runtimeServerCAs.Clone(), targets: configured}, nil
}

// Send performs one bounded transmission to the exact configured participant.
// Its POST is never redirected, automatically retried, proxied or passed on a
// connection authenticated against another participant. Recovery belongs to the
// application and can retry an identical command to retrieve its cached ACK.
func (c *Client) Send(ctx context.Context, target app.Participant, command startgate.BarrierCommand) (startgate.BarrierAck, error) {
	if c == nil || ctx == nil || ctx.Err() != nil || !validIdentity(target.Identity) ||
		!runtimeaccess.ValidFingerprint(target.CertificateFingerprint) || !validCommand(command) ||
		!strings.EqualFold(target.Identity.RuntimeID, command.TargetRuntimeID) ||
		!strings.EqualFold(target.Identity.BootID, command.BootID) ||
		target.Identity.Generation != command.Generation {
		return startgate.BarrierAck{}, errExchange
	}
	endpoint, ok := c.targets[strings.ToLower(target.Identity.RuntimeID)]
	if !ok {
		return startgate.BarrierAck{}, errExchange
	}
	body, err := json.Marshal(commandValue(command))
	if err != nil {
		return startgate.BarrierAck{}, errExchange
	}
	ctx, cancel := context.WithTimeout(ctx, maxExchangeDuration)
	defer cancel()
	// Supplying a ReadCloser avoids GetBody. With no reused connection and no
	// GetBody, transport recovery never replays this POST implicitly.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return startgate.BarrierAck{}, errExchange
	}
	req.ContentLength = int64(len(body))
	id := requestID()
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Content-Type", "application/json")
	pin := target.CertificateFingerprint
	transport := &http.Transport{
		// A literal fresh transport has no proxy and no HTTP/2 upgrade.
		DisableKeepAlives: true, DisableCompression: true,
		MaxResponseHeaderBytes: maxBodyBytes,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
			Certificates: []tls.Certificate{c.certificate},
			RootCAs:      c.serverCAs,
			VerifyConnection: func(state tls.ConnectionState) error {
				if !leafMatches(&state, pin) {
					return errCertificate
				}
				return nil
			},
		},
	}
	defer transport.CloseIdleConnections()
	client := http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return startgate.BarrierAck{}, errExchange
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !jsonHeader(resp.Header) || resp.ContentLength > maxBodyBytes {
		return startgate.BarrierAck{}, errExchange
	}
	// HP-IF-010 requires X-Request-ID on the request. The header is optional on
	// success, but an echoed value cannot identify a different request.
	if echoed := resp.Header.Values("X-Request-ID"); len(echoed) != 0 && (len(echoed) != 1 || echoed[0] != id) {
		return startgate.BarrierAck{}, errExchange
	}
	response, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil || len(response) > maxBodyBytes || ctx.Err() != nil {
		return startgate.BarrierAck{}, errExchange
	}
	ack, err := parseAck(response)
	if err != nil || !ackMatches(command, ack) {
		return startgate.BarrierAck{}, errExchange
	}
	return ack, nil
}
