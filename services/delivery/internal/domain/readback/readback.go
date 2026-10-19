package readback

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	Match Outcome = "match"
	Mismatch Outcome = "mismatch"
	Incomplete Outcome = "incomplete"
	Unknown Outcome = "unknown"
)

const (
	maxContextBytes = 32 * 1024
	maxBlockBytes = 36 * 1024
	startMarker = "<!-- hitpipe:handoff:start -->"
	endMarker = "<!-- hitpipe:handoff:end -->"
)

type Outcome string

type Expected struct {
	Workspace string `json:"workspace"`
	List string `json:"list"`
	Title string `json:"title"`
	Assignee string `json:"assignee"`
	HandoffBlock string `json:"handoff_block"`
	Context string `json:"context"`
}

type Observation struct {
	Retrieved bool `json:"retrieved"`
	RetrievalError string `json:"retrieval_error,omitempty"`
	Workspace string `json:"workspace"`
	List string `json:"list"`
	Title string `json:"title"`
	Assignees []string `json:"assignees"`
	HandoffBlock string `json:"handoff_block"`
	Context string `json:"context"`
}

type Result struct {
	Outcome Outcome `json:"outcome"`
	Reason string `json:"reason"`
}

func Compare(expected Expected, observed Observation) Result {
	if !observed.Retrieved {
		if observed.RetrievalError == "" {
			return Result{Outcome: Unknown, Reason: "retrieval evidence is absent"}
		}
		return Result{Outcome: Unknown, Reason: "retrieval failed"}
	}
	if !utf8.ValidString(observed.Context) || !utf8.ValidString(observed.HandoffBlock) {
		return Result{Outcome: Unknown, Reason: "readback contains invalid UTF-8"}
	}
	if len([]byte(observed.Context)) > maxContextBytes || len([]byte(observed.HandoffBlock)) > maxBlockBytes {
		return Result{Outcome: Incomplete, Reason: "readback exceeds bounded evidence size"}
	}
	for name, value := range map[string]string{
		"workspace": observed.Workspace,
		"list": observed.List,
		"title": observed.Title,
		"handoff_block": observed.HandoffBlock,
		"context": observed.Context,
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
	return norm.NFC.String(strings.ReplaceAll(value, "\r\n", "\n"))
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
	if strings.Count(value, startMarker) != 1 || strings.Count(value, endMarker) != 1 {
		return "handoff block must contain exactly one start and end marker"
	}
	start := strings.Index(value, startMarker)
	end := strings.Index(value, endMarker)
	if end <= start+len(startMarker) {
		return "handoff block is empty or markers are malformed"
	}
	return ""
}

func Format(result Result) error {
	if result.Outcome == Match {
		return nil
	}
	return fmt.Errorf("%s: %s", result.Outcome, result.Reason)
}
