package contentfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

func TestReadOwnedCausalScopeIntegrityExpiryAndTraversal(t *testing.T) {
	p := private(t)
	s := open(t, p)
	now := time.Now().UTC().Truncate(time.Microsecond)
	origin := fixtureOrigin()
	ref, e := s.Write(context.Background(), []byte(`"synthetic owner data"`), "CASE_CONTEXT", origin, now.Add(time.Minute), now)
	if e != nil {
		t.Fatal(e)
	}
	// Equivalent timestamp offsets are accepted; metadata time equality is instant
	// equality, not equality of a Go time.Location pointer returned by PostgreSQL.
	equivalent := ref
	equivalent.ExpiresAt = equivalent.ExpiresAt.In(time.FixedZone("synthetic offset", 3600))
	if b, e := s.ReadOwned(context.Background(), equivalent, origin, now); e != nil || string(b) != `"synthetic owner data"` {
		t.Fatal(string(b), e)
	}
	for _, mode := range []string{"workspace", "case", "revision", "hash", "class", "bytes", "expiry", "traversal"} {
		t.Run(mode, func(t *testing.T) {
			r, o := ref, origin
			switch mode {
			case "workspace":
				o.WorkspaceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			case "case":
				o.CaseID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			case "revision":
				o.Revision = 0
			case "hash":
				r.Digest = "sha256:wrong"
			case "class":
				r.Class = "HISTORY"
			case "bytes":
				r.Bytes++
			case "expiry":
				r.ExpiresAt = r.ExpiresAt.Add(time.Second)
			case "traversal":
				r.ID = "../" + r.ID
			}
			if b, e := s.ReadOwned(context.Background(), r, o, now); e == nil || len(b) != 0 {
				t.Fatal("foreign/corrupt body exposed", e)
			}
		})
	}
	if _, e := s.ReadOwned(context.Background(), ref, origin, ref.ExpiresAt); !errors.Is(e, readcases.ErrExpired) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(p, ref.ID)); !os.IsNotExist(e) {
		t.Fatal("expiry-on-read did not remove authenticated expired body", e)
	}
}
