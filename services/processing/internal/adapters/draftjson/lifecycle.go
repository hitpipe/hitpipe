package draftjson

import (
	"encoding/json"
	"unicode"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
)

func (Codec) DecodeLifecycle(body []byte) (lifecycle.Input, error) {
	var in lifecycle.Input
	if Validate("LifecycleCommand", body) != nil {
		return in, drafts.ErrInvalid
	}
	canonical, err := jcs.Transform(body)
	if err != nil || json.Unmarshal(canonical, &in.LifecycleCommand) != nil {
		return in, drafts.ErrInvalid
	}
	if in.Reason != nil && !LifecycleText(*in.Reason) {
		return in, drafts.ErrInvalid
	}
	switch in.Action {
	case lifecycle.Pause, lifecycle.Prepare:
		if in.ResumeSnapshotID != nil || in.HeldDisposition != nil {
			return in, drafts.ErrInvalid
		}
	case lifecycle.Accept:
		if in.ResumeSnapshotID == nil || in.HeldDisposition == nil {
			return in, drafts.ErrInvalid
		}
	default:
		return in, drafts.ErrConflict
	}
	in.Digest = Hash(canonical)
	return in, nil
}

func LifecycleText(s string) bool {
	if KnownSecret(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func (Codec) EncodeLifecycle(kind drafts.Kind, value any) ([]byte, error) {
	schema := ""
	switch kind {
	case lifecycle.List:
		schema = "Operations"
	case lifecycle.Snapshot:
		schema = "ResumeSnapshot"
	case lifecycle.Change:
		schema = "Operation"
	}
	body, err := json.Marshal(value)
	if err != nil || Validate(schema, body) != nil {
		return nil, drafts.ErrUnavailable
	}
	return jcs.Transform(body)
}
