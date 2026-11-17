package draftjson

import (
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

func PreviewValid(p drafts.ActivationPreview) bool {
	raw, e := json.Marshal(p.CanonicalConfig)
	if e != nil || Validate("CanonicalAcceptedConfig", raw) != nil {
		return false
	}
	raw, e = jcs.Transform(raw)
	if e != nil || Hash(raw) != p.AcceptedConfigDigest {
		return false
	}
	c := p.CanonicalConfig
	for _, v := range []string{c.PriceID, c.ListID, c.AssigneeID, c.DestinationWorkspaceID, c.ServiceLabel} {
		if KnownSecret(v) {
			return false
		}
	}
	return true
}
func (Codec) Preview(c drafts.DraftConfig, p drafts.BindingProof) (drafts.ActivationPreview, error) {
	v, e := drafts.CanonicalConfig(c, p)
	if e != nil {
		return drafts.ActivationPreview{}, e
	}
	raw, e := json.Marshal(v)
	if e != nil || Validate("CanonicalAcceptedConfig", raw) != nil {
		return drafts.ActivationPreview{}, drafts.ErrUnavailable
	}
	raw, e = jcs.Transform(raw)
	if e != nil {
		return drafts.ActivationPreview{}, drafts.ErrUnavailable
	}
	out := drafts.ActivationPreview{CanonicalConfig: v, AcceptedConfigDigest: Hash(raw)}
	if !PreviewValid(out) {
		return drafts.ActivationPreview{}, drafts.ErrUnavailable
	}
	return out, nil
}
