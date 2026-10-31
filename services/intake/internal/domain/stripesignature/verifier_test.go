package stripesignature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

const testSecret = "whsec_test_secret"

func TestSignatureKnownVector(t *testing.T) {
	body := []byte(`{"id":"evt_test_webhook","object":"event"}`)
	header := "t=1700000000,v1=d95c6b7477fbd7e9f90b1b0ef5f9c7ac25abca5382460e0d988c2b2a5b71b990"
	outcome := Verify(body, header, []byte(testSecret), 1700000000)
	if outcome.Status != Verified || outcome.Reason != ReasonVerified {
		t.Fatalf("known vector: %+v", outcome)
	}
}

func TestSignatureRawBytesAndTolerance(t *testing.T) {
	body := []byte{'{', 0, '}', '\n'}
	signature := signFixture(body, testSecret, 1700000000)
	if got := Verify(body, signature, []byte(testSecret), 1700000000); got.Status != Verified {
		t.Fatalf("non-UTF-8 raw bytes: %+v", got)
	}
	if got := Verify([]byte(`{"a":1}`), signature, []byte(testSecret), 1700000000); got.Status != Rejected {
		t.Fatalf("reserialized body must reject: %+v", got)
	}
	if got := Verify([]byte(`{"a":1}`), signFixture([]byte(`{"a":1}`), testSecret, 1700000000), []byte(testSecret), 1700000001); got.Status != Verified {
		t.Fatalf("same bytes should verify: %+v", got)
	}
	invalidUTF8 := []byte{'{', 0xff, 0xc3, 0x28, '}'}
	invalidSignature := signFixture(invalidUTF8, testSecret, 1700000000)
	if got := Verify(invalidUTF8, invalidSignature, []byte(testSecret), 1700000000); got.Status != Verified {
		t.Fatalf("invalid UTF-8 bytes must remain opaque: %+v", got)
	}
	for _, tc := range []struct {
		name string
		now  int64
		pass bool
	}{
		{name: "past299", now: 1700000000 + 299, pass: true},
		{name: "past300", now: 1700000000 + 300, pass: true},
		{name: "past301", now: 1700000000 + 301, pass: false},
		{name: "future299", now: 1700000000 - 299, pass: true},
		{name: "future300", now: 1700000000 - 300, pass: true},
		{name: "future301", now: 1700000000 - 301, pass: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Verify([]byte(`{"a":1}`), signFixture([]byte(`{"a":1}`), testSecret, 1700000000), []byte(testSecret), tc.now)
			if (got.Status == Verified) != tc.pass {
				t.Fatalf("got=%+v want pass=%v", got, tc.pass)
			}
		})
	}
	first := signFixture([]byte(`{"a":1}`), "old_secret", 1700000000)
	second := signFixture([]byte(`{"a":1}`), testSecret, 1700000000)
	header := "t=1700000000,v1=" + first[len("t=1700000000,v1="):] + ",v1=" + second[len("t=1700000000,v1="):]
	if got := Verify([]byte(`{"a":1}`), header, []byte(testSecret), 1700000000); got.Status != Verified {
		t.Fatalf("rotation candidates: %+v", got)
	}
}

func TestReviewTimestampParserRejectsOverflow(t *testing.T) {
	if parsed, ok := parseUnix("23000000000000000000"); ok || parsed != 0 {
		t.Fatalf("overflow timestamp accepted: parsed=%d ok=%v", parsed, ok)
	}
}

func TestReviewOverflowHeaderReportsMalformed(t *testing.T) {
	body := []byte(`{"a":1}`)
	valid := signFixture(body, testSecret, 1700000000)
	mac := valid[len("t=1700000000,v1="):]
	got := Verify(body, "t=23000000000000000000,v1="+mac, []byte(testSecret), 1700000000)
	if got.Status != Rejected || got.Reason != ReasonTimestampMalformed {
		t.Fatalf("overflow header: %+v", got)
	}
}

func TestReviewAbsoluteDistanceCannotWrap(t *testing.T) {
	body := []byte(`{"a":1}`)
	timestamp := int64(^uint64(0) >> 1)
	header := signFixture(body, testSecret, timestamp)
	got := Verify(body, header, []byte(testSecret), -1<<63)
	if got.Status != Rejected || got.Reason != ReasonClockUnavailable {
		t.Fatalf("invalid MinInt64 clock must fail closed: %+v", got)
	}
	got = Verify(body, header, []byte(testSecret), 1)
	if got.Status != Rejected || got.Reason != ReasonTimestampStale {
		t.Fatalf("MaxInt64 timestamp must be stale, not verified: %+v", got)
	}
}

func TestSignatureRejectsMalformedHeaders(t *testing.T) {
	body := []byte(`{"a":1}`)
	valid := signFixture(body, testSecret, 1700000000)
	for _, tc := range []struct {
		name   string
		header string
		reason Reason
	}{
		{name: "missing timestamp", header: "v1=" + valid[len("t=1700000000,v1="):], reason: ReasonTimestampMissing},
		{name: "missing v1", header: "t=1700000000", reason: ReasonV1Missing},
		{name: "empty v1", header: "t=1700000000,v1=", reason: ReasonV1Malformed},
		{name: "conflicting timestamp", header: valid + ",t=1700000001", reason: ReasonTimestampConflict},
		{name: "v0 only", header: "t=1700000000,v0=" + valid[len("t=1700000000,v1="):], reason: ReasonV1Missing},
		{name: "unknown key", header: valid + ",x=1", reason: ReasonUnsupportedHeaderKey},
		{name: "malformed token", header: valid + ",broken", reason: ReasonHeaderMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Verify(body, tc.header, []byte(testSecret), 1700000000)
			if got.Status != Rejected || got.Reason != tc.reason {
				t.Fatalf("got=%+v want reason=%s", got, tc.reason)
			}
		})
	}
}

func TestSignatureFailsClosedAndSafe(t *testing.T) {
	body := []byte(`{"a":1}`)
	valid := signFixture(body, testSecret, 1700000000)
	checks := []struct {
		name   string
		body   []byte
		header string
		secret []byte
		now    int64
		reason Reason
	}{
		{name: "empty body", body: nil, header: valid, secret: []byte(testSecret), now: 1700000000, reason: ReasonEmptyBody},
		{name: "missing secret", body: body, header: valid, secret: nil, now: 1700000000, reason: ReasonSecretUnavailable},
		{name: "missing clock", body: body, header: valid, secret: []byte(testSecret), now: 0, reason: ReasonClockUnavailable},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			got := Verify(tc.body, tc.header, tc.secret, tc.now)
			if got.Status != Rejected || got.Reason != tc.reason || got.Version != Version {
				t.Fatalf("got=%+v", got)
			}
		})
	}
}

func signFixture(body []byte, secret string, timestamp int64) string {
	message := []byte(formatUnix(timestamp) + ".")
	message = append(message, body...)
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write(message)
	return "t=" + formatUnix(timestamp) + ",v1=" + hex.EncodeToString(hash.Sum(nil))
}
