package cognitionstate

import (
	"errors"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func (builder *attentionCommandBuilder) appendReacquire(
	item workingset.Item,
	candidate attentionCandidate,
) error {
	memberships, err := candidateMemberships(candidate, builder.set.Scope())
	if err != nil {
		return err
	}
	membership := memberships[0]
	id, err := builder.nextCommandID(workingset.CommandReacquire, string(item.ID))
	if err != nil {
		return err
	}
	command := &workingset.ReacquireCommand{
		CommandID: id, ExpectedVersion: builder.set.Version(), Actor: taskstate.AuthorityCode,
		Request: workingset.ReacquireRequest{
			ItemID: item.ID, Ref: candidate.ref, Scope: membership.Scope, Retention: membership.Retention,
			ExpectedReacquisitionCount: item.ReacquisitionCount,
			Reason:                     "Code-owned cognition attention reacquired exact historical state.",
		},
	}
	return builder.appendCommand(workingset.CommandReacquire, command)
}

func candidateMemberships(candidate attentionCandidate, root workingset.Scope) ([]workingset.Membership, error) {
	memberships := append([]workingset.Membership(nil), candidate.memberships...)
	if candidate.pinned {
		pinned := workingset.Membership{Retention: workingset.RetentionPinned, Scope: root}
		if !membershipIncluded(memberships, pinned) {
			memberships = append(memberships, pinned)
		}
	}
	if len(memberships) == 0 {
		return nil, fmt.Errorf(
			"%w: candidate has no attention membership", ErrInvalidReconciliation,
		)
	}
	sort.Slice(memberships, func(left, right int) bool {
		if memberships[left].Scope.Kind != memberships[right].Scope.Kind {
			return memberships[left].Scope.Kind < memberships[right].Scope.Kind
		}
		if memberships[left].Scope.ID != memberships[right].Scope.ID {
			return memberships[left].Scope.ID < memberships[right].Scope.ID
		}
		return memberships[left].Retention < memberships[right].Retention
	})
	for index := 1; index < len(memberships); index++ {
		if memberships[index-1].Scope == memberships[index].Scope {
			return nil, fmt.Errorf("%w: candidate has two retentions for one scope", ErrInvalidReconciliation)
		}
	}
	return memberships, nil
}

func membershipIncluded(values []workingset.Membership, expected workingset.Membership) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func itemByRef(set *workingset.Set, ref taskstate.Ref) (workingset.Item, bool) {
	for _, item := range set.Items() {
		if taskstate.RefIdentity(item.Ref) == taskstate.RefIdentity(ref) {
			return item, true
		}
	}
	return workingset.Item{}, false
}

func attentionMutationError(mandatory bool, err error) error {
	if mandatory && isAttentionCapacityError(err) {
		return fmt.Errorf("%w: %v", ErrReconciliationCapacity, err)
	}
	return fmt.Errorf("%w: working-set attention mutation: %v", ErrInvalidReconciliation, err)
}

func isAttentionCapacityError(err error) bool {
	return errors.Is(err, workingset.ErrCapacityExceeded) ||
		errors.Is(err, workingset.ErrBudgetExceeded) ||
		errors.Is(err, workingset.ErrPinnedBudgetExceeded)
}

func rejectAdvisoryUnavailable(outcomes []AdvisoryOutcome, ref taskstate.Ref) []AdvisoryOutcome {
	for index := range outcomes {
		if taskstate.RefIdentity(evidenceLedgerRef(outcomes[index].Request.TargetRef)) == taskstate.RefIdentity(ref) &&
			outcomes[index].Request.Operation == "retain" {
			outcomes[index].Disposition = AdvisoryRejectedUnavailable
			outcomes[index].Reason = "The exact historical evidence was invalidated and cannot be reacquired."
		}
	}
	return outcomes
}
