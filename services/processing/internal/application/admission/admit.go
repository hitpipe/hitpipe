package admission

import (
	"context"
	"errors"
)

var ErrNotAuthorized = errors.New("established admission context required")
var ErrNotAdmissible = errors.New("operation does not permit new admission")
var ErrConflict = errors.New("message ID digest conflict")

// EstablishedContext MUST originate from previously verified caller authority and
// installed source binding. Never construct it from signal fields or HTTP headers.
// This slice deliberately provides no runtime IAM/Intake resolver or default.
// ExpectedRevision is the accepted local operation revision, not a wire field.
type EstablishedContext struct {
	WorkspaceID, ConnectionID, AccountID, OperationID string
	Live, AuthorityVerified                           bool
	ExpectedRevision                                  int64
}

func (c EstablishedContext) Check(s Signal) error {
	if !c.AuthorityVerified || c.ExpectedRevision < 1 ||
		!uuidPattern.MatchString(c.OperationID) ||
		c.WorkspaceID != s.WorkspaceID || c.ConnectionID != s.ConnectionID || c.AccountID != s.AccountID || c.Live != s.Live {
		return ErrNotAuthorized
	}
	return nil
}

// Receipt is returned only after a successful durable commit. Revision is omitted
// because the accepted inbox schema does not retain an original result revision.
type Receipt struct{ MessageID, Disposition, RecordID string }
type Store interface {
	Accept(context.Context, Signal, EstablishedContext) (Receipt, error)
}

// Admit has no network or authority discovery side effects. The supplied context
// is a mandatory trusted composition input; unknown authority never becomes true.
func Admit(ctx context.Context, raw []byte, established EstablishedContext, store Store) (Receipt, error) {
	s, e := Decode(raw)
	if e != nil {
		return Receipt{}, e
	}
	if e = established.Check(s); e != nil {
		return Receipt{}, e
	}
	if store == nil {
		return Receipt{}, ErrNotAdmissible
	}
	return store.Accept(ctx, s, established)
}
