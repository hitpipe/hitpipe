package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	demoWebhookRoute  = "/webhooks/stripe/demo"
	demoStripeVersion = "2026-09-30.endive"
)

type demoBinding struct {
	SourceAccount        string `json:"sourceAccount"`
	SourcePrice          string `json:"sourcePrice"`
	DestinationWorkspace string `json:"destinationWorkspace"`
	TargetList           string `json:"targetList"`
	Assignee             string `json:"assignee"`
	Label                string `json:"label"`
}

type demoProviders struct {
	demoBinding
	signingSecret []byte
	egress        *http.Client
}

var (
	demoAccount = regexp.MustCompile(`^acct_[A-Za-z0-9]{8,64}$`)
	demoPrice   = regexp.MustCompile(`^price_[A-Za-z0-9]{8,64}$`)
	demoNumeric = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	demoEvent   = regexp.MustCompile(`^evt_[A-Za-z0-9]{8,80}$`)
	demoSecret  = regexp.MustCompile(`^whsec_[A-Za-z0-9_-]{16,240}$`)
)

func demoRead(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 8192 {
		return nil, errors.New("invalid private provider demo file")
	}
	return os.ReadFile(path)
}

func loadDemoProviders(state, share string) (*demoProviders, error) {
	raw, err := demoRead(filepath.Join(state, "provider-demo.json"))
	if os.IsNotExist(err) {
		return &demoProviders{demoBinding: demoBinding{stripeAccount, "price_fixed", clickUpWorkspace, clickUpList, clickUpAssignee, label}}, nil
	}
	if err != nil {
		return nil, err
	}
	var binding demoBinding
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&binding) != nil || decoder.Decode(new(any)) != io.EOF ||
		!demoAccount.MatchString(binding.SourceAccount) || !demoPrice.MatchString(binding.SourcePrice) ||
		!demoNumeric.MatchString(binding.DestinationWorkspace) || !demoNumeric.MatchString(binding.TargetList) ||
		!demoNumeric.MatchString(binding.Assignee) || strings.TrimSpace(binding.Label) != binding.Label ||
		len(binding.Label) == 0 || len([]rune(binding.Label)) > 120 {
		return nil, errors.New("invalid provider demo binding")
	}
	key, err := demoRead(filepath.Join(state, "provider-webhook.secret"))
	if err != nil || !demoSecret.Match(bytes.TrimSpace(key)) {
		return nil, errors.New("missing or invalid Stripe Sandbox webhook signing secret")
	}
	socket := filepath.Join(share, "provider-egress.sock")
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return &demoProviders{
		demoBinding: binding, signingSecret: bytes.TrimSpace(key),
		egress: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}, Transport: &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			info, err := os.Lstat(socket)
			if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 {
				return nil, errors.New("provider boundary socket is unavailable")
			}
			return dialer.DialContext(ctx, "unix", socket)
		}}},
	}, nil
}

func (p *demoProviders) external() bool { return p.egress != nil }

func (p *demoProviders) attest(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://egress.invalid/binding", nil)
	if err != nil {
		return err
	}
	res, err := p.egress.Do(req)
	if err != nil {
		return errors.New("provider egress has not verified Sandbox and ClickUp")
	}
	defer res.Body.Close()
	var binding demoBinding
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&binding) != nil ||
		binding != p.demoBinding {
		return errors.New("provider egress binding differs from the selected workspace")
	}
	return nil
}

func (p *demoProviders) proxy(provider string, auth func(*http.Request) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<10))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method,
			"http://egress.invalid/"+provider+r.URL.RequestURI(), bytes.NewReader(body))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := p.egress.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
		if err != nil || len(raw) > 1<<20 || res.StatusCode >= 300 && res.StatusCode < 400 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		for _, name := range []string{"Content-Type", "Stripe-Version", "Retry-After"} {
			if value := res.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(raw)
	}
}

func matchesDemoSignature(header string, contents, key []byte, now time.Time) bool {
	timer := ""
	var signatures [][]byte
	for _, part := range strings.Split(header, ",") {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			return false
		}
		switch strings.TrimSpace(name) {
		case "t":
			if timer != "" {
				return false
			}
			timer = value
		case "v1":
			decoded, err := hex.DecodeString(value)
			if err == nil && len(decoded) == sha256.Size {
				signatures = append(signatures, decoded)
			}
		}
	}
	seconds, err := strconv.ParseInt(timer, 10, 64)
	if err != nil || seconds < now.Unix()-300 || seconds > now.Unix()+300 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(timer + "."))
	mac.Write(contents)
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		if hmac.Equal(expected, signature) {
			return true
		}
	}
	return false
}

func (s *backend) serveDemoWebhook(ctx context.Context) error {
	path := filepath.Join(s.share, "stripe-webhook.sock")
	if err := resetSocket(path); err != nil {
		return err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
		listener.Close()
		return err
	}
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 6 * time.Second,
		WriteTimeout: 15 * time.Second, Handler: http.HandlerFunc(s.forwardDemoWebhook)}
	context.AfterFunc(ctx, func() { server.Close() })
	go server.Serve(listener)
	return nil
}

func (s *backend) forwardDemoWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RequestURI() != demoWebhookRoute {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	headers := r.Header.Values("Stripe-Signature")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil || len(headers) != 1 || len(headers[0]) > 2048 ||
		!matchesDemoSignature(headers[0], body, s.demo.signingSecret, time.Now()) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var event struct {
		ID       string `json:"id"`
		Account  string `json:"account"`
		LiveMode *bool  `json:"livemode"`
	}
	if json.Unmarshal(body, &event) != nil || !demoEvent.MatchString(event.ID) ||
		event.LiveMode == nil || *event.LiveMode || event.Account != "" && event.Account != s.demo.SourceAccount {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	marker := eventMarker(body)
	if prior, err := os.ReadFile(s.possibleIngress()); err == nil {
		if !bytes.Equal(prior, marker) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	} else if !os.IsNotExist(err) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if s.revokeQueueProof() != nil || writePrivate(s.possibleIngress(), marker) != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"https://"+intakeAddress+demoWebhookRoute, bytes.NewReader(body))
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	request.Header.Set("Stripe-Signature", headers[0])
	request.Header.Set("Content-Type", "application/json")
	res, err := s.certs.client("", "intake.test").Do(request)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode == http.StatusNoContent || res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != 429 {
		if os.Remove(s.possibleIngress()) != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(res.StatusCode)
}
