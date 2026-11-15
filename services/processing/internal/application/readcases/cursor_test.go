package readcases

import (
	"bytes"
	"testing"
	"time"
)

func TestCursorBindsScopeOperationFilterAndPosition(t *testing.T) {
	key, e := NewCursorKey(bytes.Repeat([]byte{0x91}, 32))
	if e != nil {
		t.Fatal(e)
	}
	r := Request{Kind: History, WorkspaceID: "11111111-1111-4111-8111-111111111111", CaseID: "22222222-2222-4222-8222-222222222222"}
	p := Page{Snapshot: time.Date(2026, 11, 15, 0, 1, 0, 0, time.UTC), After: time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), AfterID: "33333333-3333-4333-8333-333333333333"}
	token, e := key.Encode(r, p)
	if e != nil || len(token) > 255 {
		t.Fatal(token, e)
	}
	r.Cursor = token
	got, e := key.Decode(r)
	if e != nil || got != p {
		t.Fatal(got, e)
	}
	for _, edit := range []func(*Request){func(r *Request) { r.WorkspaceID = "44444444-4444-4444-8444-444444444444" }, func(r *Request) { r.CaseID = "44444444-4444-4444-8444-444444444444" }, func(r *Request) { r.Kind = List }, func(r *Request) { r.State = "WORKING" }, func(r *Request) { r.Cursor = "!" + r.Cursor[1:] }} {
		q := r
		edit(&q)
		if _, e = key.Decode(q); e != ErrInvalid {
			t.Fatalf("accepted foreign/malformed cursor: %+v %v", q, e)
		}
	}
	other, _ := NewCursorKey(bytes.Repeat([]byte{0x92}, 32))
	if _, e = other.Decode(r); e != ErrInvalid {
		t.Fatal(e)
	}
	var zero CursorKey
	if _, e = zero.Decode(r); e != ErrUnavailable {
		t.Fatal("default key permitted", e)
	}
	if _, e = NewCursorKey([]byte("short")); e != ErrUnavailable {
		t.Fatal(e)
	}
}
