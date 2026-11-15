package observationjson

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"regexp"
)

func Digest(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }
func Validate(name string, b []byte) error {
	v, e := parse(b)
	if e != nil || !validate(v, schemas[name]) {
		return observations.ErrInvalid
	}
	return nil
}
func (Codec) Decode(b []byte) (observations.TaskObservation, error) {
	var out observations.TaskObservation
	v, e := parse(b)
	if e != nil || !validate(v, schemas["TaskObservation"]) {
		return out, observations.ErrInvalid
	}
	m := v.(map[string]any)
	digest := m["message_digest"].(string)
	delete(m, "message_digest")
	unsigned, e := json.Marshal(m)
	if e != nil {
		return out, observations.ErrInvalid
	}
	canon, e := jcs.Transform(unsigned)
	if e != nil || Digest(canon) != digest {
		return out, observations.ErrInvalid
	}
	if !regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(m["message_id"].(string)) {
		return out, observations.ErrInvalid
	}
	if json.Unmarshal(b, &out) != nil {
		return out, observations.ErrInvalid
	}
	return out, nil
}
