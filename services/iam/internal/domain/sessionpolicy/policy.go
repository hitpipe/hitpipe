// Package sessionpolicy evaluates a minimal authority-owned session snapshot.
// It performs no IO, reads no clock and grants no business authorization.
package sessionpolicy

import "time"

// MaxAbsoluteLifetime is the absolute lifetime bound from HP-BP-001/T/002.
const MaxAbsoluteLifetime = 24 * time.Hour

// Snapshot is a value projection, not the public Session wire schema or a cookie.
// The authority computes IdleExpiresAt from activity; this policy does not renew it.
type Snapshot struct {
	Found         bool
	Revoked       bool
	CreatedAt     time.Time
	ExpiresAt     time.Time
	IdleExpiresAt time.Time
}

// Observation carries explicit time and its caller-established trust status.
type Observation struct {
	At      time.Time
	Trusted bool
}

// Usable reports whether the supplied observation can be used for a decision.
// time.Now alone does not establish Trusted.
func (o Observation) Usable() bool { return o.Trusted && usableTime(o.At) }

// State is session validity only, never permission for a protected action.
type State string

const (
	StateValid  State = "valid"
	StateDenied State = "denied"
)

// Reason is a bounded diagnostic code; it never contains input or identity data.
type Reason string

const (
	ReasonValid           Reason = "session_valid"
	ReasonClockUncertain  Reason = "clock_uncertain"
	ReasonUnknown         Reason = "unknown_session"
	ReasonRevoked         Reason = "revoked_session"
	ReasonInvalidSnapshot Reason = "invalid_snapshot"
	ReasonNotYetValid     Reason = "not_yet_valid"
	ReasonAbsoluteExpired Reason = "absolute_expired"
	ReasonIdleExpired     Reason = "idle_expired"
)

// Decision owns only its state and safe reason. The zero value is not valid.
type Decision struct {
	State  State  `json:"state"`
	Reason Reason `json:"reason"`
}

// IsValid requires the complete positive decision, not a default bool value.
func (d Decision) IsValid() bool { return d.State == StateValid && d.Reason == ReasonValid }

func deny(reason Reason) Decision { return Decision{State: StateDenied, Reason: reason} }

func usableTime(t time.Time) bool {
	return !t.IsZero() && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
}

// Evaluate implements DK-RULE-01/02 without modifying either argument.
// Precedence: clock, unknown, revoked, malformed snapshot, future creation,
// absolute expiry, idle expiry, valid. Equality at either deadline is expired.
func Evaluate(snapshot Snapshot, observation Observation) Decision {
	if !observation.Usable() {
		return deny(ReasonClockUncertain)
	}
	if !snapshot.Found {
		return deny(ReasonUnknown)
	}
	if snapshot.Revoked {
		return deny(ReasonRevoked)
	}
	created := snapshot.CreatedAt.UTC()
	absolute := snapshot.ExpiresAt.UTC()
	idle := snapshot.IdleExpiresAt.UTC()
	now := observation.At.UTC()
	if !usableTime(created) || !usableTime(absolute) || !usableTime(idle) ||
		!absolute.After(created) || !idle.After(created) || idle.After(absolute) ||
		absolute.Sub(created) > MaxAbsoluteLifetime {
		return deny(ReasonInvalidSnapshot)
	}
	if now.Before(created) {
		return deny(ReasonNotYetValid)
	}
	if !now.Before(absolute) {
		return deny(ReasonAbsoluteExpired)
	}
	if !now.Before(idle) {
		return deny(ReasonIdleExpired)
	}
	return Decision{State: StateValid, Reason: ReasonValid}
}
