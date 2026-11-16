package draftjson

import (
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

type ActivationCodec struct{}

func (ActivationCodec) Decode(b []byte) (activation.Input, error) {
	var in activation.Input
	if Validate("ActivationCommand", b) != nil {
		return in, drafts.ErrInvalid
	}
	raw, e := jcs.Transform(b)
	if e != nil || json.Unmarshal(raw, &in.Command) != nil {
		return in, drafts.ErrInvalid
	}
	in.Digest = Hash(raw)
	return in, nil
}
func (ActivationCodec) ValidateResponse(b []byte) error {
	if Validate("Operation", b) != nil {
		return drafts.ErrUnavailable
	}
	return nil
}
