package postgres

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// The driver owns PostgreSQL and fixtures, never a Docker socket. The host
// acknowledges each lifecycle step after checking the actual OCI container.
func TestProcessingOCIRecoveryOwnedStand(t *testing.T) {
	revision, purge := os.Getenv("HIT155_SOURCE_SHA"), os.Getenv("HIT126_PURGE_BIN")
	if revision == "" || purge == "" || os.Getenv("HIT78_ISOLATED_PG") != "1" || os.Getenv("HIT128_RECOVERY_STAND") != "1" {
		t.Skip("requires the private HIT-155 OCI recovery runner")
	}
	if os.Geteuid() != 0 {
		t.Fatal("the fixture driver requires root to initialize the disposable restore cluster")
	}
	activation := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	f := observePrepared(t, preparedIn(t, []string{}, activation, imageContent))
	verifyProcessingRecovery(t, f, "", purge, func(stand *processStand) recoveryDriver {
		step := 0
		signal := func(operation string, args []string) {
			t.Helper()
			step++
			index := strconv.Itoa(step)
			writeImageSignal(t, "recovery-"+index+".json", map[string]any{"operation": operation, "args": args})
			awaitImageSignal(t, "recovery-ack-"+index)
		}
		return recoveryDriver{
			boot: func(_ string, args ...string) { signal("start", args) },
			health: func(label, state string) {
				t.Log(label)
				imageHealth(t, stand, state, revision)
			},
			stop: func() { signal("kill", nil) },
		}
	})
}
