// Package admission validates the durable, already authenticated Intake message.
// It does not establish source authenticity or grant authority to create a case.
package normalizedsignal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
)

var ErrInvalidSignal = admission.ErrInvalidSignal

type Signal = admission.Signal

// Decoder implements the application input port using exact raw validation and RFC8785.
type Decoder struct{}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var fields = map[string]bool{
	"message_id": true, "message_digest": true, "schema_version": true,
	"workspace_id": true, "connection_id": true, "stripe_account_id": true,
	"livemode": true, "session_id": true, "event_id": true, "event_type": true,
	"source_created_at": true, "first_received_at": true, "expires_at": true, "receipt_id": true,
}

// Decode checks raw member names before Go's case-insensitive struct decoder.
// It retains original scalar representations for RFC 8785 canonicalization.
func (Decoder) Decode(raw []byte) (Signal, error) {
	var zero Signal
	if !utf8.Valid(raw) {
		return zero, ErrInvalidSignal
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return zero, ErrInvalidSignal
	}
	members := make(map[string]json.RawMessage, len(fields))
	for d.More() {
		key, e := d.Token()
		if e != nil {
			return zero, ErrInvalidSignal
		}
		name, ok := key.(string)
		if !ok || !fields[name] {
			return zero, ErrInvalidSignal
		}
		if _, exists := members[name]; exists {
			return zero, ErrInvalidSignal
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return zero, ErrInvalidSignal
		}
		// All current fields are strings except livemode. Reject arrays/objects and
		// invalid Unicode even if encoding/json would silently replace a surrogate.
		if name == "livemode" {
			if !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
				return zero, ErrInvalidSignal
			}
		} else {
			var s string
			if json.Unmarshal(value, &s) != nil {
				return zero, ErrInvalidSignal
			}
		}
		members[name] = value
	}
	if _, e = d.Token(); e != nil {
		return zero, ErrInvalidSignal
	}
	if _, e = d.Token(); e != io.EOF || len(members) != len(fields) {
		return zero, ErrInvalidSignal
	}
	// Transform the original input too: it rejects unpaired UTF-16 surrogates.
	if _, e = jcs.Transform(raw); e != nil {
		return zero, ErrInvalidSignal
	}
	if json.Unmarshal(raw, &zero) != nil {
		return Signal{}, ErrInvalidSignal
	}
	for _, id := range []string{zero.MessageID, zero.WorkspaceID, zero.ConnectionID, zero.ReceiptID} {
		if !uuidPattern.MatchString(id) {
			return Signal{}, ErrInvalidSignal
		}
	}
	for _, s := range []string{zero.AccountID, zero.SessionID, zero.EventID} {
		if n := utf8.RuneCountInString(s); n < 1 || n > 255 {
			return Signal{}, ErrInvalidSignal
		}
	}
	if zero.Version != "1.0" || !digestPattern.MatchString(zero.Digest) {
		return Signal{}, ErrInvalidSignal
	}
	switch zero.EventType {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed":
	default:
		return Signal{}, ErrInvalidSignal
	}
	delete(members, "message_digest")
	payload, e := json.Marshal(members)
	if e != nil {
		return Signal{}, ErrInvalidSignal
	}
	canonical, e := jcs.Transform(payload)
	if e != nil {
		return Signal{}, ErrInvalidSignal
	}
	sum := sha256.Sum256(canonical)
	if zero.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
		return Signal{}, ErrInvalidSignal
	}
	return zero, nil
}
