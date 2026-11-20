package postgres

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

// Older tests use an injected file-retention event on their own private fixture
// directories while their test stores remain open. The maintenance path is
// exercised with the real role in purge_test.go.
func expireFixtureFiles(path string, now time.Time) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		target := filepath.Join(path, entry.Name())
		f, err := os.Open(target)
		if err != nil {
			return err
		}
		line, err := bufio.NewReader(f).ReadBytes('\n')
		_ = f.Close()
		if err != nil {
			return err
		}
		var meta struct {
			Ref preparecase.Ref
		}
		if err = json.Unmarshal(line, &meta); err != nil {
			return err
		}
		if !meta.Ref.ExpiresAt.IsZero() && !now.Before(meta.Ref.ExpiresAt) {
			if err = os.Remove(target); err != nil {
				return err
			}
		}
	}
	return nil
}
