package profileupdate

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const requestBudget = 2 * time.Second

type Service struct {
	store     Store
	content   Content
	gate      Gate
	authority CommandAuthority
	reference func(string) (string, error)
}

func missing(value any) bool {
	if value == nil {
		return true
	}
	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// New leaves PATCH closed when no global command authority is provisioned.
func New(store Store, content Content, gate Gate, authority CommandAuthority, reference func(string) (string, error)) (*Service, error) {
	if missing(store) || missing(content) || missing(gate) || reference == nil {
		return nil, ErrUnavailable
	}
	return &Service{store: store, content: content, gate: gate, authority: authority, reference: reference}, nil
}

func revision(value int64) bool { return value > 0 && value <= authorizeaction.MaxWireInteger }

func textValue(value string, max int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= max
}

func validPointer(value Pointer) bool {
	return runtimeaccess.ValidUUID(value.Ref) && strings.ToLower(value.Ref) == value.Ref && runtimeaccess.ValidFingerprint(value.Digest)
}

func digest(command Command) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"command":      map[string]any{"request_id": command.RequestID, "expected_revision": command.ExpectedRevision},
		"display_name": command.DisplayName, "timezone": command.Timezone,
	})
	if err != nil {
		return "", ErrInvalid
	}
	canonical := jsontext.Value(raw)
	if err := canonical.Canonicalize(); err != nil {
		return "", ErrInvalid
	}
	hash := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func (s *Service) authenticate(ctx context.Context, peer runtimeaccess.Peer, opaqueSession, csrf string) (Identity, error) {
	caller, err := s.gate.Caller(ctx, peer)
	if err != nil {
		return Identity{}, err
	}
	if !caller.Valid() || caller.Service != "BFF" {
		return Identity{}, ErrDenied
	}
	ref, err := s.reference(opaqueSession)
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}
	local := s.gate.Local()
	who, err := s.store.Identity(ctx, ref, caller, local)
	if err != nil {
		return Identity{}, err
	}
	if who.Caller != caller || who.Local != local || who.Reference != ref || !runtimeaccess.ValidUUID(who.SessionID) ||
		!runtimeaccess.ValidUUID(who.PrincipalID) || !revision(who.SessionEpoch) || who.ValidUntil.IsZero() {
		return Identity{}, ErrUnavailable
	}
	hash := sha256.Sum256([]byte(csrf))
	if subtle.ConstantTimeCompare(who.CSRFHash[:], hash[:]) != 1 {
		return Identity{}, ErrDenied
	}
	return who, nil
}

func (s *Service) read(ctx context.Context, who Identity, ptr Pointer) (getprofile.Profile, error) {
	if ptr == (Pointer{}) {
		return getprofile.Profile{}, ErrNotFound
	}
	if !validPointer(ptr) {
		return getprofile.Profile{}, ErrUnavailable
	}
	p, err := s.content.Read(ctx, ptr.Ref, ptr.Digest)
	if err != nil || ctx.Err() != nil || !strings.EqualFold(p.UserID, who.PrincipalID) || !revision(p.Revision) ||
		!textValue(p.DisplayName, 120) || !textValue(p.Timezone, 80) {
		return getprofile.Profile{}, ErrUnavailable
	}
	p.UserID = who.PrincipalID
	return p, nil
}

func (s *Service) checkPrepared(ctx context.Context, who Identity, cmd Command, receipt Receipt) error {
	prepared, err := s.read(ctx, who, receipt.After)
	if err != nil || prepared.Revision != receipt.Revision || prepared.DisplayName != cmd.DisplayName || prepared.Timezone != cmd.Timezone {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) apply(ctx context.Context, tx ProfileTx, record CommandRecord, who Identity, cmd Command, bodyDigest string) (Receipt, error) {
	current := tx.Pointer()
	receipt, found, err := record.Load(ctx)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if found {
		if !runtimeaccess.ValidFingerprint(receipt.BodyDigest) || !validPointer(receipt.Before) || !validPointer(receipt.After) ||
			receipt.Before.Ref == receipt.After.Ref || !revision(receipt.Revision) {
			return Receipt{}, ErrUnavailable
		}
		if receipt.BodyDigest != bodyDigest {
			return Receipt{}, ErrConflict
		}
		if receipt.Revision != cmd.ExpectedRevision+1 {
			return Receipt{}, ErrUnavailable
		}
		if current == receipt.After {
			// A locked read of the exact published pointer proves the effect
			// already committed, even if the first COMMIT reply was lost.
			return receipt, s.checkPrepared(ctx, who, cmd, receipt)
		}
		if receipt.Confirmed {
			return Receipt{}, ErrStale
		}
		if current != receipt.Before {
			return Receipt{}, ErrUncertain
		}
	}

	before, err := s.read(ctx, who, current)
	if err != nil {
		return Receipt{}, err
	}
	if before.Revision != cmd.ExpectedRevision || !revision(before.Revision+1) {
		return Receipt{}, ErrStale
	}
	if found {
		if err := s.checkPrepared(ctx, who, cmd, receipt); err != nil {
			return Receipt{}, err
		}
	} else {
		after := getprofile.Profile{UserID: who.PrincipalID, DisplayName: cmd.DisplayName, Timezone: cmd.Timezone, Revision: before.Revision + 1}
		ptr, err := s.content.WriteProfile(ctx, after)
		if err != nil || !validPointer(ptr) || ptr.Ref == current.Ref || ctx.Err() != nil {
			return Receipt{}, ErrUnavailable
		}
		receipt = Receipt{BodyDigest: bodyDigest, Before: current, After: ptr, Revision: after.Revision}
		if err := s.checkPrepared(ctx, who, cmd, receipt); err != nil {
			return Receipt{}, err
		}
		if err := record.Prepare(ctx, receipt); err != nil {
			return Receipt{}, ErrUnavailable
		}
	}
	return receipt, tx.Switch(ctx, receipt.Before, receipt.After)
}

func (s *Service) Update(ctx context.Context, peer runtimeaccess.Peer, opaqueSession, csrf string, cmd Command,
	start func(context.Context, getprofile.Profile) error) error {
	requestID := strings.ToLower(cmd.RequestID)
	if ctx == nil || start == nil || !runtimeaccess.ValidUUID(requestID) || !revision(cmd.ExpectedRevision) ||
		!textValue(cmd.DisplayName, 120) || !textValue(cmd.Timezone, 80) || len(csrf) < 32 || len(csrf) > 128 {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, requestBudget)
	defer cancel()
	who, err := s.authenticate(ctx, peer, opaqueSession, csrf)
	if err != nil {
		return err
	}
	if missing(s.authority) {
		return ErrUnavailable
	}
	bodyDigest, err := digest(cmd)
	if err != nil {
		return err
	}
	return s.authority.WithCommand(ctx, who.PrincipalID, requestID, func(commandCtx context.Context, record CommandRecord) error {
		if commandCtx == nil || commandCtx.Err() != nil || missing(record) {
			return ErrUnavailable
		}
		var receipt Receipt
		err := s.gate.Within(commandCtx, peer, who, func(work context.Context) error {
			return s.store.WithProfile(work, who, func(guard context.Context, tx ProfileTx) error {
				var err error
				receipt, err = s.apply(guard, tx, record, who, cmd, bodyDigest)
				return err
			})
		})
		if err != nil {
			return err
		}
		if !receipt.Confirmed && record.Confirm(commandCtx, receipt) != nil {
			// The SQL effect is already known. Without durable completion
			// evidence let the next request reconcile the prepared intent.
			return ErrUncertain
		}
		return s.gate.Within(commandCtx, peer, who, func(work context.Context) error {
			return s.store.WithProfile(work, who, func(guard context.Context, tx ProfileTx) error {
				if tx.Pointer() != receipt.After {
					return ErrStale
				}
				profile, err := s.read(guard, who, receipt.After)
				if err != nil || profile.Revision != receipt.Revision || guard.Err() != nil {
					return ErrUnavailable
				}
				return start(guard, profile)
			})
		})
	})
}
