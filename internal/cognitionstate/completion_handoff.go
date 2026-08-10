package cognitionstate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const CompletionHandoffSchemaV1 = "omnidex.cognition-state-completion-handoff.v1"

// BuildCompletionEvidenceHandoff moves only exact code-owned completion proof
// from a satisfied obligation to its causal dependents, then closes the
// satisfied obligation's Working Set scope. Unrelated scoped state is released.
func BuildCompletionEvidenceHandoff(
	snapshot workingset.Snapshot,
	completed cognition.ObligationID,
	dependents []cognition.ObligationID,
	evidence []cognition.EvidenceRef,
) ([]WorkingSetMutation, error) {
	if completed == "" {
		return nil, fmt.Errorf("%w: completion handoff requires one completed obligation", ErrInvalidReconciliation)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		return nil, fmt.Errorf("%w: restore working set: %v", ErrInvalidReconciliation, err)
	}
	dependents = append([]cognition.ObligationID(nil), dependents...)
	sort.Slice(dependents, func(left, right int) bool { return dependents[left] < dependents[right] })
	for index, dependent := range dependents {
		if dependent == "" || dependent == completed || index > 0 && dependent == dependents[index-1] {
			return nil, fmt.Errorf("%w: completion handoff dependent %d is invalid", ErrInvalidReconciliation, index)
		}
	}
	wanted := make(map[string]cognition.EvidenceRef, len(evidence))
	for index, ref := range evidence {
		if err := ref.Validate(); err != nil {
			return nil, fmt.Errorf("%w: completion handoff evidence %d is invalid", ErrInvalidReconciliation, index)
		}
		key := taskstate.RefIdentity(evidenceLedgerRef(ref))
		if _, duplicate := wanted[key]; duplicate {
			return nil, fmt.Errorf("%w: completion handoff evidence %d is duplicated", ErrInvalidReconciliation, index)
		}
		wanted[key] = ref
	}
	sourceSHA, err := mappingDigest(struct {
		Schema     string
		WorkingSet workingset.Snapshot
		Completed  cognition.ObligationID
		Dependents []cognition.ObligationID
		Evidence   []cognition.EvidenceRef
	}{CompletionHandoffSchemaV1, snapshot, completed, dependents, evidence})
	if err != nil {
		return nil, err
	}
	builder := &attentionCommandBuilder{
		set: set, sourceSHA: sourceSHA, ledgerID: snapshot.Owner.LedgerID,
		obligation: string(completed),
	}
	completedMembership, err := AttentionMembership(
		cognition.AttentionScopeObligation, snapshot.Scope, completed, mappingZeroDigest,
	)
	if err != nil {
		return nil, err
	}
	found := make(map[string]struct{}, len(wanted))
	for _, item := range set.Items() {
		if item.State != workingset.ItemResident {
			continue
		}
		key := taskstate.RefIdentity(item.Ref)
		ref, handoff := wanted[key]
		if handoff {
			if item.Role != workingset.RoleEvidence || item.Ref.Hash != ref.SHA256 {
				return nil, fmt.Errorf("%w: completion evidence conflicts with Working Set authority", ErrInvalidReconciliation)
			}
			found[key] = struct{}{}
			for _, dependent := range dependents {
				membership, membershipErr := AttentionMembership(
					cognition.AttentionScopeObligation, snapshot.Scope, dependent, mappingZeroDigest,
				)
				if membershipErr != nil {
					return nil, membershipErr
				}
				if !itemHasScope(item, membership.Scope) {
					if err := builder.appendMembership(item.ID, membership); err != nil {
						return nil, attentionMutationError(true, err)
					}
					updated, exists := builder.set.Item(item.ID)
					if !exists {
						return nil, fmt.Errorf("%w: handed-off evidence disappeared", ErrInvalidReconciliation)
					}
					item = updated
				}
			}
		}
		if itemHasScope(item, completedMembership.Scope) {
			if err := builder.appendRelease(item.ID, completedMembership.Scope); err != nil {
				return nil, attentionMutationError(true, err)
			}
		}
		for _, membership := range item.Memberships {
			if membership.Scope.Kind != workingset.ScopeCall ||
				!strings.HasPrefix(string(membership.Scope.ID), attentionDecisionScopePrefix) {
				continue
			}
			updated, exists := builder.set.Item(item.ID)
			if !exists || updated.State != workingset.ItemResident || !itemHasScope(updated, membership.Scope) {
				continue
			}
			if err := builder.appendRelease(item.ID, membership.Scope); err != nil {
				return nil, attentionMutationError(true, err)
			}
		}
	}
	if len(found) != len(wanted) {
		return nil, fmt.Errorf("%w: completion evidence is absent from the resident Working Set", ErrInvalidReconciliation)
	}
	if err := builder.appendCloseScope(completedMembership.Scope); err != nil {
		return nil, attentionMutationError(true, err)
	}
	return builder.commands, nil
}

func itemHasScope(item workingset.Item, scope workingset.Scope) bool {
	for _, membership := range item.Memberships {
		if membership.Scope == scope {
			return true
		}
	}
	return false
}

func (builder *attentionCommandBuilder) appendCloseScope(scope workingset.Scope) error {
	id, err := builder.nextCommandID(workingset.CommandCloseScope, string(scope.ID))
	if err != nil {
		return err
	}
	return builder.appendCommand(workingset.CommandCloseScope, &workingset.CloseScopeCommand{
		CommandID: id, ExpectedVersion: builder.set.Version(), Actor: taskstate.AuthorityCode,
		Scope: scope, Reason: "Code-owned lifecycle closed a satisfied cognition obligation scope.",
	})
}
