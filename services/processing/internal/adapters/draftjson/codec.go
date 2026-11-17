package draftjson

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"strings"
)

func Hash(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }
func requestSchema(k drafts.Kind) string {
	switch k {
	case drafts.Save:
		return "DraftWrite"
	case drafts.Check:
		return "CommandMeta"
	case drafts.Rehearse:
		return "RehearsalRequest"
	}
	return ""
}
func responseSchema(k drafts.Kind) string {
	switch k {
	case drafts.Save, drafts.Get:
		return "DraftConfig"
	case drafts.Check:
		return "ReadinessCheck"
	case drafts.Rehearse:
		return "RehearsalResult"
	}
	return ""
}
func KnownSecret(s string) bool {
	s = strings.ToLower(s)
	for _, prefix := range []string{"sk_live_", "sk_test_", "whsec_", "pkcs8", "-----begin", "bearer ", "ghp_", "gho_", "xoxb-", "api_key=", "access_token="} {
		if strings.Contains(s, prefix) {
			return true
		}
	}
	return false
}
func Validate(schema string, b []byte) error {
	v, e := parse(b)
	s, ok := schemas[schema]
	if e != nil || !ok || !validate(v, s) {
		return drafts.ErrInvalid
	}
	return nil
}
func (Codec) Decode(k drafts.Kind, b []byte) (drafts.Input, error) {
	zero := drafts.Input{}
	if Validate(requestSchema(k), b) != nil {
		return zero, drafts.ErrInvalid
	}
	canonical, e := jcs.Transform(b)
	if e != nil {
		return zero, drafts.ErrInvalid
	}
	in := drafts.Input{Digest: Hash(canonical)}
	switch k {
	case drafts.Save:
		var v drafts.DraftWrite
		if json.Unmarshal(canonical, &v) != nil {
			return zero, drafts.ErrInvalid
		}
		in.Command = v.Command
		in.Config = v.Config
		in.DraftID = v.Config.DraftID
		if KnownSecret(v.Config.Name) {
			return zero, drafts.ErrInvalid
		}
		for _, s := range []*string{v.Config.PriceAlias, v.Config.ListAlias, v.Config.AssigneeAlias, v.Config.ServiceLabel} {
			if s != nil && KnownSecret(*s) {
				return zero, drafts.ErrInvalid
			}
		}
	case drafts.Check:
		if json.Unmarshal(canonical, &in.Command) != nil {
			return zero, drafts.ErrInvalid
		}
	case drafts.Rehearse:
		var v drafts.RehearsalRequest
		if json.Unmarshal(canonical, &v) != nil {
			return zero, drafts.ErrInvalid
		}
		in.Command = v.Command
		in.DraftID = v.DraftID
		in.DraftRevision = v.DraftRevision
		in.Examples = v.Examples
		in.ReadinessCheckID = v.ReadinessCheckID
	default:
		return zero, drafts.ErrInvalid
	}
	return in, nil
}
func (Codec) Encode(k drafts.Kind, v any) ([]byte, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, drafts.ErrUnavailable
	}
	b, e = jcs.Transform(b)
	if e != nil || (Codec{}).ValidateResponse(k, b) != nil {
		return nil, drafts.ErrUnavailable
	}
	return b, nil
}
func (Codec) ValidateResponse(k drafts.Kind, b []byte) error {
	if Validate(responseSchema(k), b) != nil {
		return drafts.ErrUnavailable
	}
	if k == drafts.Check {
		var check drafts.ReadinessCheck
		if json.Unmarshal(b, &check) != nil || check.Passed != (check.ActivationPreview != nil) || check.ActivationPreview != nil && !PreviewValid(*check.ActivationPreview) {
			return drafts.ErrUnavailable
		}
	}
	return nil
}
func (Codec) ConfigDigest(c drafts.DraftConfig) (string, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return "", drafts.ErrInvalid
	}
	if Validate("DraftConfig", b) != nil {
		return "", drafts.ErrInvalid
	}
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	delete(m, "name")
	delete(m, "draft_id")
	delete(m, "revision")
	b, _ = json.Marshal(m)
	b, e = jcs.Transform(b)
	if e != nil {
		return "", drafts.ErrInvalid
	}
	return Hash(b), nil
}
