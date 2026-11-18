// Package sourcefacts implements strict pinned wire shape and JCS integrity.
package sourcefacts

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gowebpki/jcs"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed schema.json
var schemas []byte
var ErrShape = errors.New("invalid_source_facts_shape")
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func UUID(s string) bool { return uuid.MatchString(s) }
func Opaque(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) >= 1 && utf8.RuneCountInString(s) <= 255
}
func NewID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrShape
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
func Integer(raw []byte) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !(raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9')) {
		return 0, false
	}
	if len(raw) > 128 {
		return 0, false
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	f, e := strconv.ParseFloat(n.String(), 64)
	if e != nil || f < 0 || f > 9007199254740991 {
		return 0, false
	}
	if f == 0 {
		m, _, _ := strings.Cut(strings.ToLower(n.String()), "e")
		return 0, !strings.ContainsAny(m, "123456789")
	}
	if f < 1 {
		return 0, false
	}
	v, ok := new(big.Rat).SetString(n.String())
	if !ok || !v.IsInt() || !v.Num().IsInt64() {
		return 0, false
	}
	i := v.Num().Int64()
	return i, i >= 0 && i <= 9007199254740991
}
func strict(name string, b []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(b) || !json.Valid(b) {
		return nil, ErrShape
	}
	if _, e := jcs.Transform(b); e != nil {
		return nil, ErrShape
	}
	var all map[string]struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(schemas, &all) != nil {
		return nil, ErrShape
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) != nil || len(fields) != len(all[name].Properties) {
		return nil, ErrShape
	}
	for k, raw := range fields {
		rule, ok := all[name].Properties[k]
		if !ok || !valid(raw, rule) {
			return nil, ErrShape
		}
	}
	return fields, nil
}
func valid(raw, rule json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	var r struct {
		Type    string            `json:"type"`
		Format  string            `json:"format"`
		Pattern string            `json:"pattern"`
		Min     int               `json:"minLength"`
		Max     int               `json:"maxLength"`
		Minimum int64             `json:"minimum"`
		Maximum int64             `json:"maximum"`
		Enum    []string          `json:"enum"`
		Any     []json.RawMessage `json:"anyOf"`
	}
	if json.Unmarshal(rule, &r) != nil {
		return false
	}
	if len(r.Any) > 0 {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return true
		}
		return valid(raw, r.Any[0])
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	switch r.Type {
	case "boolean":
		return string(raw) == "true" || string(raw) == "false"
	case "integer":
		v, ok := Integer(raw)
		return ok && v >= r.Minimum && (r.Maximum == 0 || v <= r.Maximum)
	case "string":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return false
		}
		n := utf8.RuneCountInString(s)
		if n < r.Min || (r.Max > 0 && n > r.Max) {
			return false
		}
		if len(r.Enum) > 0 {
			ok := false
			for _, v := range r.Enum {
				if s == v {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		if r.Pattern != "" && !regexp.MustCompile(r.Pattern).MatchString(s) {
			return false
		}
		if r.Format == "uuid" && !UUID(s) {
			return false
		}
		if r.Format == "date-time" {
			t, e := time.Parse(time.RFC3339Nano, s)
			if e != nil || t.Year() < 1 || t.Year() > 9999 || len(strings.Split(s, "T")[0]) != 10 {
				return false
			}
		}
		return true
	}
	return false
}
func DecodeRequest(b []byte) (Request, error) {
	m, e := strict("SourceReadRequest", b)
	if e != nil {
		return Request{}, e
	}
	v, _ := Integer(m["expected_epoch"])
	m["expected_epoch"], _ = json.Marshal(v)
	b, _ = json.Marshal(m)
	var q Request
	if json.Unmarshal(b, &q) != nil {
		return Request{}, ErrShape
	}
	return q, nil
}

// FactsDigest is SHA-256 over RFC8785 JCS of the full projection, excluding
// its self-referential digest field. The stored byte digest is separate.
func FactsDigest(f Facts) (string, error) {
	b, e := json.Marshal(f)
	if e != nil {
		return "", ErrShape
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return "", ErrShape
	}
	delete(m, "digest")
	b, e = json.Marshal(m)
	if e != nil {
		return "", ErrShape
	}
	b, e = jcs.Transform(b)
	if e != nil {
		return "", ErrShape
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
func Encode(f Facts) ([]byte, error) {
	d, e := FactsDigest(f)
	if e != nil || d != f.Digest {
		return nil, ErrShape
	}
	b, e := json.Marshal(f)
	if e != nil {
		return nil, ErrShape
	}
	if _, e = strict("SourceFacts", b); e != nil {
		return nil, e
	}
	return jcs.Transform(b)
}
func Decode(b []byte) (Facts, error) {
	if _, e := strict("SourceFacts", b); e != nil {
		return Facts{}, e
	}
	var f Facts
	if json.Unmarshal(b, &f) != nil {
		return Facts{}, ErrShape
	}
	d, e := FactsDigest(f)
	if e != nil || d != f.Digest || f.CompletedAt.Before(f.OldestObservedAt) || f.ValidUntil.After(f.OldestObservedAt.Add(10*time.Second)) {
		return Facts{}, ErrShape
	}
	return f, nil
}
