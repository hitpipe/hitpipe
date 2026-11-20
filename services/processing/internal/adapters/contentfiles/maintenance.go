package contentfiles

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

const MaxPurgeBatch = 1000

// Maintenance has the exclusive directory lease. Do not run it alongside
// previous builds that have no lease, or against a copy of a live volume.
type Maintenance struct{ files *Store }

func OpenMaintenance(path string) (*Maintenance, error) {
	s, err := openStore(path, syscall.LOCK_EX)
	if err != nil {
		return nil, err
	}
	return &Maintenance{files: s}, nil
}

func (m *Maintenance) Close() error { return m.files.Close() }

// Sweep scans in bounded chunks and unlinks at most limit entries. The callback
// must recheck the SQL references and hold their write barrier across unlink.
// Incomplete or unowned metadata cannot authorize deletion.
func (m *Maintenance) Sweep(ctx context.Context, workspace string, now time.Time, limit int, canRemove func(string) (bool, error)) (removed int, err error) {
	if !originID.MatchString(workspace) || now.IsZero() || limit < 1 || limit > MaxPurgeBatch || canRemove == nil {
		return 0, preparecase.ErrContent
	}
	dir, err := m.files.root.Open(".")
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	defer func() {
		if removed != 0 {
			err = errors.Join(err, m.files.syncDir())
		}
	}()
	var incident bool
	for removed < limit {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		entries, readErr := dir.ReadDir(64)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return removed, err
			}
			name := entry.Name()
			id := strings.TrimSuffix(name, ".pending")
			if !idPattern.MatchString(id) {
				continue
			}
			original, err := m.files.root.Lstat(name)
			if err != nil || !singleRegular(original) {
				incident = true
				continue
			}
			h, _, err := m.files.read(name)
			if h.Ref.ID == "" {
				incident = true
				continue
			}
			if !strings.EqualFold(h.Origin.WorkspaceID, workspace) || now.Before(h.Ref.ExpiresAt) {
				continue
			}
			// Valid file metadata can bound a damaged body, but the run still
			// reports the incident and never treats the body as evidence.
			incident = incident || err != nil
			ok, err := canRemove(id)
			if err != nil {
				return removed, err
			}
			if !ok {
				continue
			}
			current, err := m.files.root.Lstat(name)
			if err != nil || !singleRegular(current) || !os.SameFile(original, current) ||
				original.Size() != current.Size() || !original.ModTime().Equal(current.ModTime()) {
				incident = true
				continue
			}
			if err = m.files.root.Remove(name); err != nil {
				return removed, err
			}
			removed++
			if removed == limit {
				break
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return removed, readErr
		}
	}
	if incident {
		return removed, preparecase.ErrContent
	}
	return removed, nil
}

func singleRegular(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
