package deliveryhttp

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"io"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Exact unmodified DeliveryIntent/ContextField/Receipt schemas extracted from
// OAS sha256:a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e.
//
//go:embed schemas.json
var schemasJSON []byte
var schemas = func() map[string]map[string]any {
	var s map[string]map[string]any
	if json.Unmarshal(schemasJSON, &s) != nil {
		panic("invalid embedded OAS")
	}
	return s
}()
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Codec struct{}

func parse(b []byte) (any, error) {
	if !utf8.Valid(b) {
		return nil, dispatchintent.ErrHeld
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func(int) (any, error)
	walk = func(depth int) (any, error) {
		if depth > 16 {
			return nil, dispatchintent.ErrHeld
		}
		t, e := d.Token()
		if e != nil {
			return nil, dispatchintent.ErrHeld
		}
		switch t {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, dispatchintent.ErrHeld
				}
				s, ok := k.(string)
				if !ok {
					return nil, dispatchintent.ErrHeld
				}
				if _, yes := m[s]; yes {
					return nil, dispatchintent.ErrHeld
				}
				v, e := walk(depth + 1)
				if e != nil {
					return nil, e
				}
				m[s] = v
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return nil, dispatchintent.ErrHeld
			}
			return m, nil
		case json.Delim('['):
			a := []any{}
			for d.More() {
				v, e := walk(depth + 1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return nil, dispatchintent.ErrHeld
			}
			return a, nil
		}
		switch t.(type) {
		case json.Delim:
			return nil, dispatchintent.ErrHeld
		}
		return t, nil
	}
	v, e := walk(0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, dispatchintent.ErrHeld
	}
	return v, nil
}

// This validator implements exactly the keyword set used by the three pinned
// schemas. Unknown schema keywords fail closed rather than weakening validation.
func validate(v any, s map[string]any) bool {
	for k := range s {
		switch k {
		case "$ref", "type", "description", "additionalProperties", "properties", "required", "minLength", "maxLength", "format", "pattern", "minimum", "maximum", "const", "enum", "items", "maxItems":
		default:
			return false
		}
	}
	if r, ok := s["$ref"].(string); ok {
		prefix := "#/components/schemas/"
		if len(r) <= len(prefix) || r[:len(prefix)] != prefix {
			return false
		}
		sub, ok := schemas[r[len(prefix):]]
		return ok && validate(v, sub)
	}
	if v == nil {
		return false
	}
	if x, ok := s["const"]; ok && v != x {
		return false
	}
	if a, ok := s["enum"].([]any); ok {
		yes := false
		for _, x := range a {
			if x == v {
				yes = true
			}
		}
		if !yes {
			return false
		}
	}
	switch s["type"] {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		ps, ok := s["properties"].(map[string]any)
		if !ok {
			return false
		}
		for k, x := range m {
			sub, yes := ps[k].(map[string]any)
			if !yes || !validate(x, sub) {
				return false
			}
		}
		if a, ok := s["required"].([]any); ok {
			for _, x := range a {
				if _, yes := m[x.(string)]; !yes {
					return false
				}
			}
		}
		return true
	case "array":
		a, ok := v.([]any)
		if !ok {
			return false
		}
		if n, ok := s["maxItems"].(float64); ok && len(a) > int(n) {
			return false
		}
		sub, ok := s["items"].(map[string]any)
		if !ok {
			return false
		}
		for _, x := range a {
			if !validate(x, sub) {
				return false
			}
		}
		return true
	case "string":
		x, ok := v.(string)
		if !ok {
			return false
		}
		n := utf8.RuneCountInString(x)
		if z, ok := s["minLength"].(float64); ok && n < int(z) {
			return false
		}
		if z, ok := s["maxLength"].(float64); ok && n > int(z) {
			return false
		}
		if pattern, ok := s["pattern"].(string); ok {
			r, e := regexp.Compile(pattern)
			if e != nil || !r.MatchString(x) {
				return false
			}
		}
		if f, ok := s["format"]; ok {
			switch f {
			case "uuid":
				return uuid.MatchString(x)
			case "date-time":
				t, e := time.Parse(time.RFC3339Nano, x)
				return e == nil && !t.IsZero()
			default:
				return false
			}
		}
		return true
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		x, e := n.Float64()
		if e != nil || math.IsNaN(x) || math.IsInf(x, 0) || math.Trunc(x) != x {
			return false
		}
		if z, ok := s["minimum"].(float64); ok && x < z {
			return false
		}
		if z, ok := s["maximum"].(float64); ok && x > z {
			return false
		}
		return true
	}
	return false
}
func (Codec) Decode(b []byte, c dispatchintent.Claim, now time.Time) (dispatchintent.Message, error) {
	fail := func() (dispatchintent.Message, error) { return dispatchintent.Message{}, dispatchintent.ErrHeld }
	if now.IsZero() || len(c.MessageID) != 36 || c.MessageID[14] != '4' || !strings.ContainsRune("89ab", rune(c.MessageID[19])) || len(b) < 1 || len(b) > 65536 || len(b) != c.Ref.Bytes || intent.Digest(b) != c.Ref.Digest || c.Ref.Class != "DELIVERY_INTENT" || !now.Before(c.Ref.ExpiresAt) {
		return fail()
	}
	v, e := parse(b)
	if e != nil || !validate(v, schemas["DeliveryIntent"]) {
		return fail()
	}
	m := v.(map[string]any)
	delete(m, "message_digest")
	unsigned, e := json.Marshal(m)
	if e != nil {
		return fail()
	}
	unsigned, e = jcs.Transform(unsigned)
	if e != nil || intent.Digest(unsigned) != c.Digest {
		return fail()
	}
	var p preparecase.DeliveryIntent
	if json.Unmarshal(b, &p) != nil {
		return fail()
	}
	if p.MessageId != c.MessageID || p.MessageDigest != c.Digest || p.WorkspaceId != c.WorkspaceID || p.CaseId != c.CaseID || p.OperationId != c.OperationID || p.ConfigId != c.ConfigID || p.ConfigDigest != c.ConfigDigest || p.LineageId != c.LineageID || p.MessageId != p.LineageId || p.EpisodeId != c.EpisodeID || p.ConnectionId != c.ConnectionID || p.ConnectionEpoch != c.Epoch || !p.DeadlineAt.Equal(c.DeadlineAt) || !now.Before(p.DeadlineAt) || !now.Before(p.FactsValidUntil) {
		return fail()
	}
	values := map[string]string{}
	for _, f := range p.Context {
		if _, yes := values[f.Key]; yes || f.AddedAt.After(now) || !now.Before(f.ExpiresAt) {
			return fail()
		}
		values[f.Key] = f.Value
	}
	raw, _ := json.Marshal(values)
	canonical, e := jcs.Transform(raw)
	if e != nil || len(canonical) > 32768 {
		return fail()
	}
	return dispatchintent.Message{Body: append([]byte(nil), b...), FactsUntil: p.FactsValidUntil, Deadline: p.DeadlineAt}, nil
}
func decodeReceipt(b []byte, id string) (dispatchintent.Receipt, error) {
	v, e := parse(b)
	if e != nil || !validate(v, schemas["Receipt"]) {
		return dispatchintent.Receipt{}, dispatchintent.ErrHeld
	}
	m := v.(map[string]any)
	if m["message_id"] != id {
		return dispatchintent.Receipt{}, dispatchintent.ErrHeld
	}
	return dispatchintent.Receipt{MessageID: id, Disposition: m["disposition"].(string)}, nil
}
