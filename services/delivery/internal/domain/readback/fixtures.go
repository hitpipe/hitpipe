package readback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"unicode/utf8"
)

var nonCanonicalKey = regexp.MustCompile(`"[A-Z][A-Za-z0-9_]*"\s*:`)

type Fixture struct {
	Expected    Expected    `json:"expected"`
	Observation Observation `json:"observation"`
}

func ParseFixture(data []byte) (Fixture, error) {
	if err := validateJSONBytes(data); err != nil {
		return Fixture{}, err
	}
	var fixture Fixture
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, err
	}
	if decoder.More() {
		return Fixture{}, fmt.Errorf("fixture has trailing JSON")
	}
	return fixture, nil
}

// ParseSavedObservation decodes the strict local boundary used by the file adapter.
func ParseSavedObservation(data []byte, out any) error {
	if err := validateJSONBytes(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("saved observation has trailing JSON")
	}
	return nil
}

func validateJSONBytes(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON contains invalid UTF-8")
	}
	if nonCanonicalKey.Match(data) {
		return fmt.Errorf("JSON contains non-canonical key casing")
	}
	for i := 0; i+5 < len(data); i++ {
		if data[i] == '\\' && data[i+1] == 'u' {
			c := data[i+2]
			if c == 'd' || c == 'D' {
				return fmt.Errorf("JSON contains unsupported surrogate escape")
			}
		}
	}
	return nil
}

func EvaluateFixture(data []byte) (Result, error) {
	fixture, err := ParseFixture(data)
	if err != nil {
		return Result{}, err
	}
	if fixture.Expected.Workspace == "" || fixture.Expected.List == "" || fixture.Expected.Title == "" || fixture.Expected.Assignee == "" {
		return Result{}, fmt.Errorf("fixture expected fields are incomplete")
	}
	return Compare(fixture.Expected, fixture.Observation), nil
}
