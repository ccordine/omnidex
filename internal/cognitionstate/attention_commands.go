package cognitionstate

import (
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type attentionCommandBuilder struct {
	set        *workingset.Set
	sourceSHA  string
	commands   []WorkingSetMutation
	operation  int
	ledgerID   taskstate.LedgerID
	obligation string
}

func newAttentionCommandBuilder(input ReconciliationInput, sourceSHA string) (*attentionCommandBuilder, error) {
	set, err := workingset.Restore(input.WorkingSet)
	if err != nil {
		return nil, fmt.Errorf("%w: restore working set: %v", ErrInvalidReconciliation, err)
	}
	return &attentionCommandBuilder{
		set: set, sourceSHA: sourceSHA, ledgerID: input.Ledger.ID,
		obligation: string(input.State.Obligation().ID),
	}, nil
}

func (builder *attentionCommandBuilder) releaseUndesired(candidates []attentionCandidate) error {
	desired := make(map[string]attentionCandidate, len(candidates))
	for _, candidate := range candidates {
		desired[taskstate.RefIdentity(candidate.ref)] = candidate
	}
	for _, item := range builder.set.ResidentItems() {
		candidate, wanted := desired[taskstate.RefIdentity(item.Ref)]
		if wanted {
			if item.Ref.Hash != candidate.ref.Hash {
				return fmt.Errorf("%w: stable reference %q changed hash without a new version", ErrInvalidReconciliation, item.Ref.URI)
			}
			if item.Role != candidate.role {
				return fmt.Errorf("%w: resident reference %q has role %q, want %q", ErrInvalidReconciliation, item.Ref.URI, item.Role, candidate.role)
			}
			desiredMemberships, err := candidateMemberships(candidate, builder.set.Scope())
			if err != nil {
				return err
			}
			for _, membership := range item.Memberships {
				if membershipIncluded(desiredMemberships, membership) {
					continue
				}
				if err := builder.appendRelease(item.ID, membership.Scope); err != nil {
					return err
				}
			}
			continue
		}
		if !managedAttentionRef(item.Ref, builder.ledgerID) {
			continue
		}
		for _, membership := range item.Memberships {
			if err := builder.appendRelease(item.ID, membership.Scope); err != nil {
				return err
			}
		}
	}
	return nil
}

func (builder *attentionCommandBuilder) ensureCandidates(
	candidates []attentionCandidate,
	outcomes []AdvisoryOutcome,
) ([]attentionCandidate, []AdvisoryOutcome, error) {
	accepted := make([]attentionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		desiredMemberships, err := candidateMemberships(candidate, builder.set.Scope())
		if err != nil {
			return nil, nil, err
		}
		item, exists := itemByRef(builder.set, candidate.ref)
		if exists {
			if item.Ref.Hash != candidate.ref.Hash {
				return nil, nil, fmt.Errorf(
					"%w: stable reference %q changed hash without a new version",
					ErrInvalidReconciliation, item.Ref.URI,
				)
			}
			if item.Role != candidate.role {
				return nil, nil, fmt.Errorf(
					"%w: historical reference %q has role %q, want %q",
					ErrInvalidReconciliation, item.Ref.URI, item.Role, candidate.role,
				)
			}
			switch item.State {
			case workingset.ItemReleased:
				if candidate.advisory && !builder.fits(candidate) {
					outcomes = rejectAdvisoryCapacity(outcomes, candidate.ref)
					continue
				}
				if err := builder.appendReacquire(item, candidate); err != nil {
					if candidate.advisory && isAttentionCapacityError(err) {
						outcomes = rejectAdvisoryCapacity(outcomes, candidate.ref)
						continue
					}
					return nil, nil, attentionMutationError(candidate.mandatory, err)
				}
				for _, membership := range desiredMemberships[1:] {
					if err := builder.appendMembership(item.ID, membership); err != nil {
						return nil, nil, attentionMutationError(candidate.mandatory, err)
					}
				}
				accepted = append(accepted, candidate)
				continue
			case workingset.ItemInvalidated:
				if !candidate.mandatory {
					outcomes = rejectAdvisoryUnavailable(outcomes, candidate.ref)
					continue
				}
				return nil, nil, fmt.Errorf(
					"%w: required exact reference %q was invalidated",
					ErrInvalidReconciliation, item.Ref.URI,
				)
			case workingset.ItemResident:
			default:
				return nil, nil, fmt.Errorf(
					"%w: exact reference %q has unregistered state %q",
					ErrInvalidReconciliation, item.Ref.URI, item.State,
				)
			}
			for _, membership := range desiredMemberships {
				if itemHasExactMembership(item, membership) {
					continue
				}
				if err := builder.appendMembership(item.ID, membership); err != nil {
					return nil, nil, attentionMutationError(candidate.mandatory, err)
				}
				item, _ = itemByRef(builder.set, candidate.ref)
			}
			accepted = append(accepted, candidate)
			continue
		}
		if candidate.advisory && !builder.fits(candidate) {
			outcomes = rejectAdvisoryCapacity(outcomes, candidate.ref)
			continue
		}
		if err := builder.appendAcquire(candidate); err != nil {
			if candidate.advisory && isAttentionCapacityError(err) {
				outcomes = rejectAdvisoryCapacity(outcomes, candidate.ref)
				continue
			}
			return nil, nil, attentionMutationError(candidate.mandatory, err)
		}
		item, _ = itemByRef(builder.set, candidate.ref)
		for _, membership := range desiredMemberships[1:] {
			if err := builder.appendMembership(item.ID, membership); err != nil {
				return nil, nil, attentionMutationError(candidate.mandatory, err)
			}
		}
		accepted = append(accepted, candidate)
	}
	return accepted, outcomes, nil
}

func (builder *attentionCommandBuilder) appendAcquire(candidate attentionCandidate) error {
	memberships, err := candidateMemberships(candidate, builder.set.Scope())
	if err != nil {
		return err
	}
	membership := memberships[0]
	itemDigest, err := mappingDigest(struct {
		Ref  taskstate.Ref
		Role workingset.Role
	}{candidate.ref, candidate.role})
	if err != nil {
		return err
	}
	request := workingset.AcquireRequest{
		ID: workingset.ItemID("cognition_item_" + itemDigest), Ref: candidate.ref,
		Role: candidate.role, Retention: membership.Retention, Scope: membership.Scope,
		Priority: candidate.priority, ByteCost: len(candidate.content),
		Acquisition: workingset.Acquisition{
			Provider: providerForCandidate(candidate), OperationID: "cognition-attention-" + itemDigest,
			Reason: "Code-owned cognition attention reconciliation.",
		},
	}
	id, err := builder.nextCommandID(workingset.CommandAcquire, string(request.ID))
	if err != nil {
		return err
	}
	command := &workingset.AcquireCommand{
		CommandID: id, ExpectedVersion: builder.set.Version(), Actor: taskstate.AuthorityCode, Request: request,
	}
	return builder.appendCommand(workingset.CommandAcquire, command)
}

func itemHasExactMembership(item workingset.Item, expected workingset.Membership) bool {
	for _, membership := range item.Memberships {
		if membership == expected {
			return true
		}
	}
	return false
}

func (builder *attentionCommandBuilder) appendRetain(id workingset.ItemID, scope workingset.Scope) error {
	commandID, err := builder.nextCommandID(workingset.CommandRetain, string(id)+":"+string(scope.ID))
	if err != nil {
		return err
	}
	command := &workingset.RetainCommand{
		CommandID: commandID, ExpectedVersion: builder.set.Version(), Actor: taskstate.AuthorityCode,
		ItemID: id, Scope: scope, Retention: workingset.RetentionPinned,
	}
	return builder.appendCommand(workingset.CommandRetain, command)
}

func (builder *attentionCommandBuilder) appendRelease(id workingset.ItemID, scope workingset.Scope) error {
	commandID, err := builder.nextCommandID(workingset.CommandRelease, string(id)+":"+string(scope.ID))
	if err != nil {
		return err
	}
	command := &workingset.ReleaseCommand{
		CommandID: commandID, ExpectedVersion: builder.set.Version(), Actor: taskstate.AuthorityCode,
		ItemID: id, Scope: scope, Reason: "Code-owned lifecycle released inactive cognition state.",
	}
	return builder.appendCommand(workingset.CommandRelease, command)
}

func (builder *attentionCommandBuilder) nextCommandID(kind workingset.CommandKind, target string) (workingset.CommandID, error) {
	id, err := workingset.NewCommandID(
		AttentionPlanSchemaV1, builder.sourceSHA, strconv.Itoa(builder.operation), string(kind), target,
	)
	if err != nil {
		return "", fmt.Errorf("%w: command identity: %v", ErrInvalidReconciliation, err)
	}
	builder.operation++
	return id, nil
}

func (builder *attentionCommandBuilder) appendCommand(kind workingset.CommandKind, command workingset.Command) error {
	descriptor, err := workingset.DescribeCommand(command)
	if err != nil {
		return fmt.Errorf("%w: describe working-set command: %v", ErrInvalidReconciliation, err)
	}
	mutation := WorkingSetMutation{kind: kind, descriptor: descriptor}
	switch value := command.(type) {
	case *workingset.AcquireCommand:
		copy := *value
		mutation.acquire = &copy
	case *workingset.ReacquireCommand:
		copy := *value
		mutation.reacquire = &copy
	case *workingset.RetainCommand:
		copy := *value
		mutation.retain = &copy
	case *workingset.ReleaseCommand:
		copy := *value
		mutation.release = &copy
	case *workingset.CloseScopeCommand:
		copy := *value
		mutation.closeScope = &copy
	default:
		return fmt.Errorf("%w: command kind %q is not supported", ErrInvalidReconciliation, kind)
	}
	if _, err := builder.set.Apply(command); err != nil {
		return err
	}
	builder.commands = append(builder.commands, mutation)
	return nil
}

func (builder *attentionCommandBuilder) fits(candidate attentionCandidate) bool {
	usage, budget := builder.set.Usage(), builder.set.Budget()
	return usage.ResidentItems+1 <= budget.MaxItems && usage.ResidentBytes+len(candidate.content) <= budget.MaxBytes
}

func providerForCandidate(candidate attentionCandidate) workingset.Provider {
	if candidate.role == workingset.RoleEvidence {
		return workingset.ProviderEvidence
	}
	return workingset.ProviderTaskState
}

func rejectAdvisoryCapacity(outcomes []AdvisoryOutcome, ref taskstate.Ref) []AdvisoryOutcome {
	for index := range outcomes {
		if taskstate.RefIdentity(evidenceLedgerRef(outcomes[index].Request.TargetRef)) == taskstate.RefIdentity(ref) &&
			outcomes[index].Request.Operation == "retain" {
			outcomes[index].Disposition = AdvisoryRejectedCapacity
			outcomes[index].Reason = "The working-set capacity cannot admit this advisory evidence."
		}
	}
	return outcomes
}
