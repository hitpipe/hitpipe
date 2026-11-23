package intake

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

type Directory struct {
	dir *os.File
}

// OpenDirectory opens a provisioned private content root. The directory is
// pinned for subsequent operations. No external request controls its path.
func OpenDirectory(path string) (*Directory, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	dir := os.NewFile(uintptr(fd), "delivery-content")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&07777 != 0700 || st.Uid != uint32(unix.Geteuid()) {
		_ = dir.Close()
		return nil, ErrUnavailable
	}
	return &Directory{dir: dir}, nil
}

func (d *Directory) Close() error {
	return d.dir.Close()
}

// Check verifies the provisioned, pinned directory can durably write. No
// record is created, and the probe never opens a path based on request data.
func (d *Directory) Check() error {
	dir := int(d.dir.Fd())
	var st unix.Stat_t
	if unix.Fstat(dir, &st) != nil || st.Nlink == 0 || st.Mode&07777 != 0700 ||
		st.Uid != uint32(unix.Geteuid()) {
		return ErrUnavailable
	}
	fd, err := unix.Openat(dir, ".", unix.O_TMPFILE|unix.O_WRONLY|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrUnavailable
	}
	f := os.NewFile(uintptr(fd), "delivery-storage-probe")
	_, err = f.Write([]byte{0})
	if err == nil {
		err = f.Sync()
	}
	if errors.Join(err, f.Close()) != nil {
		return ErrUnavailable
	}
	return nil
}

func (d *Directory) Put(data []byte, expires time.Time) (ContentRef, error) {
	if len(data) == 0 || len(data) > maxContentBytes {
		return ContentRef{}, ErrUnavailable
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return ContentRef{}, ErrUnavailable
	}
	dir := int(d.dir.Fd())
	final := id.String()
	temp := "." + final
	fd, err := unix.Openat(dir, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ContentRef{}, ErrUnavailable
	}
	defer func() { _ = unix.Unlinkat(dir, temp, 0) }()
	f := os.NewFile(uintptr(fd), "delivery-content-pending")
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil || unix.Renameat2(dir, temp, dir, final, unix.RENAME_NOREPLACE) != nil || d.dir.Sync() != nil {
		return ContentRef{}, ErrUnavailable
	}
	return ContentRef{ID: id, Size: len(data), SHA256: digest(data), Expires: expires}, nil
}

// Read accepts only a reference obtained from authorized SQL. A UUID filename
// by itself does not authorize file access.
func (d *Directory) Read(ref ContentRef, now time.Time) ([]byte, error) {
	if !now.Before(ref.Expires) || ref.ID == uuid.Nil || ref.Size <= 0 || ref.Size > maxContentBytes {
		return nil, ErrNotFound
	}
	fd, err := unix.Openat(int(d.dir.Fd()), ref.ID.String(), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	f := os.NewFile(uintptr(fd), "delivery-content-record")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG ||
		st.Mode&07777 != 0600 || st.Nlink != 1 || st.Uid != uint32(unix.Geteuid()) {
		return nil, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(ref.Size)+1))
	if err != nil || len(data) != ref.Size || digest(data) != ref.SHA256 {
		return nil, ErrUnavailable
	}
	return data, nil
}
