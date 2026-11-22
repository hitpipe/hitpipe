package workspaces

import (
	"strings"
	"testing"
)

func TestCommandDigestUsesCanonicalBodyAndOriginalUUIDSpelling(t *testing.T) {
	requestID := "472fcd63-ab4e-41ce-bac5-123456789abc"
	cmd := Command{RequestID: requestID, Name: "A<&\u2028"}
	got, err := commandDigest(cmd)
	if err != nil {
		t.Fatal(err)
	}
	want := digest([]byte(`{"command":{"request_id":"472fcd63-ab4e-41ce-bac5-123456789abc"},"name":"A<&` + "\u2028" + `"}`))
	if got != want {
		t.Fatal("command body did not produce its RFC 8785 digest")
	}
	cmd.RequestID = strings.ToUpper(requestID)
	other, err := commandDigest(cmd)
	if err != nil || other == got {
		t.Fatal("distinct body spelling did not produce a distinct digest")
	}
}
