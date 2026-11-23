package readback

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	Match      Outcome = "match"
	Mismatch   Outcome = "mismatch"
	Incomplete Outcome = "incomplete"
	Unknown    Outcome = "unknown"
)

const (
	maxBlockBytes = 36 * 1024
)

var handoffUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Outcome is the bounded result of comparing expected attributes with evidence.
type Outcome string

// Expected is the application-owned projection of DeliveryIntent fields used by this slice.
type Expected struct {
	Workspace    string `json:"workspace"`
	List         string `json:"list"`
	Title        string `json:"title"`
	Assignee     string `json:"assignee"`
	HandoffBlock string `json:"handoff_block"`
	Context      string `json:"context"`
}

// Observation is the application-owned projection of ObservedTaskAttributes.
type Observation struct {
	TargetID       string   `json:"target_id"`
	Retrieved      bool     `json:"retrieved"`
	RetrievalError string   `json:"retrieval_error,omitempty"`
	Workspace      string   `json:"workspace"`
	List           string   `json:"list"`
	Title          string   `json:"title"`
	Assignees      []string `json:"assignees"`
	HandoffBlock   string   `json:"handoff_block"`
	Context        string   `json:"context"`
}

// Result records the outcome and a human-readable reason without performing side effects.
func normalize(value string) string {
	return norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"))
}

func validText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

type Result struct {
	Outcome                                           Outcome
	Workspace, List, Assignee, Title, Marker, Context *bool
	Block                                             *string
}

func Compare(expected Expected, observed Observation) Result {
	r := Result{Outcome: Unknown}
	if !observed.Retrieved || observed.RetrievalError != "" {
		return r
	}
	r.Workspace = equal(expected.Workspace, observed.Workspace, false)
	r.List = equal(expected.List, observed.List, false)
	r.Title = equal(expected.Title, observed.Title, true)
	if observed.Assignees != nil && expected.Assignee != "" {
		matches := false
		for _, a := range observed.Assignees {
			matches = matches || a == expected.Assignee
		}
		r.Assignee = &matches
	}
	want, caseID, wantOK := ExtractBlock(expected.HandoffBlock)
	got, other, gotOK := ExtractBlock(observed.HandoffBlock)
	if wantOK && gotOK {
		marker := caseID == other
		r.Marker = &marker
		context := want == got
		r.Context = &context
		r.Block = &got
	}
	r.Outcome = Match
	incomplete := false
	for _, m := range []*bool{r.Workspace, r.List, r.Assignee, r.Title, r.Marker, r.Context} {
		if m == nil {
			incomplete = true
			continue
		}
		if !*m {
			r.Outcome = Mismatch
		}
	}
	if r.Outcome == Match && incomplete {
		r.Outcome = Incomplete
	}
	return r
}

func equal(expected, actual string, normalized bool) *bool {
	if expected == "" || actual == "" || !validText(actual) || !validText(expected) {
		return nil
	}
	if normalized {
		expected, actual = normalize(expected), normalize(actual)
	}
	match := expected == actual
	return &match
}

// ExtractBlock selects exactly one well-formed v1 block in provider text.
// Other text is left out of the observation evidence.
func ExtractBlock(value string) (block, caseID string, ok bool) {
	value = normalize(value)
	if !validText(value) {
		return "", "", false
	}
	const open = "[HITPIPE-HANDOFF:"
	const end = "[/HITPIPE-HANDOFF:"
	const suffix = ":v1]"
	lines := strings.Split(value, "\n")
	start, stop := -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, open):
			if start >= 0 || !strings.HasSuffix(line, suffix) {
				return "", "", false
			}
			caseID = strings.TrimSuffix(strings.TrimPrefix(line, open), suffix)
			if !handoffUUID.MatchString(caseID) {
				return "", "", false
			}
			start = i
		case strings.HasPrefix(line, end):
			if start < 0 || stop >= 0 || line != end+caseID+suffix {
				return "", "", false
			}
			stop = i
		}
	}
	if start < 0 || stop-start < 4 || !strings.HasPrefix(lines[start+1], "Service: ") ||
		!strings.HasPrefix(lines[start+2], "Order: ") || lines[start+3] != "Context:" {
		return "", "", false
	}
	block = strings.Join(lines[start:stop+1], "\n")
	if len(block) > maxBlockBytes {
		return "", "", false
	}
	return block, caseID, true
}
