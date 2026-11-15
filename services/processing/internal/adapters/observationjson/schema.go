// Package observationjson validates the exact pinned TaskObservation wire schemas.
package observationjson

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"time"
	"unicode/utf8"
)

//go:embed schemas.json
var schemaJSON []byte
var schemas = func() map[string]map[string]any {
	var s map[string]map[string]any
	if json.Unmarshal(schemaJSON, &s) != nil {
		panic("invalid embedded pinned OAS")
	}
	return s
}()
var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Codec struct{}

func parse(b []byte) (any, error) {
	if !utf8.Valid(b) {
		return nil, observations.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func(int) (any, error)
	walk = func(depth int) (any, error) {
		if depth > 16 {
			return nil, observations.ErrInvalid
		}
		t, e := d.Token()
		if e != nil {
			return nil, observations.ErrInvalid
		}
		switch t {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, observations.ErrInvalid
				}
				s, ok := k.(string)
				if !ok {
					return nil, observations.ErrInvalid
				}
				if _, yes := m[s]; yes {
					return nil, observations.ErrInvalid
				}
				v, e := walk(depth + 1)
				if e != nil {
					return nil, e
				}
				m[s] = v
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return nil, observations.ErrInvalid
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
				return nil, observations.ErrInvalid
			}
			return a, nil
		}
		switch t.(type) {
		case json.Delim:
			return nil, observations.ErrInvalid
		}
		return t, nil
	}
	v, e := walk(0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, observations.ErrInvalid
	}
	return v, nil
}

// This validator implements exactly the keyword set used by the three pinned
// schemas. Unknown schema keywords fail closed rather than weakening validation.
func validate(v any, s map[string]any) bool {
	for k := range s {
		switch k {
		case "$ref", "type", "description", "additionalProperties", "properties", "required", "minLength", "maxLength", "format", "pattern", "minimum", "maximum", "const", "enum", "anyOf", "items", "maxItems", "minItems", "uniqueItems":
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
	if variants, ok := s["anyOf"].([]any); ok {
		for _, variant := range variants {
			if sub, ok := variant.(map[string]any); ok && validate(v, sub) {
				return true
			}
		}
		return false
	}
	if s["type"] == "null" {
		return v == nil
	}
	if v == nil {
		return false
	}
	if x, ok := s["const"]; ok {
		if n, yes := v.(json.Number); yes {
			r, ok := new(big.Rat).SetString(n.String())
			if !ok || !r.IsInt() {
				return false
			}
			want, ok := new(big.Rat).SetString(fmt.Sprint(x))
			if !ok || r.Cmp(want) != 0 {
				return false
			}
		} else if !reflect.DeepEqual(v, x) {
			return false
		}
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
		if n, ok := s["minItems"].(float64); ok && len(a) < int(n) {
			return false
		}
		if unique, ok := s["uniqueItems"].(bool); ok && unique {
			seen := map[string]bool{}
			for _, v := range a {
				raw, _ := json.Marshal(v)
				key := string(raw)
				if seen[key] {
					return false
				}
				seen[key] = true
			}
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
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		r, ok := new(big.Rat).SetString(n.String())
		if !ok || !r.IsInt() {
			return false
		}
		if x, ok := s["minimum"]; ok {
			min, _ := new(big.Rat).SetString(fmt.Sprint(x))
			if min == nil || r.Cmp(min) < 0 {
				return false
			}
		}
		if x, ok := s["maximum"]; ok {
			max, _ := new(big.Rat).SetString(fmt.Sprint(x))
			if max == nil || r.Cmp(max) > 0 {
				return false
			}
		}
		return true
	}
	return false
}
