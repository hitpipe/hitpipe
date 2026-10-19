package readback

import (
	"encoding/json"
	"fmt"
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
	maxContextBytes = 32 * 1024
	maxBlockBytes   = 36 * 1024
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
type Result struct {
	Outcome Outcome `json:"outcome"`
	Reason  string  `json:"reason"`
}

// Compare validates evidence and compares it without trimming, case-folding or collapsing spaces.
// Compare validates the local DeliveryIntent/ObservedTaskAttributes projection.
// It normalizes only NFC and CR/CRLF, preserves meaningful spaces, requires one
// canonical handoff block and applies the JCS-escaped context and rendered-block
// byte budgets before returning match, mismatch, incomplete or unknown.
func Compare(expected Expected, observed Observation) Result {
	if !observed.Retrieved {
		if observed.RetrievalError == "" {
			return Result{Outcome: Unknown, Reason: "retrieval evidence is absent"}
		}
		return Result{Outcome: Unknown, Reason: "retrieval failed"}
	}
	if !validText(expected.Workspace) || !validText(expected.List) || !validText(expected.Title) || !validText(expected.Assignee) || !validText(expected.Context) || !validText(expected.HandoffBlock) || !validText(observed.Workspace) || !validText(observed.List) || !validText(observed.Title) || !validText(observed.Context) || !validText(observed.HandoffBlock) || !validText(observed.TargetID) || !validText(observed.RetrievalError) {
		return Result{Outcome: Unknown, Reason: "readback contains invalid UTF-8"}
	}
	for _, assignee := range observed.Assignees {
		if !validText(assignee) {
			return Result{Outcome: Unknown, Reason: "readback contains invalid assignee text"}
		}
	}
	if observed.RetrievalError != "" {
		return Result{Outcome: Unknown, Reason: "retrieval evidence is contradictory"}
	}
	if contextBudget(expected) > maxContextBytes || contextBudgetFromObserved(observed) > maxContextBytes || renderedBlockBytes(expected.HandoffBlock) > maxBlockBytes || renderedBlockBytes(observed.HandoffBlock) > maxBlockBytes {
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
	if reason := validateHandoff(expected.HandoffBlock); reason != "" {
		return Result{Outcome: Incomplete, Reason: "expected " + reason}
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

func contextBudget(expected Expected) int {
	return jcsBytes(map[string]string{
		"assignee": normalize(expected.Assignee), "context": normalize(expected.Context), "handoff_block": normalize(expected.HandoffBlock),
		"list": normalize(expected.List), "title": normalize(expected.Title), "workspace": normalize(expected.Workspace),
	})
}

func contextBudgetFromObserved(observed Observation) int {
	return jcsBytes(map[string]any{
		"assignees": observed.Assignees, "context": normalize(observed.Context), "handoff_block": normalize(observed.HandoffBlock),
		"list": normalize(observed.List), "title": normalize(observed.Title), "workspace": normalize(observed.Workspace),
	})
}

func jcsBytes(value any) int {
	b, _ := json.Marshal(value)
	return len(b)
}

func renderedBlockBytes(value string) int { return len([]byte(normalize(value))) }

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
	if !handoffUUID.MatchString(caseID) {
		return "handoff block case identifier is not a lowercase UUID"
	}
	bodyStart := openEnd + len(suffix) + 1
	close := "\n" + closePrefix + caseID + suffix
	if bodyStart >= len(value) || !strings.HasSuffix(value, close) {
		return "handoff block closing marker is malformed"
	}
	body := value[bodyStart : len(value)-len(close)]
	if strings.Contains(body, "[HITPIPE-HANDOFF:") || strings.Contains(body, "[/HITPIPE-HANDOFF:") {
		return "handoff block contains a nested or multiple marker"
	}
	lines := strings.Split(body, "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "Service: ") || !strings.HasPrefix(lines[1], "Order: ") || !strings.HasPrefix(lines[2], "Context: ") {
		return "handoff block must contain Service, Order and Context labels on separate lines"
	}
	for _, line := range lines {
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "Service: "), "Order: "), "Context: ")) == "" {
			return "handoff block labels must have non-empty values"
		}
	}
	return ""
}

func Format(result Result) error {
	if result.Outcome == Match {
		return nil
	}
	return fmt.Errorf("%s: %s", result.Outcome, result.Reason)
}
