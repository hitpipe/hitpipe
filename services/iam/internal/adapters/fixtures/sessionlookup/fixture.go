// Package sessionlookup provides strictly synthetic diagnostic adapters. It is
// not a production session authority and never accepts cookies or credentials.
package sessionlookup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

const MaxFixtureBytes = 64 * 1024

var errFixture = errors.New("invalid_fixture")
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$`)

// encoding/json matches struct fields without regard to case. The fixture
// contract deliberately requires exact names and non-null values when present.
func exactObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return nil, errFixture
	}
	for name, value := range fields {
		known := false
		for _, key := range allowed {
			known = known || name == key
		}
		if !known || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errFixture
		}
	}
	return fields, nil
}

func exactShape(data []byte) error {
	root, err := exactObject(data, "version", "session", "clock")
	if err != nil {
		return err
	}
	if _, err := exactObject(root["session"], "found", "revoked", "created_at", "expires_at", "idle_expires_at"); err != nil {
		return err
	}
	_, err = exactObject(root["clock"], "trusted", "now")
	return err
}

type rawFixture struct {
	Version *int `json:"version"`
	Session *struct {
		Found    *bool   `json:"found"`
		Revoked  *bool   `json:"revoked"`
		Created  *string `json:"created_at"`
		Absolute *string `json:"expires_at"`
		Idle     *string `json:"idle_expires_at"`
	} `json:"session"`
	Clock *struct {
		Trusted *bool   `json:"trusted"`
		Now     *string `json:"now"`
	} `json:"clock"`
}

// checkJSON rejects duplicate keys and excessive depth before typed decoding.
func checkJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var value func(int) error
	value = func(depth int) error {
		if depth > 16 {
			return errFixture
		}
		token, err := d.Token()
		if err != nil {
			return errFixture
		}
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return errFixture
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return errFixture
					}
					seen[name] = true
					if err := value(depth + 1); err != nil {
						return err
					}
				}
				end, err := d.Token()
				if err != nil || end != json.Delim('}') {
					return errFixture
				}
			case '[':
				for d.More() {
					if err := value(depth + 1); err != nil {
						return err
					}
				}
				end, err := d.Token()
				if err != nil || end != json.Delim(']') {
					return errFixture
				}
			default:
				return errFixture
			}
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errFixture
	}
	return nil
}

func parseTimestamp(text *string, required bool) (time.Time, error) {
	if text == nil {
		if required {
			return time.Time{}, errFixture
		}
		return time.Time{}, nil
	}
	if !timestampPattern.MatchString(*text) {
		return time.Time{}, errFixture
	}
	if (*text)[len(*text)-1] != 'Z' {
		offset := (*text)[len(*text)-6:]
		hour, _ := strconv.Atoi(offset[1:3])
		minute, _ := strconv.Atoi(offset[4:6])
		if hour > 23 || minute > 59 {
			return time.Time{}, errFixture
		}
	}
	t, err := time.Parse(time.RFC3339Nano, *text)
	if err != nil || t.UTC().Year() < 1 || t.UTC().Year() > 9999 {
		return time.Time{}, errFixture
	}
	return t, nil
}

// Decode validates the local fixture format, not the canonical public wire API.
// All errors are safe codes and never include input text or parser diagnostics.
func Decode(reader io.Reader) (sessionpolicy.Snapshot, sessionpolicy.Observation, error) {
	var s sessionpolicy.Snapshot
	var o sessionpolicy.Observation
	if reader == nil {
		return s, o, errFixture
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxFixtureBytes+1))
	if err != nil || len(data) > MaxFixtureBytes || !utf8.Valid(data) || checkJSON(data) != nil || exactShape(data) != nil {
		return s, o, errFixture
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var raw rawFixture
	if d.Decode(&raw) != nil || raw.Version == nil || *raw.Version != 1 || raw.Session == nil || raw.Clock == nil ||
		raw.Session.Found == nil || raw.Session.Revoked == nil || raw.Clock.Trusted == nil {
		return s, o, errFixture
	}
	s.Found, s.Revoked, o.Trusted = *raw.Session.Found, *raw.Session.Revoked, *raw.Clock.Trusted
	if s.CreatedAt, err = parseTimestamp(raw.Session.Created, s.Found); err != nil {
		return s, o, errFixture
	}
	if s.ExpiresAt, err = parseTimestamp(raw.Session.Absolute, s.Found); err != nil {
		return s, o, errFixture
	}
	if s.IdleExpiresAt, err = parseTimestamp(raw.Session.Idle, s.Found); err != nil {
		return s, o, errFixture
	}
	if o.At, err = parseTimestamp(raw.Clock.Now, true); err != nil {
		return s, o, errFixture
	}
	return s, o, nil
}
