// Package intent renders the fixed SERVICE_ORDER/handoff-block-v1 contract.
package intent

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"golang.org/x/text/unicode/norm"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Renderer struct{}

// Normalized is suitable for a visible preview. Build refuses non-normalized
// accepted input, so processing never silently changes a saved user's meaning.
func Normalized(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", preparecase.ErrHold
	}
	s = norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' {
			return "", preparecase.ErrHold
		}
	}
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "[HITPIPE-HANDOFF:") || strings.HasPrefix(l, "[/HITPIPE-HANDOFF:") {
			return "", preparecase.ErrHold
		}
	}
	return s, nil
}
func sameNormalized(s string) bool { n, e := Normalized(s); return e == nil && n == s }
func Digest(b []byte) string       { sum := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(sum[:]) }
func (Renderer) Build(c preparecase.Case, d preparecase.Decision, values map[string]string, fields []preparecase.ContextField) (preparecase.Payload, error) {
	fail := func() (preparecase.Payload, error) { return preparecase.Payload{}, preparecase.ErrHold }
	p := d.Snapshot.Proof
	if d.Financial.Status != eligibility.Valid || p == nil || !uuid.MatchString(p.FactsID) || !digest.MatchString(p.Digest) || p.SessionCreatedAt == nil || d.Snapshot.Facts.SessionCreatedAt == nil || !digest.MatchString(c.ConfigDigest) || c.Epoch < 1 || c.Epoch > 9007199254740991 {
		return fail()
	}
	created, e := time.Parse(time.RFC3339Nano, *p.SessionCreatedAt)
	if e != nil || !created.Equal(*d.Snapshot.Facts.SessionCreatedAt) {
		return fail()
	}
	for _, pair := range []struct {
		raw string
		t   time.Time
	}{{p.OldestObservedAt, d.Snapshot.Facts.OldestObservedAt}, {p.CompletedAt, d.Snapshot.Facts.CompletedAt}, {p.ValidUntil, d.Snapshot.Facts.ValidUntil}} {
		parsed, e := time.Parse(time.RFC3339Nano, pair.raw)
		if e != nil || !parsed.Equal(pair.t) {
			return fail()
		}
	}
	if d.Snapshot.ConfigID != c.ConfigID || d.Snapshot.SchemaVersion != c.Policy.SchemaVersion || d.Snapshot.EvaluatorVersion != c.Policy.EvaluatorVersion || d.Snapshot.Facts.CaseID != c.CaseID || d.Snapshot.Facts.SessionID != c.SessionID || eligibility.Evaluate(c.Policy, d.Snapshot.Facts, d.At).Status != eligibility.Valid {
		return fail()
	}
	m := c.Mapping
	if m.TitlePattern != "SERVICE_ORDER" || !sameNormalized(m.ServiceLabel) || strings.Contains(m.ServiceLabel, "\n") || utf8.RuneCountInString(m.ServiceLabel) < 1 || utf8.RuneCountInString(m.ServiceLabel) > 120 || len(m.RequiredKeys) > 20 || len(fields) != len(m.RequiredKeys) || len(values) != len(m.RequiredKeys) {
		return fail()
	}
	for _, s := range []string{c.CaseID, c.WorkspaceID, c.OperationID, c.ConfigID, m.SourceConnectionID, m.DestinationConnectionID} {
		if !uuid.MatchString(s) {
			return fail()
		}
	}
	for _, s := range []string{m.ListID, m.AssigneeID, m.DestinationWorkspaceID, c.SessionID} {
		if !sameNormalized(s) || strings.Contains(s, "\n") || utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > 255 {
			return fail()
		}
	}
	keys := append([]string{}, m.RequiredKeys...)
	sort.Strings(keys)
	byKey := map[string]preparecase.ContextField{}
	for _, f := range fields {
		if _, exists := byKey[f.Key]; exists || !f.ExpiresAt.After(d.At) || f.AddedAt.After(d.At) {
			return fail()
		}
		byKey[f.Key] = f
	}
	ordered := make([]preparecase.ContextField, 0, len(keys))
	for i, k := range keys {
		v, ok := values[k]
		f, present := byKey[k]
		if !keyPattern.MatchString(k) || !ok || !present || f.Value != v || !sameNormalized(v) || (i > 0 && keys[i-1] == k) || utf8.RuneCountInString(v) > 32768 {
			return fail()
		}
		ordered = append(ordered, f)
	}
	b, e := json.Marshal(values)
	if e != nil {
		return fail()
	}
	b, e = jcs.Transform(b)
	if e != nil || len(b) > 32768 {
		return fail()
	}
	caseBytes, e := hex.DecodeString(strings.ReplaceAll(c.CaseID, "-", ""))
	if e != nil {
		return fail()
	}
	publicRef := "HP-" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(caseBytes)
	marker := "[HITPIPE-HANDOFF:" + c.CaseID + ":v1]"
	block := marker + "\nService: " + m.ServiceLabel + "\nOrder: " + c.SessionID + "\nContext:\n"
	for _, k := range keys {
		block += k + ": " + values[k] + "\n"
	}
	block += "[/HITPIPE-HANDOFF:" + c.CaseID + ":v1]"
	if len([]byte(block)) > 36864 {
		return fail()
	}
	lineage, e := contentfiles.UUID()
	if e != nil {
		return fail()
	}
	episode := c.EpisodeID
	deadline := c.EpisodeDeadlineAt
	if episode == "" {
		if c.HasEpisodes {
			return fail()
		}
		episode, e = contentfiles.UUID()
		if e != nil {
			return fail()
		}
		deadline = d.At.Add(300 * time.Second)
	}
	if !deadline.After(d.At) {
		return fail()
	}
	intent := preparecase.DeliveryIntent{AssigneeId: m.AssigneeID, CaseId: c.CaseID, CaseMarker: marker, ConfigDigest: c.ConfigDigest, ConfigId: c.ConfigID, ConnectionEpoch: c.Epoch, ConnectionId: m.DestinationConnectionID, Context: ordered, DeadlineAt: deadline, DestinationWorkspaceId: m.DestinationWorkspaceID, EpisodeId: episode, FactsValidUntil: d.Snapshot.Facts.ValidUntil, LineageId: lineage, MessageId: lineage, OperationId: c.OperationID, OrderReference: publicRef, RenderedHandoffBlock: block, RenderingVersion: "handoff-block-v1", SchemaVersion: "1.0", ServiceLabel: m.ServiceLabel, SourceFactsDigest: p.Digest, SourceFactsId: p.FactsID, SourceSessionId: c.SessionID, TargetListId: m.ListID, Title: m.ServiceLabel + " — " + publicRef, WorkspaceId: c.WorkspaceID}
	raw, e := json.Marshal(intent)
	if e != nil {
		return fail()
	}
	var members map[string]json.RawMessage
	json.Unmarshal(raw, &members)
	delete(members, "message_digest")
	raw, e = json.Marshal(members)
	if e != nil {
		return fail()
	}
	canonical, e := jcs.Transform(raw)
	if e != nil {
		return fail()
	}
	intent.MessageDigest = Digest(canonical)
	raw, e = json.Marshal(intent)
	if e != nil {
		return fail()
	}
	raw, e = jcs.Transform(raw)
	if e != nil || len(raw) > 65536 {
		return fail()
	}
	return preparecase.Payload{Body: raw, EpisodeID: episode, LineageID: lineage, MessageID: lineage, Digest: intent.MessageDigest, DeadlineAt: deadline}, nil
}
