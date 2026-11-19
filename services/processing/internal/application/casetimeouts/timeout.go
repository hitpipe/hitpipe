// Package casetimeouts owns the accepted age limits for Processing cases.
package casetimeouts

import "time"

const (
	WorkingLimit = 300 * time.Second
	WaitingLimit = 24 * time.Hour

	WorkingElapsed = "CASE_WORKING_DEADLINE_REACHED"
	WaitingElapsed = "CASE_WAITING_DATA_DEADLINE_REACHED"
	HistoryMissing = "CASE_TIMER_HISTORY_MISSING"
	HistoryInvalid = "CASE_TIMER_HISTORY_INVALID"
	HistoryExpired = "CASE_TIMER_HISTORY_EXPIRED"
	EpisodeMissing = "CASE_TIMER_EPISODE_MISSING"
	EpisodeInvalid = "CASE_TIMER_EPISODE_INVALID"
)

type Source struct {
	State, EnteredState             string
	Admitted, Entered               time.Time
	EpisodeStarted, EpisodeDeadline time.Time
	OpenEpisodes                    int
}

// Deadline qualifies durable timestamps. History content provenance is checked
// by the owner adapter. No arrival, retry or process boot supplies a timestamp.
func (s Source) Deadline(now time.Time) (time.Time, string) {
	if s.Entered.IsZero() {
		return time.Time{}, HistoryMissing
	}
	if s.EnteredState != s.State || s.Entered.Before(s.Admitted) || s.Entered.After(now) {
		return time.Time{}, HistoryInvalid
	}
	if s.State == "WAITING_DATA" {
		return s.Entered.Add(WaitingLimit), ""
	}
	if s.OpenEpisodes == 0 {
		return time.Time{}, EpisodeMissing
	}
	if s.OpenEpisodes != 1 || s.EpisodeStarted.Before(s.Admitted) || s.EpisodeStarted.After(s.Entered) ||
		!s.EpisodeDeadline.After(s.EpisodeStarted) || s.EpisodeDeadline.Before(s.Entered) ||
		s.EpisodeDeadline.After(s.EpisodeStarted.Add(WorkingLimit)) {
		return time.Time{}, EpisodeInvalid
	}
	return s.EpisodeDeadline, ""
}

type Decision struct {
	State, Reason string
	Due           time.Time
}

func Decide(s Source, now time.Time, problem string) Decision {
	due, reason := s.Deadline(now)
	if reason == "" {
		reason = problem
	}
	if reason != "" {
		return Decision{State: s.State, Reason: reason}
	}
	if now.Before(due) {
		return Decision{}
	}
	if s.State == "WAITING_DATA" {
		return Decision{State: "ATTENTION", Reason: WaitingElapsed, Due: due}
	}
	return Decision{State: "UNKNOWN", Reason: WorkingElapsed, Due: due}
}

func SourceHeld(reason string) bool {
	switch reason {
	case HistoryMissing, HistoryInvalid, HistoryExpired, EpisodeMissing, EpisodeInvalid:
		return true
	default:
		return false
	}
}
