// Package profilefiles reads IAM-owned profile content on the accepted Linux
// runtime. Its filename and digest encoding are private adapter choices.
package profilefiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
)

var (
	ErrConfiguration = errors.New("profile_content_configuration_invalid")
	ErrUnavailable   = errors.New("profile_content_unavailable")
)

// MaxBytes bounds one local read; it is not a product retention or TTL rule.
const MaxBytes = 32 * 1024

type Reader struct {
	mu  sync.RWMutex
	dir *os.File
}

// New pins an owner-only directory descriptor. Every path component is opened
// without following symlinks, including the directory itself.
func New(root string) (*Reader, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return nil, ErrConfiguration
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrConfiguration
	}
	for _, component := range strings.Split(strings.TrimPrefix(root, "/"), "/") {
		next, openErr := syscall.Openat(fd, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if openErr != nil {
			return nil, ErrConfiguration
		}
		fd = next
	}
	dir := os.NewFile(uintptr(fd), "iam-profile-content")
	if dir == nil {
		syscall.Close(fd)
		return nil, ErrConfiguration
	}
	if !private(dir, true) {
		dir.Close()
		return nil, ErrConfiguration
	}
	return &Reader{dir: dir}, nil
}

func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dir == nil {
		return nil
	}
	err := r.dir.Close()
	r.dir = nil
	return err
}

func private(file *os.File, directory bool) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	if directory {
		return info.IsDir() && info.Mode().Perm() == 0700
	}
	return info.Mode().IsRegular() && info.Mode().Perm() == 0600 && stat.Nlink == 1 && info.Size() > 0 && info.Size() <= MaxBytes
}

func canonicalUUID(value string) bool {
	if len(value) != 36 || value == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for i, c := range []byte(value) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (r *Reader) Read(ctx context.Context, ref, digest string) (getprofile.Profile, error) {
	var zero getprofile.Profile
	if ctx == nil {
		return zero, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if r == nil || !canonicalUUID(ref) || len(digest) != len("sha256:")+64 || !strings.HasPrefix(digest, "sha256:") || strings.ToLower(digest) != digest {
		return zero, ErrUnavailable
	}
	expected, err := hex.DecodeString(digest[len("sha256:"):])
	if err != nil {
		return zero, ErrUnavailable
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.dir == nil || !private(r.dir, true) {
		return zero, ErrUnavailable
	}
	fd, err := syscall.Openat(int(r.dir.Fd()), ref+".json", syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return zero, ErrUnavailable
	}
	file := os.NewFile(uintptr(fd), "iam-profile-file")
	if file == nil {
		syscall.Close(fd)
		return zero, ErrUnavailable
	}
	defer file.Close()
	if !private(file, false) {
		return zero, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil || len(data) > MaxBytes || !private(file, false) {
		return zero, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	actual := sha256.Sum256(data)
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return zero, ErrUnavailable
	}
	profile, err := decode(data)
	if err != nil {
		return zero, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return profile, nil
}

// decode rejects duplicate keys as well as unknown/missing/null fields. The
// accepted timezone is a bounded string; it does not require an IANA identifier.
func decode(data []byte) (getprofile.Profile, error) {
	var profile getprofile.Profile
	if !utf8.Valid(data) || !validUnicodeEscapes(data) {
		return profile, ErrUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return profile, ErrUnavailable
	}
	seen := make(map[string]bool, 4)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return getprofile.Profile{}, ErrUnavailable
		}
		seen[key] = true
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return getprofile.Profile{}, ErrUnavailable
		}
		switch key {
		case "user_id":
			err = json.Unmarshal(raw, &profile.UserID)
		case "display_name":
			err = json.Unmarshal(raw, &profile.DisplayName)
		case "timezone":
			err = json.Unmarshal(raw, &profile.Timezone)
		case "revision":
			err = json.Unmarshal(raw, &profile.Revision)
		default:
			return getprofile.Profile{}, ErrUnavailable
		}
		if err != nil {
			return getprofile.Profile{}, ErrUnavailable
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return getprofile.Profile{}, ErrUnavailable
	}
	if _, err = decoder.Token(); err != io.EOF {
		return getprofile.Profile{}, ErrUnavailable
	}
	nameLength, timezoneLength := utf8.RuneCountInString(profile.DisplayName), utf8.RuneCountInString(profile.Timezone)
	if len(seen) != 4 || !canonicalUUID(strings.ToLower(profile.UserID)) || nameLength < 1 || nameLength > 120 || timezoneLength < 1 || timezoneLength > 80 || profile.Revision < 1 || profile.Revision > 9007199254740991 {
		return getprofile.Profile{}, ErrUnavailable
	}
	return profile, nil
}

// validUnicodeEscapes prevents encoding/json from silently replacing lone UTF-16
// surrogates with U+FFFD. Normal JSON syntax is still owned by the decoder. An
// escaped backslash is skipped as a unit, so a literal \uD800 is not an escape.
func validUnicodeEscapes(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		unit, ok := unicodeUnit(data[i+1 : i+5])
		if !ok {
			return false
		}
		i += 4
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low, ok := unicodeUnit(data[i+3 : i+7])
			if !ok || low < 0xDC00 || low > 0xDFFF {
				return false
			}
			i += 6
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return false
		}
	}
	return !inString
}
func unicodeUnit(data []byte) (uint16, bool) {
	var unit uint16
	for _, digit := range data {
		unit <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			unit |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			unit |= uint16(digit - 'a' + 10)
		case digit >= 'A' && digit <= 'F':
			unit |= uint16(digit - 'A' + 10)
		default:
			return 0, false
		}
	}
	return unit, true
}
