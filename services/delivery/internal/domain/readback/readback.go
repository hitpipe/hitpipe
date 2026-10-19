package readback

import (
	"fmt"
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
	maxContextBytes     = 32 * 1024
	maxBlockBytes       = 36 * 1024
	maxHandoffBodyBytes = maxBlockBytes
)

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
type Result struct {
	Outcome Outcome `json:"outcome"`
	Reason  string  `json:"reason"`
}

// Compare validates evidence and compares it without trimming, case-folding or collapsing spaces.
func Compare(expected Expected, observed Observation) Result {
	if !observed.Retrieved {
		if observed.RetrievalError == "" {
			return Result{Outcome: Unknown, Reason: "retrieval evidence is absent"}
		}
		return Result{Outcome: Unknown, Reason: "retrieval failed"}
	}
	if !utf8.ValidString(expected.Workspace) || !utf8.ValidString(expected.List) || !utf8.ValidString(expected.Title) || !utf8.ValidString(expected.Assignee) || !utf8.ValidString(expected.Context) || !utf8.ValidString(expected.HandoffBlock) || !utf8.ValidString(observed.Workspace) || !utf8.ValidString(observed.List) || !utf8.ValidString(observed.Title) || !utf8.ValidString(observed.Context) || !utf8.ValidString(observed.HandoffBlock) {
		return Result{Outcome: Unknown, Reason: "readback contains invalid UTF-8"}
	}
	if observed.RetrievalError != "" {
		return Result{Outcome: Unknown, Reason: "retrieval evidence is contradictory"}
	}
	if len([]byte(observed.Context)) > maxContextBytes || len([]byte(observed.HandoffBlock)) > maxBlockBytes || len([]byte(expected.Context)) > maxContextBytes || len([]byte(expected.HandoffBlock)) > maxBlockBytes {
		return Result{Outcome: Incomplete, Reason: "readback exceeds bounded evidence size"}
	}
	for name, value := range map[string]string{
		"workspace":     observed.Workspace,
		"list":          observed.List,
		"title":         observed.Title,
		"handoff_block": observed.HandoffBlock,
		"context":       observed.Context,
	} {
		if value == "" {
			return Result{Outcome: Incomplete, Reason: "missing " + name}
		}
	}
	if expected.Assignee == "" {
		return Result{Outcome: Incomplete, Reason: "expected assignee is empty"}
	}
	if reason := validateHandoff(observed.HandoffBlock); reason != "" {
		return Result{Outcome: Incomplete, Reason: reason}
	}
	if normalize(observed.Workspace) != normalize(expected.Workspace) {
		return Result{Outcome: Mismatch, Reason: "workspace mismatch"}
	}
	if normalize(observed.List) != normalize(expected.List) {
		return Result{Outcome: Mismatch, Reason: "list mismatch"}
	}
	if normalize(observed.Title) != normalize(expected.Title) {
		return Result{Outcome: Mismatch, Reason: "title mismatch"}
	}
	if !containsNormalized(observed.Assignees, expected.Assignee) {
		return Result{Outcome: Mismatch, Reason: "configured assignee absent"}
	}
	if normalize(observed.HandoffBlock) != normalize(expected.HandoffBlock) {
		return Result{Outcome: Mismatch, Reason: "handoff block mismatch"}
	}
	if normalize(observed.Context) != normalize(expected.Context) {
		return Result{Outcome: Mismatch, Reason: "context mismatch"}
	}
	return Result{Outcome: Match, Reason: "all expected readback attributes match"}
}

func normalize(value string) string {
	return norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"))
}

func containsNormalized(values []string, expected string) bool {
	for _, value := range values {
		if normalize(value) == normalize(expected) {
			return true
		}
	}
	return false
}

func validateHandoff(value string) string {
	value = normalize(value)
	const prefix = "[HITPIPE-HANDOFF:"
	const suffix = ":v1]"
	const closePrefix = "[/HITPIPE-HANDOFF:"
	if !strings.HasPrefix(value, prefix) {
		return "handoff block must be one canonical v1 block"
	}
	openEnd := strings.Index(value, suffix)
	if openEnd < len(prefix) || openEnd-len(prefix) != 36 {
		return "handoff block case identifier is malformed"
	}
	caseID := value[len(prefix):openEnd]
	bodyStart := openEnd + len(suffix) + 1
	close := "\n" + closePrefix + caseID + suffix
	if bodyStart >= len(value) || !strings.HasSuffix(value, close) {
		return "handoff block closing marker is malformed"
	}
	body := value[bodyStart : len(value)-len(close)]
	if len([]byte(body)) == 0 || len([]byte(body)) > maxHandoffBodyBytes {
		return "handoff block body is empty or exceeds its byte budget"
	}
	return ""
}

func Format(result Result) error {
	if result.Outcome == Match {
		return nil
	}
	return fmt.Errorf("%s: %s", result.Outcome, result.Reason)
}
