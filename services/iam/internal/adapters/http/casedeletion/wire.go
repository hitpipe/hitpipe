package casedeletion

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
)

func status(err error) int {
	switch {
	case errors.Is(err, app.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, app.ErrUnauthorized):
		return http.StatusForbidden
	case errors.Is(err, app.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, app.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusServiceUnavailable
	}
}

// Decode only the exact schema. DisallowUnknownFields alone also permits
// folded field names and repeats keys, so verify keys before assigning values.
func decodeReceipt(data []byte) (app.Receipt, error) {
	var receipt app.Receipt
	f := map[string]any{
		"environment": &receipt.Environment, "workspace_id": &receipt.WorkspaceID,
		"deletion_id": &receipt.DeletionID, "scope_digest": &receipt.ScopeDigest,
		"generation": &receipt.Generation, "revision": &receipt.Revision,
		"applied_at": &receipt.AppliedAt, "application_ref": &receipt.ApplicationRef,
	}
	if !utf8.Valid(data) {
		return app.Receipt{}, errWire
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return app.Receipt{}, errWire
	}
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return app.Receipt{}, errWire
		}
		dst, ok := f[name]
		if !ok {
			return app.Receipt{}, errWire
		}
		delete(f, name)
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, dst) != nil {
			return app.Receipt{}, errWire
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || len(f) != 0 {
		return app.Receipt{}, errWire
	}
	if _, err := d.Token(); err != io.EOF || !validReceipt(receipt) {
		return app.Receipt{}, errWire
	}
	return receipt, nil
}

func validReceipt(receipt app.Receipt) bool { return receipt.Validate() == nil }

// Basic scope/shape before emission and after decoding. The signature remains
// a separately verified app-level proof against the configured publisher key.
func validSnapshot(snapshot app.Snapshot, workspace string, generation uint64) bool {
	p := snapshot.Payload
	return p.SchemaVersion == app.SchemaVersion && app.ValidEnvironment(p.Environment) && app.ValidID(p.PublisherID) &&
		p.WorkspaceID == workspace && p.Generation == generation && app.ValidVersion(p.Revision) &&
		p.Full && p.Decisions != nil && len(p.Decisions) <= app.MaxDecisions && len(snapshot.Signature) == ed25519.SignatureSize
}

// checkJSON verifies one JSON value and rejects duplicate keys at every depth.
// The client still uses the existing app-level signature verifier and live
// revision barrier; the decoder is not a freshness or authority check.
func checkJSON(data []byte) error {
	if !utf8.Valid(data) {
		return errWire
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := value(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errWire
	}
	return nil
}

func value(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errWire
	}
	token, err := d.Token()
	if err != nil {
		return errWire
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]struct{}{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok {
				return errWire
			}
			// The wire schemas use only lower-case ASCII keys. Reject case-
			// folded/Unicode lookalikes before the typed decoder can alias them.
			for _, r := range name {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
					return errWire
				}
			}
			if _, exists := seen[name]; exists {
				return errWire
			}
			seen[name] = struct{}{}
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
		if end, err := d.Token(); err != nil || end != json.Delim('}') {
			return errWire
		}
	case json.Delim('['):
		for d.More() {
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
		if end, err := d.Token(); err != nil || end != json.Delim(']') {
			return errWire
		}
	case json.Delim(']'), json.Delim('}'):
		return errWire
	}
	return nil
}
