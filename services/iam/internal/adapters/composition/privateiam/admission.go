package privateiam

import (
	"context"
	"reflect"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
)

type workKey struct{}

// policyWork exists only inside a real startgate.Start callback. It binds the
// existing authorization service to that section without re-entering the gate.
type policyWork struct {
	caller   authorizeaction.Caller
	epoch    int64
	expected *authorizeaction.ScopeEpochs
	facts    authorizeaction.ScopeEpochs
}

func (w *policyWork) withContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, workKey{}, w)
}

type admissionPorts struct{}

func (admissionPorts) Current(ctx context.Context) (authorizeaction.Caller, error) {
	work, ok := ctx.Value(workKey{}).(*policyWork)
	if !ok || ctx.Err() != nil {
		return authorizeaction.Caller{}, authorizeaction.ErrUnavailable
	}
	return work.caller, nil
}

func (admissionPorts) Within(ctx context.Context, binding authorizeaction.GateBinding, callback func(context.Context, authorizeaction.Admission) error) error {
	work, ok := ctx.Value(workKey{}).(*policyWork)
	if !ok || work.caller != binding.Caller || ctx.Err() != nil {
		return authorizeaction.ErrUnavailable
	}
	return callback(ctx, admission{binding: binding, work: work})
}

type admission struct {
	binding authorizeaction.GateBinding
	work    *policyWork
}

func (a admission) Binding() authorizeaction.GateBinding { return a.binding }

func (a admission) Confirm(ctx context.Context, facts authorizeaction.ScopeEpochs) (int64, error) {
	// The existing gate keeps known denials sticky for this boot. Policy
	// consultation handles all current SQL denies conservatively as well.
	if ctx.Err() != nil || facts.WorkspaceEpoch != a.work.epoch || len(facts.Revocations) != 0 {
		return 0, authorizeaction.ErrUnavailable
	}
	if a.work.expected != nil && !reflect.DeepEqual(*a.work.expected, facts) {
		return 0, authorizeaction.ErrUnavailable
	}
	a.work.facts = facts
	return facts.WorkspaceEpoch, nil
}
