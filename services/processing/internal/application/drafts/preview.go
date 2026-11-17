package drafts

import (
	"golang.org/x/text/unicode/norm"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CanonicalConfig is the one pure HP-ACTIVATION-002 builder used for preview,
// its durable validation and the exact bytes committed to accepted_configs.
// Binding times are not configuration identity; no IO or provider calls occur.
func CanonicalConfig(c DraftConfig, p BindingProof) (CanonicalAcceptedConfig, error) {
	var z CanonicalAcceptedConfig
	if !complete(c) || !p.Granted || !UUID(p.WorkspaceID) || !UUID(p.PrincipalID) || p.Epoch < 1 || !UUID(p.SourceConnectionID) || !UUID(p.DestinationConnectionID) || p.SourceConnectionID != str(c.SourceConnectionID) || p.DestinationConnectionID != str(c.DestinationConnectionID) || p.Mode != str(c.Mode) || p.PriceAlias != str(c.PriceAlias) || p.ListAlias != str(c.ListAlias) || p.AssigneeAlias != str(c.AssigneeAlias) {
		return z, ErrConflict
	}
	for _, id := range []string{p.SourceAccountID, p.DestinationWorkspaceID, p.PriceID, p.ListID, p.AssigneeID} {
		if !sqlIDPattern.MatchString(id) {
			return z, ErrConflict
		}
	}
	label := str(c.ServiceLabel)
	if label != p.ServiceLabel || !utf8.ValidString(label) || label == "" || strings.IndexFunc(label, unicode.IsControl) >= 0 || !norm.NFC.IsNormalString(label) {
		return z, ErrConflict
	}
	keys := append([]string{}, (*c.RequiredContextKeys)...)
	return CanonicalAcceptedConfig{PackID: "PAID_ORDER_TASK_V1", SourceConnectionID: p.SourceConnectionID, DestinationConnectionID: p.DestinationConnectionID, DestinationWorkspaceID: p.DestinationWorkspaceID, PriceID: p.PriceID, ListID: p.ListID, AssigneeID: p.AssigneeID, Mode: p.Mode, RequiredContextKeys: keys, TitlePattern: str(c.TitlePattern), ServiceLabel: label}, nil
}

// SameBinding compares the meaning accepted under the check, never fresh
// CheckedAt/ExpiresAt timestamps from an otherwise identical owner lookup.
func SameBinding(a, b BindingProof) bool {
	return a.Granted && b.Granted && a.WorkspaceID == b.WorkspaceID && a.PrincipalID == b.PrincipalID && a.Epoch == b.Epoch && a.SourceConnectionID == b.SourceConnectionID && a.DestinationConnectionID == b.DestinationConnectionID && a.Mode == b.Mode && a.PriceAlias == b.PriceAlias && a.ListAlias == b.ListAlias && a.AssigneeAlias == b.AssigneeAlias && a.SourceAccountID == b.SourceAccountID && a.DestinationWorkspaceID == b.DestinationWorkspaceID && a.PriceID == b.PriceID && a.ListID == b.ListID && a.AssigneeID == b.AssigneeID && a.ServiceLabel == b.ServiceLabel
}
