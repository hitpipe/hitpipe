package casedeletion

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

type Client struct {
	origin url.URL
	http   *http.Client
}

// NewClient creates the narrow Processing client. Provision origin, CA and the
// publisher's exact leaf fingerprint outside requests. No proxy, redirect or
// caller-chosen destination is used. The network result is never sufficient to
// decide that restored data may open: verify the publisher signature and use
// the authoritative live-generation/revision barrier.
func NewClient(origin string, processingCertificate tls.Certificate, publisherCAs *x509.CertPool, publisherFingerprint string) (*Client, error) {
	where, err := url.Parse(origin)
	if err != nil || where.Scheme != "https" || where.User != nil ||
		where.RawQuery != "" || where.ForceQuery || where.Opaque != "" || strings.Contains(origin, "#") ||
		(where.Path != "" && where.Path != "/") || where.RawPath != "" ||
		publisherCAs == nil || len(publisherCAs.Subjects()) == 0 ||
		!runtimeaccess.ValidFingerprint(publisherFingerprint) || len(processingCertificate.Certificate) == 0 || processingCertificate.PrivateKey == nil {
		return nil, errConfiguration
	}
	port, err := strconv.ParseUint(where.Port(), 10, 16)
	if err != nil || port == 0 || (net.ParseIP(where.Hostname()) == nil && !runtimeaccess.ValidDNSName(where.Hostname())) {
		return nil, errConfiguration
	}
	if _, err := x509.ParseCertificate(processingCertificate.Certificate[0]); err != nil {
		return nil, errConfiguration
	}
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		DialContext: dialer.DialContext, TLSHandshakeTimeout: 2 * time.Second,
		ResponseHeaderTimeout: exchangeTimeout, MaxResponseHeaderBytes: maxHeaderBytes,
		DisableKeepAlives: true, DisableCompression: true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
			RootCAs: publisherCAs.Clone(), Certificates: []tls.Certificate{processingCertificate},
			VerifyConnection: func(state tls.ConnectionState) error {
				if fingerprint(currentLeaf(&state)) != publisherFingerprint {
					return errTransport
				}
				return nil
			},
		},
	}
	return &Client{origin: *where, http: &http.Client{
		Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout: exchangeTimeout,
	}}, nil
}

func (c *Client) Snapshot(ctx context.Context, workspace string, generation uint64) (app.Snapshot, error) {
	if c == nil || c.http == nil || ctx == nil || ctx.Err() != nil || !workspaceID(workspace) || !app.ValidVersion(generation) {
		return app.Snapshot{}, errWire
	}
	endpoint := c.origin
	endpoint.Path = SnapshotPath
	endpoint.RawQuery = url.Values{
		"workspace_id": {workspace},
		"generation":   {strconv.FormatUint(generation, 10)},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return app.Snapshot{}, errTransport
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return app.Snapshot{}, errTransport
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !jsonContent(resp.Header) || resp.ContentLength > MaxSnapshotBytes {
		return app.Snapshot{}, errTransport
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSnapshotBytes+1))
	if err != nil || len(data) > MaxSnapshotBytes || checkJSON(data) != nil {
		return app.Snapshot{}, errTransport
	}
	var snapshot app.Snapshot
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&snapshot); err != nil || ctx.Err() != nil || !validSnapshot(snapshot, workspace, generation) {
		return app.Snapshot{}, errTransport
	}
	return snapshot, nil
}

func (c *Client) SubmitReceipt(ctx context.Context, receipt app.Receipt) error {
	if c == nil || c.http == nil || ctx == nil || ctx.Err() != nil || !validReceipt(receipt) {
		return errWire
	}
	body, err := json.Marshal(receipt)
	if err != nil || len(body) > MaxReceiptBytes {
		return errWire
	}
	endpoint := c.origin
	endpoint.Path = ReceiptPath
	// A ReadCloser avoids GetBody, so transport recovery cannot replay the POST
	// implicitly. A caller may submit the same exact receipt by deliberate retry.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return errTransport
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errTransport
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || ctx.Err() != nil {
		return errTransport
	}
	return nil
}
