package dispatch

import (
	"net/http"
	"testing"
	"time"
)

func TestPublicationRetryAfter(t *testing.T) {
	now := time.Date(2026, time.November, 23, 12, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	for _, test := range []struct {
		name  string
		value string
		want  time.Time
	}{
		{"empty", "", time.Time{}},
		{"zero", "0", now},
		{"seconds", " 360 ", now.Add(6 * time.Minute)},
		{"date", now.Add(10 * time.Minute).Format(http.TimeFormat), now.Add(10 * time.Minute)},
		{"invalid", "-1", time.Time{}},
		{"oversized", "99999999999999999999999", expires},
		{"beyond expiry", expires.Add(time.Second).Format(http.TimeFormat), expires},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := retryAfter(test.value, now, expires)
			if got == nil && !test.want.IsZero() || got != nil && !got.Equal(test.want) {
				t.Fatalf("retryAfter(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestPublicationBackoffBounds(t *testing.T) {
	for _, test := range []struct {
		attempt int
		ceiling time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{7, 128 * time.Second},
		{9, 256 * time.Second},
	} {
		for range 32 {
			delay := publicationBackoff(test.attempt)
			if delay < test.ceiling/2 || delay >= test.ceiling {
				t.Fatalf("attempt %d delay %v outside half-to-full range for %v", test.attempt, delay, test.ceiling)
			}
		}
	}
}
