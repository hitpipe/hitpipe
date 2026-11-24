package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestProviderDemoRequiresUnmodifiedTimelySignedEvent(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := []byte("{\n  \"id\": \"evt_testfixture\", \"livemode\": false\n}")
	key := []byte("fixture secret")
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%d.", now.Unix())
	mac.Write(body)
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(mac.Sum(nil)))
	if !matchesDemoSignature(header, body, key, now) {
		t.Fatal("exact valid bytes did not verify")
	}
	if matchesDemoSignature(header, append(append([]byte(nil), body...), ' '), key, now) ||
		matchesDemoSignature(header, body, key, now.Add(6*time.Minute)) ||
		matchesDemoSignature(header+",t=1700000000", body, key, now) ||
		matchesDemoSignature(header, body, []byte("wrong secret"), now) {
		t.Fatal("tampered, stale, malformed, or foreign signature accepted")
	}
}
