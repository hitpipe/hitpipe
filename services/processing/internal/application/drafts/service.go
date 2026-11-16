package drafts

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"golang.org/x/text/unicode/norm"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

var sqlIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,255}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func UUID(s string) bool { return uuidPattern.MatchString(s) }
func NewID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrUnavailable
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func Missing(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return r.IsNil()
	}
	return false
}

type Service struct {
	Repository Repository
	Authority  Authority
	Boundary   Boundary
	Bindings   Bindings
	Clock      Clock
	Codec      Codec
	Policy     Policy
}

func (s Service) Ready() bool {
	return !Missing(s.Repository) && !Missing(s.Authority) && !Missing(s.Boundary) && !Missing(s.Clock) && !Missing(s.Codec) && s.Policy.Valid()
}
func (s Service) Execute(ctx context.Context, r Request, emit func([]byte, int) error) error {
	if ctx == nil || ctx.Err() != nil || !s.Ready() || emit == nil {
		return ErrUnavailable
	}
	if !UUID(r.WorkspaceID) || (r.Kind != Save && r.Kind != Get && r.Kind != Check && r.Kind != Rehearse) || (r.Kind == Get || r.Kind == Check) && !UUID(r.DraftID) {
		return ErrInvalid
	}
	if r.Session == "" {
		return ErrUnauthenticated
	}
	if r.Kind != Get && (len(r.CSRF) < 32 || len(r.CSRF) > 128) {
		return ErrInvalid
	}
	in := Input{}
	var e error
	if r.Kind != Get {
		in, e = s.Codec.Decode(r.Kind, r.Body)
		if e != nil {
			return e
		}
		if r.Kind == Save {
			r.DraftID = in.Config.DraftID
		} else if r.Kind == Rehearse {
			r.DraftID = in.DraftID
		}
		if r.IfMatch != nil && (in.Command.ExpectedRevision == nil || *r.IfMatch != *in.Command.ExpectedRevision) {
			return ErrStale
		}
	}
	grant, e := s.Authority.Authorize(ctx, Access{r.Kind, r.WorkspaceID, r.DraftID, r.Session, r.CSRF})
	if e != nil {
		return e
	}
	valid := func() bool {
		now := s.Clock.Now()
		return !now.IsZero() && grant.Allowed && !grant.RevocationPending && grant.WorkspaceID == r.WorkspaceID && UUID(grant.PrincipalID) && UUID(grant.SessionID) && grant.Epoch > 0 && grant.Epoch <= 9007199254740991 && grant.ExpiresAt.After(now) && (grant.Role == "OWNER" || (r.Kind == Get && (grant.Role == "OPERATOR" || grant.Role == "READER"))) && (r.Kind == Get || grant.CSRFVerified)
	}
	if !valid() {
		return ErrDenied
	}
	protected := func(phase string, work func(context.Context) error) error {
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		var called, active atomic.Bool
		active.Store(true)
		defer active.Store(false)
		var actionErr error
		err := s.Boundary.Start(bounded, grant, phase, func(c context.Context) error {
			if !active.Load() || !called.CompareAndSwap(false, true) || c == nil || c.Err() != nil || bounded.Err() != nil || !valid() {
				return ErrDenied
			}
			w, stop := context.WithCancel(bounded)
			defer stop()
			end := context.AfterFunc(c, stop)
			defer end()
			actionErr = work(w)
			return actionErr
		})
		if actionErr != nil {
			return actionErr
		}
		if err != nil {
			return err
		}
		if !called.Load() || bounded.Err() != nil {
			return ErrDenied
		}
		return nil
	}
	var snap Snapshot
	if e = protected("DRAFT_SQL_CONTENT_READ", func(c context.Context) error { var e error; snap, e = s.Repository.Load(c, r, in, grant); return e }); e != nil {
		return e
	}
	status := 200
	if r.Kind == Save {
		status = 201
	}
	var body []byte
	if snap.Replay != nil {
		body = snap.Replay
	} else if r.Kind == Get {
		body, e = s.Codec.Encode(Get, snap.Config)
	} else {
		now := s.Clock.Now()
		candidate := Candidate{Config: snap.Config, ConfigDigest: snap.ConfigDigest, ContentExpiry: snap.ConfigExpiry}
		var view any
		switch r.Kind {
		case Save:
			cfg := in.Config
			if snap.Exists {
				if in.Command.ExpectedRevision == nil || *in.Command.ExpectedRevision != snap.Config.Revision || cfg.Revision != snap.Config.Revision {
					return ErrStale
				}
				cfg.Revision = snap.Config.Revision + 1
			} else if in.Command.ExpectedRevision != nil || cfg.Revision != 1 {
				return ErrStale
			}
			if cfg.Revision > 9007199254740991 {
				return ErrStale
			}
			candidate.Config = cfg
			candidate.ConfigDigest, e = s.Codec.ConfigDigest(cfg)
			if e != nil {
				return e
			}
			candidate.ContentExpiry = now.Add(s.Policy.ContentTTL).UTC().Truncate(time.Microsecond)
			view = cfg
		case Check:
			if in.Command.ExpectedRevision == nil || *in.Command.ExpectedRevision != snap.Config.Revision {
				return ErrStale
			}
			var proof BindingProof
			var proofErr error
			// No SQL transaction/row lock is held across an authoritative owner lookup.
			if Missing(s.Bindings) {
				proofErr = ErrUnavailable
			} else {
				call, cancel := context.WithTimeout(ctx, 2*time.Second)
				proof, proofErr = s.Bindings.Check(call, grant, BindingQuery{str(snap.Config.SourceConnectionID), str(snap.Config.DestinationConnectionID), str(snap.Config.Mode), str(snap.Config.PriceAlias), str(snap.Config.ListAlias), str(snap.Config.AssigneeAlias)})
				cancel()
			}
			now = s.Clock.Now()
			check, e := s.readiness(r, in, grant, snap, proof, proofErr, now)
			if e != nil {
				return e
			}
			view = check
			if check.Passed {
				candidate.PositiveValidUntil = &check.ExpiresAt
				candidate.BindingCheck = &proof
			}
		case Rehearse:
			if in.Command.ExpectedRevision == nil || *in.Command.ExpectedRevision != snap.Config.Revision || in.DraftRevision != snap.Config.Revision {
				return ErrStale
			}
			id, e := NewID()
			if e != nil {
				return e
			}
			out := []string{}
			for _, ex := range in.Examples {
				out = append(out, RehearseExample(snap.Config, ex, now))
			}
			view = RehearsalResult{RehearsalID: id, DraftID: r.DraftID, DraftRevision: snap.Config.Revision, ConfigDigest: snap.ConfigDigest, Results: out, LiveRequestsMade: 0}
		}
		candidate.Response, e = s.Codec.Encode(r.Kind, view)
		if e != nil {
			return e
		}
		if e = protected("DRAFT_COMMAND_COMMIT", func(c context.Context) error {
			var e error
			body, e = s.Repository.Commit(c, r, in, grant, snap, candidate)
			return e
		}); e != nil {
			return e
		}
	}
	if e != nil {
		return e
	}
	if e = s.Codec.ValidateResponse(r.Kind, body); e != nil {
		return e
	}
	return protected("DRAFT_RESPONSE_START", func(c context.Context) error {
		if c.Err() != nil {
			return c.Err()
		}
		return emit(body, status)
	})
}
func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func complete(c DraftConfig) bool {
	return c.SourceConnectionID != nil && c.DestinationConnectionID != nil && c.Mode != nil && c.PriceAlias != nil && c.ListAlias != nil && c.AssigneeAlias != nil && c.RequiredContextKeys != nil && c.ServiceLabel != nil && c.TitlePattern != nil
}
func (s Service) readiness(r Request, in Input, g Grant, snap Snapshot, p BindingProof, proofErr error, now time.Time) (ReadinessCheck, error) {
	id, e := NewID()
	if e != nil {
		return ReadinessCheck{}, e
	}
	out := ReadinessCheck{CheckID: id, DraftID: r.DraftID, DraftRevision: snap.Config.Revision, ConfigDigest: snap.ConfigDigest, CheckedAt: now.UTC(), ExpiresAt: now.Add(s.Policy.CheckTTL), Issues: []Error{}}
	for _, t := range []time.Time{snap.ConfigExpiry, g.ExpiresAt} {
		if t.Before(out.ExpiresAt) {
			out.ExpiresAt = t
		}
	}
	issue := func(code string) {
		out.Issues = append(out.Issues, Error{Code: code, Message: "Draft configuration could not be verified", RequestID: in.Command.RequestID})
	}
	c := snap.Config
	if !complete(c) {
		issue("CONFIG_INCOMPLETE")
	}
	label := str(c.ServiceLabel)
	if strings.ContainsAny(label, "\r\n") || strings.IndexFunc(label, unicode.IsControl) >= 0 || !norm.NFC.IsNormalString(label) {
		issue("CATALOG_LABEL_INVALID")
	}
	if proofErr != nil {
		issue("OWNER_CHECK_UNAVAILABLE")
	} else if !p.Granted || p.WorkspaceID != g.WorkspaceID || p.PrincipalID != g.PrincipalID || p.Epoch != g.Epoch || p.SourceConnectionID != str(c.SourceConnectionID) || p.DestinationConnectionID != str(c.DestinationConnectionID) || p.Mode != str(c.Mode) || p.PriceAlias != str(c.PriceAlias) || p.ListAlias != str(c.ListAlias) || p.AssigneeAlias != str(c.AssigneeAlias) || !sqlIDPattern.MatchString(p.SourceAccountID) || !sqlIDPattern.MatchString(p.DestinationWorkspaceID) || !sqlIDPattern.MatchString(p.PriceID) || !sqlIDPattern.MatchString(p.ListID) || !sqlIDPattern.MatchString(p.AssigneeID) || p.CheckedAt.IsZero() || p.CheckedAt.Before(snap.LoadedAt) || p.CheckedAt.After(now) || !p.ExpiresAt.After(now) {
		issue("OWNER_BINDINGS_UNVERIFIED")
	} else if p.ExpiresAt.Before(out.ExpiresAt) {
		out.ExpiresAt = p.ExpiresAt
	}
	out.ExpiresAt = out.ExpiresAt.UTC().Truncate(time.Microsecond)
	if now.IsZero() || !out.ExpiresAt.After(now) {
		return out, ErrUnavailable
	}
	out.Passed = len(out.Issues) == 0
	return out, nil
}

// Rehearsal is a pure labelled synthetic adapter to the existing financial
// kernel. It creates no SourceFacts proof, real identity, case or connector port.
// RehearseExample reuses the existing pure synthetic evaluator at activation.
func RehearseExample(c DraftConfig, ex ScenarioExample, now time.Time) string {
	return rehearse(c, ex, now)
}
func rehearse(c DraftConfig, ex ScenarioExample, now time.Time) string {
	if !complete(c) || ex.Duplicate {
		return "WOULD_HOLD_UNKNOWN"
	}
	if len(ex.MissingContextKeys) > 0 {
		return "WOULD_WAIT"
	}
	live := str(c.Mode) == "LIVE"
	policy := eligibility.Policy{ConfigID: c.DraftID, SchemaVersion: "1.0", EvaluatorVersion: "paid-order-v1", AccountID: "synthetic_account", ConnectionID: str(c.SourceConnectionID), LiveMode: &live, PriceID: "synthetic_price", ActivationAt: now.Add(-time.Hour)}
	// This data is explicitly hypothetical, never a verified Stripe read/proof.
	facts := syntheticFacts(policy, ex, now)
	result := eligibility.Evaluate(policy, facts, now)
	if result.Status == eligibility.Valid {
		return "WOULD_CONFIRM"
	}
	if result.Reason == "PAYMENT_STATUS" {
		return "WOULD_WAIT"
	}
	if result.Status == eligibility.Ineligible {
		return "WOULD_EXCLUDE"
	}
	return "WOULD_HOLD_UNKNOWN"
}
