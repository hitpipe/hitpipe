package profileupdate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
)

type staticFiles map[string]getprofile.Profile

func (f staticFiles) Read(_ context.Context, id, _ string) (getprofile.Profile, error) {
	profile, found := f[id]
	if !found {
		return getprofile.Profile{}, ErrUnavailable
	}
	return profile, nil
}

func (staticFiles) WriteProfile(context.Context, getprofile.Profile) (Pointer, error) {
	panic("a recovered intent must reuse its original profile file")
}

type loadedReceipt struct{ value Receipt }

func (r loadedReceipt) Load(context.Context) (Receipt, bool, error) { return r.value, true, nil }
func (loadedReceipt) Prepare(context.Context, Receipt) error {
	panic("a recovered intent must reuse its original receipt")
}
func (loadedReceipt) Confirm(context.Context, Receipt) error {
	panic("confirmation runs only after SQL resolves")
}

type trackedPointer struct {
	current  Pointer
	switches int
}

func (p *trackedPointer) Pointer() Pointer { return p.current }

func (p *trackedPointer) Switch(_ context.Context, old, next Pointer) error {
	if p.current != old {
		return ErrStale
	}
	p.switches++
	p.current = next
	return nil
}

func TestReconcileOnlyExactDurableIntent(t *testing.T) {
	before := Pointer{"472fcd63-ab4e-41ce-bac5-123456789abc", "sha256:" + strings.Repeat("1", 64)}
	after := Pointer{"472fcd63-ab4e-41ce-bac5-123456789abd", "sha256:" + strings.Repeat("2", 64)}
	other := Pointer{"472fcd63-ab4e-41ce-bac5-123456789abe", "sha256:" + strings.Repeat("3", 64)}
	who := Identity{PrincipalID: "60a92117-41d4-4b02-95e7-187d85e07817"}
	cmd := Command{RequestID: "6ba7b810-9dad-41d1-80b4-00c04fd430c8", DisplayName: "New", Timezone: "UTC", ExpectedRevision: 4}
	bodyDigest, err := digest(cmd)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{content: staticFiles{
		before.Ref: {UserID: who.PrincipalID, DisplayName: "Old", Timezone: "UTC", Revision: 4},
		after.Ref:  {UserID: who.PrincipalID, DisplayName: "New", Timezone: "UTC", Revision: 5},
	}}
	for _, tc := range []struct {
		name                 string
		current              Pointer
		confirmed, different bool
		want                 error
		switches             int
	}{
		{name: "already committed", current: after},
		{name: "proved rollback", current: before, switches: 1},
		{name: "pending ambiguous history", current: other, want: ErrUncertain},
		{name: "confirmed newer history", current: other, confirmed: true, want: ErrStale},
		{name: "different body", current: before, different: true, want: ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestDigest := bodyDigest
			if tc.different {
				requestDigest = other.Digest
			}
			receipt := Receipt{BodyDigest: bodyDigest, Before: before, After: after, Revision: 5, Confirmed: tc.confirmed}
			tx := &trackedPointer{current: tc.current}
			result, err := service.apply(t.Context(), tx, loadedReceipt{receipt}, who, cmd, requestDigest)
			if !errors.Is(err, tc.want) || tx.switches != tc.switches {
				t.Fatalf("reconciliation returned %v after %d writes", err, tx.switches)
			}
			if tc.want == nil && (result != receipt || tx.current != after) {
				t.Fatal("reconciliation changed the prepared intent")
			}
		})
	}
}
