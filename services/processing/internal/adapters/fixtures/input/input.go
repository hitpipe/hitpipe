// Package input reads bounded synthetic fixtures. It is not an authenticated
// SourceFacts wire adapter and has no provider or production connector capability.
package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"io"
	"os"
	"time"
)

const MaxBytes = 1 << 20

// Scenario is an offline diagnostic envelope, not a canonical API request.
type Scenario struct {
	Policy eligibility.Policy `json:"policy"`
	Facts  eligibility.Facts  `json:"facts"`
	Now    time.Time          `json:"now"`
}

// Read loads an independent value snapshot. Errors never disclose file contents.
func Read(path string) (Scenario, error) {
	var s Scenario
	f, err := os.Open(path)
	if err != nil {
		return s, errors.New("FIXTURE_UNAVAILABLE")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(b) > MaxBytes {
		return s, errors.New("FIXTURE_SIZE_OR_READ")
	}
	if err = StrictDecode(b, &s); err != nil {
		return s, err
	}
	return s, nil
}

// StrictDecode rejects duplicate/unknown keys, trailing JSON and invalid typed
// numbers. Its error is safe for diagnostic output. It does not validate OAS.
func StrictDecode(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if err := walk(d); err != nil {
		return errors.New("INVALID_JSON")
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("INVALID_JSON")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("INVALID_JSON_SHAPE")
	}
	return nil
}
func walk(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return e
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return errors.New("duplicate")
			}
			seen[key] = true
			if e = walk(d); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e := walk(d); e != nil {
				return e
			}
		}
	default:
		return errors.New("delimiter")
	}
	_, err = d.Token()
	return err
}
