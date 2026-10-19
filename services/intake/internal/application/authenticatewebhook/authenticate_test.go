package authenticatewebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/fixtures/signingsecrets"
	"github.com/hitpipe/hitpipe/services/intake/internal/domain/stripesignature"
)

const (
	fixtureInstallation = "install_test"
	fixtureSecret       = "whsec_test_secret"
	fixtureNow          = int64(1700000000)
)

type fixedClock struct {
	now int64
	err error
}

func (c fixedClock) NowUnix(context.Context) (int64, error) { return c.now, c.err }

type mapSecrets map[string][]byte

func (m mapSecrets) Resolve(_ context.Context, installationID string) ([]byte, error) {
	secret, ok := m[installationID]
	if !ok {
		return nil, errors.New("not found")
	}
	return secret, nil
}

func TestAuthenticateWebhookContractWithFixtureAndDouble(t *testing.T) {
	body := []byte(`{"id":"evt_test_webhook","object":"event"}`)
	header := sign(body, fixtureSecret, fixtureNow)
	temp := t.TempDir()
	secretPath := filepath.Join(temp, "secrets.json")
	if err := os.WriteFile(secretPath, []byte(`{"install_test":"whsec_test_secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fileResolver, err := signingsecrets.NewFileResolver(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	implementations := []struct {
		name    string
		secrets SecretResolver
		clock   Clock
	}{
		{name: "file-fixture", secrets: fileResolver, clock: fixedClock{now: fixtureNow}},
		{name: "in-memory-double", secrets: mapSecrets{fixtureInstallation: []byte(fixtureSecret)}, clock: fixedClock{now: fixtureNow}},
	}
	for _, implementation := range implementations {
		t.Run(implementation.name+"/normal", func(t *testing.T) {
			consumer, err := New(implementation.secrets, implementation.clock)
			if err != nil {
				t.Fatal(err)
			}
			result := consumer.Authenticate(context.Background(), Input{InstallationID: fixtureInstallation, EventID: "evt_1", RawBody: body, Signature: header})
			if result.Webhook == nil || result.Outcome.Status != stripesignature.Verified {
				t.Fatalf("result=%+v", result)
			}
		})
		t.Run(implementation.name+"/boundary", func(t *testing.T) {
			consumer, err := New(implementation.secrets, fixedClock{now: fixtureNow + 300})
			if err != nil {
				t.Fatal(err)
			}
			result := consumer.Authenticate(context.Background(), Input{InstallationID: fixtureInstallation, EventID: "evt_1", RawBody: body, Signature: header})
			if result.Webhook == nil || result.Outcome.Status != stripesignature.Verified {
				t.Fatalf("boundary result=%+v", result)
			}
		})
		t.Run(implementation.name+"/error", func(t *testing.T) {
			consumer, err := New(implementation.secrets, implementation.clock)
			if err != nil {
				t.Fatal(err)
			}
			altered := append([]byte(nil), body...)
			altered[len(altered)-2] = 'x'
			result := consumer.Authenticate(context.Background(), Input{InstallationID: fixtureInstallation, EventID: "evt_1", RawBody: altered, Signature: header})
			if result.Webhook != nil || result.Outcome.Reason != stripesignature.ReasonSignatureMismatch {
				t.Fatalf("error result=%+v", result)
			}
		})
	}
}

func TestAuthenticateWebhookRejectsRepeatWithoutSignature(t *testing.T) {
	body := []byte(`{"id":"evt_repeat","object":"event"}`)
	consumer, err := New(mapSecrets{fixtureInstallation: []byte(fixtureSecret)}, fixedClock{now: fixtureNow})
	if err != nil {
		t.Fatal(err)
	}
	first := consumer.Authenticate(context.Background(), Input{InstallationID: fixtureInstallation, EventID: "evt_repeat", RawBody: body, Signature: sign(body, fixtureSecret, fixtureNow)})
	second := consumer.Authenticate(context.Background(), Input{InstallationID: fixtureInstallation, EventID: "evt_repeat", RawBody: append(body, '!'), Signature: firstHeader(body)})
	if first.Webhook == nil || second.Webhook != nil || second.Outcome.Status != stripesignature.Rejected {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestAuthenticateWebhookFailsClosedOnDependencyErrors(t *testing.T) {
	secretUnavailable, err := New(mapSecrets{}, fixedClock{now: fixtureNow})
	if err != nil {
		t.Fatal(err)
	}
	result := secretUnavailable.Authenticate(context.Background(), Input{InstallationID: "missing", RawBody: []byte("{}"), Signature: "t=1700000000,v1=00"})
	if result.Outcome.Reason != stripesignature.ReasonSecretUnavailable {
		t.Fatalf("secret result=%+v", result)
	}
	clockUnavailable, err := New(mapSecrets{fixtureInstallation: []byte(fixtureSecret)}, fixedClock{err: errors.New("clock unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	result = clockUnavailable.Authenticate(context.Background(), Input{InstallationID: fixtureInstallation, RawBody: []byte("{}"), Signature: "t=1700000000,v1=00"})
	if result.Outcome.Reason != stripesignature.ReasonClockUnavailable {
		t.Fatalf("clock result=%+v", result)
	}
}

func sign(body []byte, secret string, timestamp int64) string {
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(formatTimestamp(timestamp) + "."))
	_, _ = hash.Write(body)
	return "t=" + formatTimestamp(timestamp) + ",v1=" + hex.EncodeToString(hash.Sum(nil))
}

func firstHeader(body []byte) string { return sign(body, fixtureSecret, fixtureNow) }

func formatTimestamp(timestamp int64) string {
	if timestamp == fixtureNow {
		return "1700000000"
	}
	return "1700000000"
}
