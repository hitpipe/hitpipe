package postgres

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

func TestProcessingCaseTimeoutsOwnedStand(t *testing.T) {
	binary := os.Getenv("HIT111_PROCESSING_BIN")
	if binary == "" {
		t.Skip("requires isolated PG18.7 and executable")
	}
	now := time.Now().UTC()
	f := preparedSince(t, []string{}, now.Add(-72*time.Hour))
	foreign := preparedSince(t, []string{}, now.Add(-72*time.Hour))
	ws := f.request.WorkspaceID
	// Start the executable one minute into the accepted five-minute window.
	due := now.Add(-time.Minute).Truncate(time.Microsecond)
	expiring := seedTimerPeriod(t, f, "WAITING_DATA", due.Add(-24*time.Hour), "")
	broken := seedTimerPeriod(t, f, "WAITING_DATA", due.Add(-24*time.Hour), "corrupt")
	expired := seedTimerPeriod(t, f, "WAITING_DATA", due.Add(-24*time.Hour), "expired")
	noEpisode := seedTimerPeriod(t, f, "WORKING", now.Add(-time.Minute), "missing_episode")
	future := seedTimerPeriod(t, f, "WAITING_DATA", now.Add(time.Hour), "future")
	stand := createProcessStand(t, binary, f.path, map[string]string{ws: "acct_synthetic", foreign.request.WorkspaceID: "acct_synthetic"})
	old := stand.boot("timers-first.log")
	stand.pollHealth(old, "CLOSED")
	stand.iamOpen.Store(true)
	stand.pollHealth(old, "READY")
	wait := func() {
		t.Helper()
		end := time.Now().Add(6 * time.Second)
		for {
			if r := timerCase(t, ws, expiring); r.state == "ATTENTION" && r.notice == 1 &&
				timerCase(t, ws, broken).reason == casetimeouts.HistoryInvalid &&
				timerCase(t, ws, expired).reason == casetimeouts.HistoryExpired &&
				timerCase(t, ws, noEpisode).reason == casetimeouts.EpisodeMissing {
				return
			}
			if !time.Now().Before(end) {
				t.Fatalf("worker did not persist the four outcomes: elapsed=%+v broken=%+v expired=%+v missing=%+v",
					timerCase(t, ws, expiring), timerCase(t, ws, broken), timerCase(t, ws, expired), timerCase(t, ws, noEpisode))
			}
			time.Sleep(75 * time.Millisecond)
		}
	}
	wait()
	final := timerCase(t, ws, expiring)
	if final.at.Before(due) || final.at.Sub(due) > 5*time.Minute || final.intents != 0 {
		t.Fatalf("waiting limit not reached in the window: %+v", final)
	}
	if err := old.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-old.done
	again := stand.boot("timers-restart.log")
	stand.pollHealth(again, "READY")
	wait()

	h := map[string]string{"X-HitPipe-Session": stand.token}
	path := func(id string) string { return "/api/v1/workspaces/" + ws + "/cases/" + id }
	detail := func(id string) (d readcases.CaseDetail) {
		t.Helper()
		raw := stand.must("bff.test", "8443", "GET", path(id), nil, h, 200)
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		return
	}
	visible := detail(expiring)
	if visible.Case.State != "ATTENTION" || visible.Case.Revision != 3 || !visible.Case.UpdatedAt.Equal(final.at) ||
		visible.Case.DueAt != nil || visible.Case.TargetURL != nil ||
		!strings.Contains(visible.Case.NextAction, "24 часа непрерывного ожидания") {
		t.Fatal(visible)
	}
	getHistory := func(id string) readcases.HistoryView {
		t.Helper()
		var v readcases.HistoryView
		body := stand.must("bff.test", "8443", "GET", path(id)+"/history", nil, h, 200)
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	history := getHistory(expiring)
	if len(history.Items) != 3 || history.Items[2].ReasonCode != casetimeouts.WaitingElapsed ||
		history.Items[2].NewState != "ATTENTION" || !history.Items[2].At.Equal(final.at) {
		t.Fatal(history)
	}
	for _, c := range []struct{ id, state, text string }{
		{broken, "WAITING_DATA", "недоступны или несогласованны"},
		{expired, "WAITING_DATA", "просрочены"},
		{noEpisode, "WORKING", "Не найден открытый эпизод"},
	} {
		d := detail(c.id)
		if d.Case.State != c.state || d.Case.DueAt != nil || !strings.Contains(d.Case.NextAction, c.text) ||
			d.Case.TargetURL != nil || d.ObservationComplete || len(d.AvailableActions) != 0 {
			t.Fatal(d)
		}
		s := timerCase(t, ws, c.id)
		if s.notice != 1 || s.entries != 3 || s.intents != 0 || s.revision != 3 {
			t.Fatal(s)
		}
	}
	stand.must("bff.test", "8443", "GET", path(broken)+"/history", nil, h, 503)
	for _, url := range []string{path(future), path(future) + "/history"} {
		body := stand.must("bff.test", "8443", "GET", url, nil, h, 503)
		if !strings.Contains(string(body), casetimeouts.HistoryInvalid) {
			t.Fatal(string(body))
		}
	}
	items := getHistory(expired).Items
	if len(items) != 3 || items[1].ExpiresAt.After(now) || items[1].EvidenceRef != nil ||
		items[1].EpisodeID != nil || items[2].ReasonCode != casetimeouts.HistoryExpired {
		t.Fatal(items)
	}
	stand.must("bff.test", "8443", "GET", path(foreign.request.CaseID), nil, h, 404)
	stand.must("bff.test", "8443", "GET", path(foreign.request.CaseID)+"/history", nil, h, 404)
	if stand.deliveryCount.Load() != 0 {
		t.Fatal("timer created or resent an intent")
	}
	t.Log("PROCESS PASS: actual worker timed out waiting inside five minutes, preserved reason and exactly one notification across kill/restart, held corrupt/expired history and missing episode, owner mTLS views and cross-workspace denial")
}
