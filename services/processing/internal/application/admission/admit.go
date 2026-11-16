package admission

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var ErrNotAuthorized = errors.New("established admission context required")
var ErrNotAdmissible = errors.New("operation does not permit new admission")
var ErrConflict = errors.New("message ID digest conflict")
var ErrStaleRevision = errors.New("operation revision changed")
var ErrNotFound = errors.New("admission operation not found")

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
		!strings.EqualFold(c.WorkspaceID, s.WorkspaceID) || !strings.EqualFold(c.ConnectionID, s.ConnectionID) || c.AccountID != s.AccountID || c.Live != s.Live {
		return ErrNotAuthorized
	}
	return nil
}

// Receipt is returned only after a successful durable commit. Revision is omitted
// because the accepted inbox schema does not retain an original result revision.
type Receipt struct {
	MessageID   string `json:"message_id"`
	Disposition string `json:"disposition"`
	RecordID    string `json:"record_id,omitempty"`
}
type Decoder interface{ Decode([]byte) (Signal, error) }

type Store interface {
	Accept(context.Context, Signal, EstablishedContext) (Receipt, error)
}

// Admit has no network or authority discovery side effects. The supplied context
// is a mandatory trusted composition input; unknown authority never becomes true.
func Admit(ctx context.Context, raw []byte, established EstablishedContext, decoder Decoder, store Store) (Receipt, error) {
	return AdmitWithContext(ctx, raw, decoder, store, func(context.Context, Signal) (EstablishedContext, error) { return established, nil })
}

// AdmitWithContext is the same admission entry with trusted context discovery
// after one exact raw decode. resolve receives identity hints, never authority;
// its implementation must verify installed bindings and current owner grants.
// It does not serialize/rewrite the signal or calculate a second digest.
func AdmitWithContext(ctx context.Context, raw []byte, decoder Decoder, store Store, resolve func(context.Context, Signal) (EstablishedContext, error)) (Receipt, error) {
	if decoder == nil {
		return Receipt{}, ErrInvalidSignal
	}
	s, e := decoder.Decode(raw)
	if e != nil {
		return Receipt{}, e
	}
	if resolve == nil {
		return Receipt{}, ErrNotAuthorized
	}
	established, e := resolve(ctx, s)
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
