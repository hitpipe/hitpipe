package authorizeaction

import (
	"bytes"
	"encoding/json"
	"io"
)

// UnmarshalJSON preserves the accepted required/null distinction and rejects
// duplicate/unknown keys. It adds no listener or transport security claim.
func (request *Request) UnmarshalJSON(data []byte) error {
	if request == nil {
		return ErrInvalidRequest
	}
	*request = Request{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalidRequest
	}
	var parsed Request
	seen := make(map[string]bool, 4)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return ErrInvalidRequest
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return ErrInvalidRequest
		}
		seen[name] = true
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return ErrInvalidRequest
		}
		switch name {
		case "workspace_id":
			if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &parsed.WorkspaceID) != nil {
				return ErrInvalidRequest
			}
		case "runtime_id":
			if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &parsed.RuntimeID) != nil {
				return ErrInvalidRequest
			}
		case "action":
			if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &parsed.Action) != nil {
				return ErrInvalidRequest
			}
		case "resource_id":
			if json.Unmarshal(raw, &parsed.ResourceID) != nil {
				return ErrInvalidRequest
			}
		default:
			return ErrInvalidRequest
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return ErrInvalidRequest
	}
	if _, err = decoder.Token(); err != io.EOF {
		return ErrInvalidRequest
	}
	if len(seen) != 4 || !validRequest(parsed) {
		return ErrInvalidRequest
	}
	*request = copyRequest(parsed)
	return nil
}
