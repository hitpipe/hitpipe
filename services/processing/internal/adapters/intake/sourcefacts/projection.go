// Package sourcefacts projects an already received SourceFacts message. It does
// not authenticate a sender, establish authority, read a provider or grant an
// action. Local policy pins are never represented as Intake attestations.
package sourcefacts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
)

// Expected is supplied by the caller from its already established context.
// Policy remains a separate local input; it is not learned from the message.
type Expected struct {
	CaseID, SessionID, AccountID, ConnectionID string
	LiveMode                                   bool
}

// Provenance retains the source's identifiers and exact time strings, including
// their original offsets/precision. Digest is preserved, not verified/recomputed.
type Provenance struct {
	FactsID, Digest                           string
	SessionCreatedAt                          *string
	OldestObservedAt, CompletedAt, ValidUntil string
}

type Projection struct {
	Snapshot   evaluatepaidorder.Snapshot
	Provenance Provenance
}

// Decode rejects ambiguous/invalid shape before projection. There is no clock,
// IO, mutable cache or retry. A zero result is returned on every failure.
func Decode(data []byte, expected Expected, policy eligibility.Policy) (Projection, error) {
	fail := func() (Projection, error) { return Projection{}, errors.New("INVALID_SOURCE_FACTS") }
	if !eligibility.ValidatePolicy(policy) || expected.CaseID == "" || expected.SessionID == "" ||
		expected.AccountID != policy.AccountID || expected.ConnectionID != policy.ConnectionID || expected.LiveMode != *policy.LiveMode {
		return fail()
	}
	fields, err := decodeFields(data)
	if err != nil {
		return fail()
	}
	str := func(k string) string { var s string; _ = json.Unmarshal(fields[k], &s); return s }
	var live bool
	_ = json.Unmarshal(fields["livemode"], &live)
	if str("case_id") != expected.CaseID || str("session_id") != expected.SessionID || str("stripe_account_id") != expected.AccountID || str("connection_id") != expected.ConnectionID || live != expected.LiveMode {
		return fail()
	}
	provenance := Provenance{FactsID: str("facts_id"), Digest: str("digest"), OldestObservedAt: str("oldest_observed_at"), CompletedAt: str("completed_at"), ValidUntil: str("valid_until")}
	if !bytes.Equal(bytes.TrimSpace(fields["session_created_at"]), []byte("null")) {
		s := str("session_created_at")
		provenance.SessionCreatedAt = &s
	}
	// All names, presence, nullable values and scalar shapes were validated from
	// generated OAS metadata. Only the domain's account name differs from wire.
	fields["account_id"] = fields["stripe_account_id"]
	delete(fields, "stripe_account_id")
	delete(fields, "facts_id")
	delete(fields, "digest")
	b, err := json.Marshal(fields)
	if err != nil {
		return fail()
	}
	var facts eligibility.Facts
	if err = json.Unmarshal(b, &facts); err != nil {
		return fail()
	}
	return Projection{Snapshot: evaluatepaidorder.Snapshot{ConfigID: policy.ConfigID, SchemaVersion: policy.SchemaVersion, EvaluatorVersion: policy.EvaluatorVersion, Facts: facts}, Provenance: provenance}, nil
}

type fieldRule struct {
	kind                 string
	nullable             bool
	format, pattern      string
	choices              []string
	minLength, maxLength int
	minimum, maximum     int64
}

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var dateTime = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

func decodeFields(data []byte) (map[string]json.RawMessage, error) {
	invalid := errors.New("INVALID_SOURCE_FACTS")
	if !utf8.Valid(data) {
		return nil, invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, invalid
	}
	fields := make(map[string]json.RawMessage, len(wireFields))
	for d.More() {
		t, err = d.Token()
		if err != nil {
			return nil, invalid
		}
		k, ok := t.(string)
		if !ok {
			return nil, invalid
		}
		rule, ok := wireFields[k]
		if !ok || fields[k] != nil {
			return nil, invalid
		}
		var v json.RawMessage
		if d.Decode(&v) != nil || !rule.valid(v) {
			return nil, invalid
		}
		if rule.kind == "integer" && !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			n, _ := integer(v)
			v = json.RawMessage(strconv.FormatInt(n, 10))
		}
		fields[k] = v
	}
	if _, err = d.Token(); err != nil {
		return nil, invalid
	}
	if _, err = d.Token(); err != io.EOF || len(fields) != len(wireFields) {
		return nil, invalid
	}
	return fields, nil
}

func (r fieldRule) valid(raw json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return r.nullable
	}
	switch r.kind {
	case "boolean":
		var v bool
		return json.Unmarshal(raw, &v) == nil
	case "integer":
		v, ok := integer(raw)
		return ok && v >= r.minimum && (r.maximum == 0 || v <= r.maximum)
	case "string":
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return false
		}
		n := utf8.RuneCountInString(v)
		if n < r.minLength || (r.maxLength > 0 && n > r.maxLength) || (len(r.choices) > 0 && !slices.Contains(r.choices, v)) {
			return false
		}
		if r.pattern != "" && !regexp.MustCompile(r.pattern).MatchString(v) {
			return false
		}
		if r.format == "uuid" && !uuid.MatchString(v) {
			return false
		}
		if r.format == "date-time" {
			_, err := time.Parse(time.RFC3339Nano, v)
			return dateTime.MatchString(v) && err == nil
		}
		return true
	}
	return false
}

// JSON Schema integer is mathematical: 1, 1.0 and 1e0 are equivalent. Check
// the exact rational value after a bounded-magnitude screen, without rounding
// fractional values or numbers outside HP-MVP-A-004's safe integer range.
func integer(raw []byte) (int64, bool) {
	text := string(bytes.TrimSpace(raw))
	var n json.Number
	if json.Unmarshal(raw, &n) != nil || n.String() == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || f < 0 || f > 9007199254740991 {
		return 0, false
	}
	if f == 0 {
		mantissa, _, _ := strings.Cut(strings.ToLower(text), "e")
		return 0, !strings.ContainsAny(mantissa, "123456789")
	}
	if f < 1 {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return 0, false
	}
	v := r.Num().Int64()
	return v, v >= 0 && v <= 9007199254740991
}
