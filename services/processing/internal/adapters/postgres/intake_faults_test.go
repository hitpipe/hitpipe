package postgres

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/jackc/pgx/v5"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Actual transaction result faults; a successful Commit is never fabricated.
type intakeFaultDB struct {
	Beginner
	mode string
}
type intakeFaultTx struct {
	pgx.Tx
	mode string
}

func (d intakeFaultDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.Beginner.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return intakeFaultTx{tx, d.mode}, nil
}
func (tx intakeFaultTx) Commit(c context.Context) error {
	if tx.mode == "abort" {
		_, _ = tx.Tx.Exec(c, `SELECT 1/0`)
	}
	if e := tx.Tx.Commit(c); e != nil {
		return e
	}
	if tx.mode == "crash" {
		os.Exit(24)
	}
	if tx.mode == "ack_loss" {
		return errors.New("controlled lost PostgreSQL commit acknowledgement")
	}
	return nil
}
func TestIntakeHTTPErrorsCommitAckAndProcessCrash(t *testing.T) {
	if os.Getenv("HIT104_CRASH_HELPER") == "1" {
		f := intakeAt(t, os.Getenv("HIT104_WS"), os.Getenv("HIT104_ROOT"))
		raw, e := base64.StdEncoding.DecodeString(os.Getenv("HIT104_WIRE"))
		if e != nil {
			t.Fatal(e)
		}
		f.repo.DB = intakeFaultDB{f.repo.DB, "crash"}
		f.service.Store = f.repo
		f.start(t)
		_, _, _ = f.call(t, raw, "POST", "/internal/v1/intake-signals", nil)
		t.Fatal("crash before HTTP receipt did not occur")
	}
	for _, mode := range []string{"abort", "ack_loss", "closed_connection", "missing_content", "crash"} {
		t.Run(mode, func(t *testing.T) {
			f := newIntake(t)
			raw := intakeWire(t, f.ws, nil)
			if mode == "crash" {
				exe, _ := os.Executable()
				cmd := exec.Command(exe, "-test.run=^TestIntakeHTTPErrorsCommitAckAndProcessCrash$")
				cmd.Env = append(os.Environ(), "HIT104_CRASH_HELPER=1", "HIT104_WS="+f.ws, "HIT104_ROOT="+f.path, "HIT104_WIRE="+base64.StdEncoding.EncodeToString(raw))
				out, e := cmd.CombinedOutput()
				var exit *exec.ExitError
				if !errors.As(e, &exit) || exit.ExitCode() != 24 {
					t.Fatalf("no actual committed process exit24: %v %s", e, out)
				}
			} else {
				switch mode {
				case "abort", "ack_loss":
					f.repo.DB = intakeFaultDB{f.repo.DB, mode}
				case "closed_connection":
					c := connect(t)
					c.Close(ctx)
					f.repo.DB = c
				case "missing_content":
					f.files.Close()
				}
				f.service.Store = f.repo
				f.start(t)
				f.apply(t, raw, 503, "no_positive_receipt_"+mode)
			}
			n := 0
			if mode == "ack_loss" || mode == "crash" {
				n = 1
			}
			counts(t, f.ws, n, n, n)
			files, e := contentfiles.New(f.path)
			if e != nil {
				t.Fatal(e)
			}
			defer files.Close()
			f.files = files
			f.repo.Content = files
			f.repo.DB = newObservationPool(t, 2)
			f.service.Store = f.repo
			f.start(t)
			first := f.apply(t, raw, 202, "new_connection_after_"+mode)
			second := f.apply(t, raw, 202, "replay_after_"+mode)
			if !bytes.Equal(first, second) {
				t.Fatal("original receipt was not restored")
			}
			counts(t, f.ws, 1, 1, 1)
		})
	}
}
func TestIntakeHTTPDurableFileFailuresAndExpiryRestart(t *testing.T) {
	for _, step := range []string{"write", "file-fsync", "rename", "directory-fsync"} {
		t.Run(step, func(t *testing.T) {
			f := newIntake(t)
			hit := false
			f.files.Step = func(s string) error {
				if s == step {
					hit = true
					return errors.New("controlled Intake HISTORY write failure")
				}
				return nil
			}
			raw := intakeWire(t, f.ws, nil)
			f.apply(t, raw, 503, "file_"+step)
			if !hit {
				t.Fatal("fault not reached")
			}
			counts(t, f.ws, 0, 0, 0)
			f.files.Step = nil
			f.apply(t, raw, 202, "retry_after_file_"+step)
			counts(t, f.ws, 1, 1, 1)
		})
	}
	f := newIntake(t)
	f.repo.HistoryRetention = 300 * time.Millisecond
	f.service.Store = f.repo
	f.start(t)
	raw := intakeWire(t, f.ws, nil)
	first := f.apply(t, raw, 202, "short_expiring_history")
	time.Sleep(400 * time.Millisecond)
	if e := f.files.PurgeExpired(time.Now()); e != nil {
		t.Fatal(e)
	}
	files, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer files.Close()
	f.files = files
	f.repo.Content = files
	f.repo.DB = newObservationPool(t, 1)
	f.service.Store = f.repo
	f.start(t)
	again := f.apply(t, raw, 202, "full_original_receipt_after_detail_purge_restart")
	if !bytes.Equal(first, again) {
		t.Fatal("TTL extended or receipt lost")
	}
	counts(t, f.ws, 1, 1, 1)
}

// Store cancellation must not become APPLIED even when an implementation returns
// an empty Receipt; the application only emits after successful real acceptance.
var _ admission.Store = Store{}
