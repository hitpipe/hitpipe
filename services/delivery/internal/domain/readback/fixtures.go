package readback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"
)

type Fixture struct {
	Expected    Expected    `json:"expected"`
	Observation Observation `json:"observation"`
}

// ParseFixture decodes one strict fixture document. It rejects duplicate, mixed-case
// or escaped object keys, malformed UTF-8/surrogates and any bytes after the root value.
func ParseFixture(data []byte) (Fixture, error) {
	if err := validateJSONDocument(data); err != nil {
		return Fixture{}, err
	}
	var fixture Fixture
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Fixture{}, fmt.Errorf("fixture has trailing JSON: %w", err)
	}
	return fixture, nil
}

// ParseSavedObservation decodes the strict local boundary used by FileReader. It
// applies the same structural and Unicode policy as ParseFixture.
func ParseSavedObservation(data []byte, out any) error {
	if err := validateJSONDocument(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("saved observation has trailing JSON: %w", err)
	}
	return nil
}

func validateJSONDocument(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON contains invalid UTF-8")
	}
	i := 0
	if err := scanJSONValue(data, &i); err != nil {
		return err
	}
	skipJSONSpace(data, &i)
	if i != len(data) {
		return fmt.Errorf("JSON has trailing bytes after the root value")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("more than one JSON value")
		}
		return err
	}
	return nil
}

func skipJSONSpace(data []byte, i *int) {
	for *i < len(data) && (data[*i] == ' ' || data[*i] == '\n' || data[*i] == '\r' || data[*i] == '\t') {
		*i++
	}
}

func scanJSONValue(data []byte, i *int) error {
	skipJSONSpace(data, i)
	if *i >= len(data) {
		return fmt.Errorf("JSON value is missing")
	}
	switch data[*i] {
	case '{':
		*i++
		keys := map[string]bool{}
		skipJSONSpace(data, i)
		if *i < len(data) && data[*i] == '}' {
			*i++
			return nil
		}
		for {
			skipJSONSpace(data, i)
			if *i >= len(data) || data[*i] != '"' {
				return fmt.Errorf("JSON object key must be a string")
			}
			raw, decoded, err := scanJSONString(data, i)
			if err != nil {
				return err
			}
			if raw != decoded {
				return fmt.Errorf("JSON object keys must use exact unescaped spelling")
			}
			for _, r := range decoded {
				if unicode.IsUpper(r) {
					return fmt.Errorf("JSON object key %q uses non-canonical casing", decoded)
				}
			}
			if keys[decoded] {
				return fmt.Errorf("JSON object contains duplicate key %q", decoded)
			}
			keys[decoded] = true
			skipJSONSpace(data, i)
			if *i >= len(data) || data[*i] != ':' {
				return fmt.Errorf("JSON object key %q has no colon", decoded)
			}
			*i++
			if err := scanJSONValue(data, i); err != nil {
				return err
			}
			skipJSONSpace(data, i)
			if *i >= len(data) {
				return fmt.Errorf("JSON object is not closed")
			}
			if data[*i] == '}' {
				*i++
				return nil
			}
			if data[*i] != ',' {
				return fmt.Errorf("JSON object requires comma or close")
			}
			*i++
		}
	case '[':
		*i++
		skipJSONSpace(data, i)
		if *i < len(data) && data[*i] == ']' {
			*i++
			return nil
		}
		for {
			if err := scanJSONValue(data, i); err != nil {
				return err
			}
			skipJSONSpace(data, i)
			if *i >= len(data) {
				return fmt.Errorf("JSON array is not closed")
			}
			if data[*i] == ']' {
				*i++
				return nil
			}
			if data[*i] != ',' {
				return fmt.Errorf("JSON array requires comma or close")
			}
			*i++
		}
	case '"':
		_, _, err := scanJSONString(data, i)
		return err
	case 't':
		return scanLiteral(data, i, "true")
	case 'f':
		return scanLiteral(data, i, "false")
	case 'n':
		return scanLiteral(data, i, "null")
	default:
		start := *i
		for *i < len(data) && data[*i] != ',' && data[*i] != ']' && data[*i] != '}' && data[*i] != ' ' && data[*i] != '\n' && data[*i] != '\r' && data[*i] != '\t' {
			*i++
		}
		if start == *i {
			return fmt.Errorf("invalid JSON value")
		}
		var number json.Number
		if err := json.Unmarshal(data[start:*i], &number); err != nil {
			return fmt.Errorf("invalid JSON value: %w", err)
		}
		return nil
	}
}

func scanLiteral(data []byte, i *int, literal string) error {
	if len(data)-*i < len(literal) || string(data[*i:*i+len(literal)]) != literal {
		return fmt.Errorf("invalid JSON literal")
	}
	*i += len(literal)
	return nil
}

func scanJSONString(data []byte, i *int) (string, string, error) {
	start := *i
	*i++
	for *i < len(data) {
		if data[*i] == '"' {
			raw := string(data[start+1 : *i])
			var decoded string
			if err := json.Unmarshal(data[start:*i+1], &decoded); err != nil {
				return "", "", err
			}
			*i++
			return raw, decoded, nil
		}
		if data[*i] < 0x20 {
			return "", "", fmt.Errorf("JSON string contains a control byte")
		}
		if data[*i] == '\\' {
			*i++
			if *i >= len(data) {
				return "", "", fmt.Errorf("JSON string has incomplete escape")
			}
			if data[*i] == 'u' {
				if *i+4 >= len(data) {
					return "", "", fmt.Errorf("JSON unicode escape is incomplete")
				}
				value, ok := hex4(data[*i+1 : *i+5])
				if !ok {
					return "", "", fmt.Errorf("JSON unicode escape is invalid")
				}
				*i += 5
				if value >= 0xD800 && value <= 0xDBFF {
					if *i+5 >= len(data) || data[*i] != '\\' || data[*i+1] != 'u' {
						return "", "", fmt.Errorf("JSON contains an unpaired high surrogate")
					}
					low, ok := hex4(data[*i+2 : *i+6])
					if !ok || low < 0xDC00 || low > 0xDFFF {
						return "", "", fmt.Errorf("JSON contains an invalid surrogate pair")
					}
					*i += 6
				} else if value >= 0xDC00 && value <= 0xDFFF {
					return "", "", fmt.Errorf("JSON contains an unpaired low surrogate")
				}
				continue
			}
			if !bytes.ContainsRune([]byte(`"\\/bfnrt`), rune(data[*i])) {
				return "", "", fmt.Errorf("JSON escape is invalid")
			}
			*i++
			continue
		}
		*i++
	}
	return "", "", fmt.Errorf("JSON string is not closed")
}

func hex4(value []byte) (int, bool) {
	if len(value) != 4 {
		return 0, false
	}
	n := 0
	for _, c := range value {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += int(c-'A') + 10
		default:
			return 0, false
		}
	}
	return n, true
}

// EvaluateFixture parses a CLI fixture and projects it through the same comparator
// used by the application consumer; it performs no provider I/O or side effects.
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
