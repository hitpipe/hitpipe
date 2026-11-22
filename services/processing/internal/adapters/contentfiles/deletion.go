package contentfiles

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

// DeletionTarget is the exact durable reference from the tenant projection.
type DeletionTarget struct {
	Ref    preparecase.Ref    `json:"ref"`
	Origin preparecase.Origin `json:"origin"`
	Shared bool               `json:"shared"`
}

func deletionMarkName(t DeletionTarget) string {
	key := t.Origin.WorkspaceID + "/" + t.Origin.CaseID + "/" + t.Ref.ID
	return ".deleted-" + strings.TrimPrefix(hash([]byte(key)), "sha256:")
}

func sameTarget(a, b DeletionTarget) bool {
	return a.Origin.WorkspaceID == b.Origin.WorkspaceID && a.Origin.CaseID == b.Origin.CaseID &&
		a.Origin.Revision <= b.Origin.Revision && a.Ref.ID == b.Ref.ID && a.Ref.Class == b.Ref.Class &&
		a.Ref.Digest == b.Ref.Digest && a.Ref.Bytes == b.Ref.Bytes && a.Ref.ExpiresAt.Equal(b.Ref.ExpiresAt)
}

func validDeletionTarget(t DeletionTarget) bool {
	return validOrigin(t.Origin) && idPattern.MatchString(t.Ref.ID) &&
		(t.Ref.Class == "CASE_CONTEXT" || t.Ref.Class == "HISTORY") && !t.Ref.ExpiresAt.IsZero() &&
		t.Ref.Bytes > 0 && t.Ref.Bytes <= maxBody && validDigest(t.Ref.Digest)
}

func validDigest(s string) bool {
	prefix, hexPart, ok := strings.Cut(s, ":")
	if !ok || prefix != "sha256" || len(hexPart) != 64 || strings.ToLower(hexPart) != hexPart {
		return false
	}
	_, err := hex.DecodeString(hexPart)
	return err == nil
}

func (s *Store) deletionMark(t DeletionTarget) (DeletionTarget, error) {
	var prior DeletionTarget
	found, err := s.readControl(deletionMarkName(t), &prior)
	if err != nil || !found {
		return prior, err
	}
	if !validDeletionTarget(prior) || !sameTarget(prior, t) {
		return DeletionTarget{}, preparecase.ErrContent
	}
	return prior, nil
}

func (s *Store) deleted(ref preparecase.Ref, origin preparecase.Origin) (bool, error) {
	if ref.Class != "CASE_CONTEXT" && ref.Class != "HISTORY" {
		return false, nil
	}
	t := DeletionTarget{Ref: ref, Origin: origin}
	if !validDeletionTarget(t) {
		return false, preparecase.ErrContent
	}
	mark, err := s.deletionMark(t)
	return mark.Ref.ID != "", err
}

// PreflightDeletion resolves file identity under the exclusive volume lease.
// A missing file is acceptable only after a durable, exact deletion mark.
func (m *Maintenance) PreflightDeletion(t DeletionTarget, requireLive bool) error {
	return m.files.preflightDeletion(t, requireLive)
}

func (s *Store) preflightDeletion(t DeletionTarget, requireLive bool) error {
	if !validDeletionTarget(t) {
		return preparecase.ErrContent
	}
	if _, err := s.root.Lstat(t.Ref.ID + ".pending"); !errors.Is(err, os.ErrNotExist) {
		return preparecase.ErrContent
	}
	prior, err := s.deletionMark(t)
	if err != nil || (requireLive && prior.Ref.ID != "") {
		return preparecase.ErrContent
	}
	stat, err := s.root.Lstat(t.Ref.ID)
	if !requireLive && errors.Is(err, os.ErrNotExist) && prior.Ref.ID != "" && !t.Shared && !prior.Shared {
		return nil
	}
	if err != nil || !singleRegular(stat) {
		return preparecase.ErrContent
	}
	h, _, err := s.read(t.Ref.ID)
	if err != nil || !sameTarget(DeletionTarget{Ref: h.Ref, Origin: h.Origin}, t) {
		return preparecase.ErrContent
	}
	return nil
}

func (m *Maintenance) DeleteVersion(ctx context.Context, t DeletionTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.PreflightDeletion(t, false); err != nil {
		return err
	}
	prior, err := m.files.deletionMark(t)
	if err != nil {
		return err
	}
	// Journal the exact, validated ref before unlink. If a shared reference no
	// longer protects the file, persist that new disposition before unlink.
	if prior.Ref.ID == "" || prior.Shared != t.Shared {
		if err := m.files.writeControl(deletionMarkName(t), t); err != nil {
			return err
		}
	}
	if !t.Shared {
		if err := m.files.root.Remove(t.Ref.ID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := m.files.syncDir(); err != nil {
			return err
		}
	}
	return m.VerifyDeletion(t)
}

func (m *Maintenance) VerifyDeletion(t DeletionTarget) error {
	return m.files.VerifyDeletion(t)
}

func (s *Store) VerifyDeletion(t DeletionTarget) error {
	if err := s.preflightDeletion(t, false); err != nil {
		return err
	}
	mark, err := s.deletionMark(t)
	if err != nil || mark.Ref.ID == "" || mark.Shared != t.Shared {
		return preparecase.ErrContent
	}
	if !t.Shared {
		if _, err = s.root.Lstat(t.Ref.ID); !errors.Is(err, os.ErrNotExist) {
			return preparecase.ErrContent
		}
	}
	return nil
}

func applicationName(id string, generation uint64) string {
	key, _ := json.Marshal(struct {
		ID         string
		Generation uint64
	}{id, generation})
	return ".deletion-application-" + strings.TrimPrefix(hash(key), "sha256:")
}

func (m *Maintenance) ReadApplication(id string, generation uint64, value any) (bool, error) {
	return m.files.ReadApplication(id, generation, value)
}

func (s *Store) ReadApplication(id string, generation uint64, value any) (bool, error) {
	return s.readControl(applicationName(id, generation), value)
}

func (m *Maintenance) WriteApplication(id string, generation uint64, value any) error {
	return m.files.writeControl(applicationName(id, generation), value)
}

const controlLimit = 256 << 10

func (s *Store) readControl(name string, value any) (bool, error) {
	f, err := s.root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !singleRegular(info) || info.Size() > controlLimit {
		return false, preparecase.ErrContent
	}
	raw, err := io.ReadAll(io.LimitReader(f, controlLimit+1))
	if err != nil || len(raw) > controlLimit || json.Unmarshal(raw, value) != nil {
		return false, preparecase.ErrContent
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return false, preparecase.ErrContent
	}
	return true, nil
}

func (s *Store) writeControl(name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > controlLimit {
		return preparecase.ErrContent
	}
	id, err := UUID()
	if err != nil {
		return err
	}
	tmp := ".deletion-tmp-" + id
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(tmp)
	_, writeErr := f.Write(raw)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := s.root.Rename(tmp, name); err != nil {
		return err
	}
	return s.syncDir()
}
