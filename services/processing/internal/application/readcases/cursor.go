package readcases

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var states = []string{"RECEIVED", "WAITING_DATA", "WORKING", "UNKNOWN", "ATTENTION", "CONFIRMED", "EXCLUDED", "CLOSED_WITHOUT_CONFIRMATION"}

func UUID(s string) bool { return uuid.MatchString(s) }
func State(s string) bool {
	for _, v := range states {
		if s == v {
			return true
		}
	}
	return false
}

type Page struct {
	Snapshot, After time.Time
	AfterID         string
}

// CursorKey must be deployment-provided and retained across restarts. There is
// no hardcoded/default key. Tokens authenticate workspace, operation, filter and
// case scope without trusting client-supplied JSON or a foreign cursor.
type CursorKey [32]byte

func NewCursorKey(b []byte) (CursorKey, error) {
	var k CursorKey
	if len(b) != 32 {
		return k, ErrUnavailable
	}
	copy(k[:], b)
	return k, nil
}
func (k CursorKey) usable() bool { var zero CursorKey; return k != zero }
func idBytes(s string) []byte    { b, _ := hex.DecodeString(strings.ReplaceAll(s, "-", "")); return b }
func idString(b []byte) string {
	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func (k CursorKey) Encode(r Request, p Page) (string, error) {
	if !k.usable() || !UUID(r.WorkspaceID) || p.Snapshot.IsZero() || p.After.IsZero() || !UUID(p.AfterID) {
		return "", ErrUnavailable
	}
	b := make([]byte, 67)
	b[0] = 1
	b[1] = byte(r.Kind)
	copy(b[2:18], idBytes(r.WorkspaceID))
	if r.Kind != List {
		if !UUID(r.CaseID) {
			return "", ErrInvalid
		}
		copy(b[18:34], idBytes(r.CaseID))
	}
	for i, v := range states {
		if v == r.State {
			b[34] = byte(i + 1)
		}
	}
	binary.BigEndian.PutUint64(b[35:43], uint64(p.Snapshot.UnixMicro()))
	binary.BigEndian.PutUint64(b[43:51], uint64(p.After.UnixMicro()))
	copy(b[51:67], idBytes(p.AfterID))
	mac := hmac.New(sha256.New, k[:])
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(append(b, mac.Sum(nil)...)), nil
}
func (k CursorKey) Decode(r Request) (Page, error) {
	if !k.usable() {
		return Page{}, ErrUnavailable
	}
	b, e := base64.RawURLEncoding.DecodeString(r.Cursor)
	if e != nil || len(b) != 99 || b[0] != 1 {
		return Page{}, ErrInvalid
	}
	mac := hmac.New(sha256.New, k[:])
	mac.Write(b[:67])
	if !hmac.Equal(b[67:], mac.Sum(nil)) {
		return Page{}, ErrInvalid
	}
	expectedCase := make([]byte, 16)
	if r.Kind != List {
		expectedCase = idBytes(r.CaseID)
	}
	st := byte(0)
	for i, v := range states {
		if v == r.State {
			st = byte(i + 1)
		}
	}
	if b[1] != byte(r.Kind) || !hmac.Equal(b[2:18], idBytes(r.WorkspaceID)) || !hmac.Equal(b[18:34], expectedCase) || b[34] != st {
		return Page{}, ErrInvalid
	}
	p := Page{Snapshot: time.UnixMicro(int64(binary.BigEndian.Uint64(b[35:43]))).UTC(), After: time.UnixMicro(int64(binary.BigEndian.Uint64(b[43:51]))).UTC(), AfterID: idString(b[51:67])}
	if p.After.After(p.Snapshot) {
		return Page{}, ErrInvalid
	}
	return p, nil
}
