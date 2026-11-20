package privateiam

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

func parseRegistration(data []byte) (runtimeaccess.Registration, error) {
	var result runtimeaccess.Registration
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return result, runtimeaccess.ErrInvalid
	}
	seen := make(map[string]bool, 5)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return runtimeaccess.Registration{}, runtimeaccess.ErrInvalid
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return runtimeaccess.Registration{}, runtimeaccess.ErrInvalid
		}
		seen[name] = true
		var target any
		switch name {
		case "runtime_id":
			target = &result.RuntimeID
		case "service":
			target = &result.Service
		case "generation":
			target = &result.Generation
		case "boot_id":
			target = &result.BootID
		case "certificate_fingerprint":
			target = &result.CertificateFingerprint
		default:
			return runtimeaccess.Registration{}, runtimeaccess.ErrInvalid
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, target) != nil {
			return runtimeaccess.Registration{}, runtimeaccess.ErrInvalid
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return runtimeaccess.Registration{}, runtimeaccess.ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF || len(seen) != 5 || !result.Valid() {
		return runtimeaccess.Registration{}, runtimeaccess.ErrInvalid
	}
	return result, nil
}

func validAuthorization(value authorizeaction.Authorization) bool {
	if value.Epoch <= 0 || value.Epoch > authorizeaction.MaxWireInteger || value.CheckedAt.IsZero() {
		return false
	}
	for _, id := range []string{value.PrincipalID, value.SessionID, value.WorkspaceID} {
		if id != "" && !runtimeaccess.ValidUUID(id) {
			return false
		}
	}
	return value.Role == "" || value.Role == "OWNER" || value.Role == "OPERATOR" || value.Role == "READER"
}

func validSnapshot(value runtimeaccess.Snapshot) bool {
	if !runtimeaccess.ValidUUID(value.RuntimeID) || !runtimeaccess.ValidUUID(value.BootID) || value.Generation <= 0 || value.Generation > authorizeaction.MaxWireInteger || value.Revision <= 0 || value.Revision > authorizeaction.MaxWireInteger || len(value.Denials) > 10000 {
		return false
	}
	if value.Gate != "CLOSED" && value.Gate != "SYNCING" && value.Gate != "OPEN" {
		return false
	}
	for _, denial := range value.Denials {
		if denial.WorkspaceID != nil && !runtimeaccess.ValidUUID(*denial.WorkspaceID) || !runtimeaccess.ValidUUID(denial.SubjectID) || !runtimeaccess.ValidUUID(denial.RevocationID) || denial.Epoch <= 0 || denial.Epoch > authorizeaction.MaxWireInteger || denial.AcceptedAt.IsZero() || denial.EffectiveAt != nil && denial.EffectiveAt.IsZero() {
			return false
		}
		switch denial.Scope {
		case "SESSION", "MEMBERSHIP", "CONNECTION", "WORKSPACE":
		default:
			return false
		}
	}
	return true
}
