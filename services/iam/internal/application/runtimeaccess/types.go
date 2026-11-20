// Package runtimeaccess defines the trusted inputs and private runtime wire types.
package runtimeaccess

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
)

var (
	ErrInvalid     = errors.New("runtime_request_invalid")
	ErrDenied      = errors.New("runtime_authority_denied")
	ErrConflict    = errors.New("runtime_registration_conflict")
	ErrStale       = errors.New("runtime_snapshot_stale")
	ErrUnavailable = errors.New("runtime_state_unavailable")
)

type Peer struct {
	DNSName                string
	CertificateFingerprint string
}

type Registration struct {
	RuntimeID              string `json:"runtime_id"`
	Service                string `json:"service"`
	Generation             int64  `json:"generation"`
	BootID                 string `json:"boot_id"`
	CertificateFingerprint string `json:"certificate_fingerprint"`
}

// Binding comes from explicit runtime provisioning, outside request input.
type Binding struct {
	Registration
	DNSName string `json:"dns_name"`
}

type Denial struct {
	WorkspaceID  *string    `json:"workspace_id"`
	Scope        string     `json:"scope"`
	SubjectID    string     `json:"subject_id"`
	Epoch        int64      `json:"epoch"`
	RevocationID string     `json:"revocation_id"`
	AcceptedAt   time.Time  `json:"accepted_at"`
	EffectiveAt  *time.Time `json:"effective_at"`
}

type Snapshot struct {
	RuntimeID  string   `json:"runtime_id"`
	Generation int64    `json:"generation"`
	BootID     string   `json:"boot_id"`
	Revision   int64    `json:"snapshot_revision"`
	Denials    []Denial `json:"denials"`
	Gate       string   `json:"gate"`

	// Complete is internal evidence of a full inventory and validated SQL snapshot.
	Complete bool `json:"-"`
}

func (r Registration) Valid() bool {
	return ValidUUID(r.RuntimeID) && ValidUUID(r.BootID) &&
		r.Generation > 0 && r.Generation <= authorizeaction.MaxWireInteger &&
		ValidFingerprint(r.CertificateFingerprint) && validService(r.Service)
}

func (r Registration) Normalized() Registration {
	r.RuntimeID = strings.ToLower(r.RuntimeID)
	r.BootID = strings.ToLower(r.BootID)
	return r
}

func (r Registration) Caller() authorizeaction.Caller {
	return authorizeaction.Caller{
		Service: r.Service, RuntimeID: r.RuntimeID, BootID: r.BootID,
		CertificateFingerprint: r.CertificateFingerprint, Generation: r.Generation,
	}
}

func ValidUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	var raw [16]byte
	_, err := hex.Decode(raw[:], []byte(compact))
	return err == nil && raw != ([16]byte{})
}

func ValidFingerprint(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, b := range value[len("sha256:"):] {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}

func validService(service string) bool {
	switch service {
	case "IAM", "INTAKE", "PROCESSING", "DELIVERY", "BFF", "AI":
		return true
	default:
		return false
	}
}

func ValidDNSName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, b := range label {
			if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-') {
				return false
			}
		}
	}
	return true
}
