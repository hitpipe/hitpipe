package revocation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

var (
	errWire   = errors.New("revocation control: invalid wire value")
	timestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-](0[0-9]|1[0-9]|2[0-3]):[0-5][0-9])$`)
)

type commandWire struct {
	TargetRuntimeID string    `json:"target_runtime_id"`
	BootID          string    `json:"boot_id"`
	Generation      uint64    `json:"generation"`
	RevocationID    string    `json:"revocation_id"`
	WorkspaceID     *string   `json:"workspace_id"`
	Scope           string    `json:"scope"`
	SubjectID       string    `json:"subject_id"`
	Epoch           uint64    `json:"epoch"`
	DeadlineAt      time.Time `json:"deadline_at"`
}

type ackWire struct {
	RevocationID            string    `json:"revocation_id"`
	RuntimeID               string    `json:"runtime_id"`
	BootID                  string    `json:"boot_id"`
	WorkspaceID             *string   `json:"workspace_id"`
	Scope                   string    `json:"scope"`
	SubjectID               string    `json:"subject_id"`
	Generation              uint64    `json:"generation"`
	AppliedEpoch            uint64    `json:"applied_epoch"`
	OldStartSectionsDrained bool      `json:"old_start_sections_drained"`
	AcknowledgedAt          time.Time `json:"acknowledged_at"`
}

func commandValue(c startgate.BarrierCommand) commandWire {
	return commandWire{
		TargetRuntimeID: c.TargetRuntimeID, BootID: c.BootID, Generation: c.Generation,
		RevocationID: c.RevocationID, WorkspaceID: c.Subject.WorkspaceID, Scope: string(c.Subject.Scope),
		SubjectID: c.Subject.SubjectID, Epoch: c.Epoch, DeadlineAt: c.DeadlineAt.UTC(),
	}
}

func ackValue(a startgate.BarrierAck) ackWire {
	return ackWire{
		RevocationID: a.RevocationID, RuntimeID: a.RuntimeID, BootID: a.BootID, Generation: a.Generation,
		WorkspaceID: a.Subject.WorkspaceID, Scope: string(a.Subject.Scope), SubjectID: a.Subject.SubjectID,
		AppliedEpoch: a.AppliedEpoch, OldStartSectionsDrained: a.OldStartSectionsDrained,
		AcknowledgedAt: a.AcknowledgedAt.UTC(),
	}
}

func safeWireInteger(n uint64) bool { return n > 0 && n <= startgate.MaxSafeInteger }

func validTime(t time.Time) bool { return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 }

func validIdentity(i startgate.Identity) bool {
	return runtimeaccess.ValidUUID(i.RuntimeID) && runtimeaccess.ValidUUID(i.BootID) && safeWireInteger(i.Generation)
}

func exactSubject(s startgate.Subject) bool {
	if !runtimeaccess.ValidUUID(s.SubjectID) {
		return false
	}
	switch s.Scope {
	case startgate.Session:
		// SESSION is global in this slice; null is not a missing workspace.
		return s.WorkspaceID == nil
	case startgate.Workspace:
		return s.WorkspaceID != nil && strings.EqualFold(*s.WorkspaceID, s.SubjectID)
	default:
		// The contract names additional scopes; this coordinator does not implement them.
		return false
	}
}

func validCommand(c startgate.BarrierCommand) bool {
	return validIdentity(startgate.Identity{RuntimeID: c.TargetRuntimeID, BootID: c.BootID, Generation: c.Generation}) &&
		runtimeaccess.ValidUUID(c.RevocationID) && exactSubject(c.Subject) && safeWireInteger(c.Epoch) && validTime(c.DeadlineAt.UTC())
}

func ackMatches(c startgate.BarrierCommand, a startgate.BarrierAck) bool {
	return exactSubject(a.Subject) && c.Subject.Scope == a.Subject.Scope &&
		strings.EqualFold(c.Subject.SubjectID, a.Subject.SubjectID) &&
		((c.Subject.WorkspaceID == nil && a.Subject.WorkspaceID == nil) ||
			(c.Subject.WorkspaceID != nil && a.Subject.WorkspaceID != nil && strings.EqualFold(*c.Subject.WorkspaceID, *a.Subject.WorkspaceID))) &&
		strings.EqualFold(c.RevocationID, a.RevocationID) && strings.EqualFold(c.TargetRuntimeID, a.RuntimeID) &&
		strings.EqualFold(c.BootID, a.BootID) && c.Generation == a.Generation && c.Epoch == a.AppliedEpoch &&
		a.OldStartSectionsDrained && validTime(a.AcknowledgedAt.UTC()) && a.AcknowledgedAt.Before(c.DeadlineAt)
}

func parseCommand(body []byte) (startgate.BarrierCommand, error) {
	var w commandWire
	fields := map[string]any{
		"target_runtime_id": &w.TargetRuntimeID, "boot_id": &w.BootID, "generation": &w.Generation,
		"revocation_id": &w.RevocationID, "workspace_id": &w.WorkspaceID, "scope": &w.Scope,
		"subject_id": &w.SubjectID, "epoch": &w.Epoch, "deadline_at": &w.DeadlineAt,
	}
	if err := exactObject(body, fields); err != nil {
		return startgate.BarrierCommand{}, err
	}
	c := startgate.BarrierCommand{
		TargetRuntimeID: w.TargetRuntimeID, BootID: w.BootID, Generation: w.Generation, RevocationID: w.RevocationID,
		Subject: startgate.Subject{WorkspaceID: w.WorkspaceID, Scope: startgate.Scope(w.Scope), SubjectID: w.SubjectID},
		Epoch:   w.Epoch, DeadlineAt: w.DeadlineAt.UTC(),
	}
	if !validCommand(c) {
		return startgate.BarrierCommand{}, errWire
	}
	return c, nil
}

func parseAck(body []byte) (startgate.BarrierAck, error) {
	var w ackWire
	fields := map[string]any{
		"revocation_id": &w.RevocationID, "runtime_id": &w.RuntimeID, "boot_id": &w.BootID,
		"workspace_id": &w.WorkspaceID, "scope": &w.Scope, "subject_id": &w.SubjectID,
		"generation": &w.Generation, "applied_epoch": &w.AppliedEpoch,
		"old_start_sections_drained": &w.OldStartSectionsDrained, "acknowledged_at": &w.AcknowledgedAt,
	}
	if err := exactObject(body, fields); err != nil {
		return startgate.BarrierAck{}, err
	}
	return startgate.BarrierAck{
		RevocationID: w.RevocationID, RuntimeID: w.RuntimeID, BootID: w.BootID, Generation: w.Generation,
		Subject:      startgate.Subject{WorkspaceID: w.WorkspaceID, Scope: startgate.Scope(w.Scope), SubjectID: w.SubjectID},
		AppliedEpoch: w.AppliedEpoch, OldStartSectionsDrained: w.OldStartSectionsDrained, AcknowledgedAt: w.AcknowledgedAt.UTC(),
	}, nil
}

// Each schema here is a flat object of required scalars; only workspace_id is
// nullable. Scan the exact property names before decoding each scalar so the
// ordinary encoding/json case folding and last-one-wins behavior cannot apply.
func exactObject(body []byte, fields map[string]any) error {
	if !utf8.Valid(body) {
		return errWire
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if head, err := decoder.Token(); err != nil || head != json.Delim('{') {
		return errWire
	}
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		target, known := fields[key]
		if err != nil || !ok || !known || seen[key] {
			return errWire
		}
		seen[key] = true
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || (key != "workspace_id" && bytes.Equal(raw, []byte("null"))) {
			return errWire
		}
		if _, date := target.(*time.Time); date {
			var value string
			if json.Unmarshal(raw, &value) != nil || !timestamp.MatchString(value) {
				return errWire
			}
		}
		if json.Unmarshal(raw, target) != nil {
			return errWire
		}
	}
	if closer, err := decoder.Token(); err != nil || closer != json.Delim('}') || len(seen) != len(fields) {
		return errWire
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errWire
	}
	return nil
}
