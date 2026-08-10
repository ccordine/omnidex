package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const ObservationRetentionSchemaV1 = "omnidex.cognition-state-observation-retention.v1"

// BuildObservationRetention makes an accepted environment observation available
// to the next decision for the exact obligation that caused it. It is a durable
// Working Set lifecycle mutation; prompt construction never infers recency.
func BuildObservationRetention(
	snapshot workingset.Snapshot,
	obligationID cognition.ObligationID,
	observation cognition.Observation,
) ([]WorkingSetMutation, error) {
	if err := observation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: observation: %v", ErrInvalidReconciliation, err)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		return nil, fmt.Errorf("%w: restore working set: %v", ErrInvalidReconciliation, err)
	}
	membership, err := AttentionMembership(
		cognition.AttentionScopeDecision, set.Scope(), obligationID, observation.Revision.SHA256,
	)
	if err != nil {
		return nil, err
	}
	candidate := evidenceCandidate(EvidenceMaterial{
		Ref: observation.EvidenceRef(), Content: observation.Content,
	}, false)
	candidate.memberships = []workingset.Membership{membership}
	sourceSHA, err := mappingDigest(struct {
		Schema       string
		WorkingSetID workingset.SetID
		Version      uint64
		ObligationID cognition.ObligationID
		Observation  cognition.Observation
	}{ObservationRetentionSchemaV1, snapshot.ID, snapshot.Version, obligationID, observation})
	if err != nil {
		return nil, err
	}
	builder := &attentionCommandBuilder{
		set: set, sourceSHA: sourceSHA, ledgerID: snapshot.Owner.LedgerID,
		obligation: string(obligationID),
	}
	item, exists := itemByRef(set, candidate.ref)
	if !exists {
		if err := builder.appendAcquire(candidate); err != nil {
			return nil, attentionMutationError(true, err)
		}
		return builder.commands, nil
	}
	if item.Ref.Hash != candidate.ref.Hash || item.Role != workingset.RoleEvidence {
		return nil, fmt.Errorf(
			"%w: observation reference conflicts with historical Working Set authority",
			ErrInvalidReconciliation,
		)
	}
	switch item.State {
	case workingset.ItemReleased:
		if err := builder.appendReacquire(item, candidate); err != nil {
			return nil, attentionMutationError(true, err)
		}
	case workingset.ItemResident:
		if itemHasExactMembership(item, membership) {
			return []WorkingSetMutation{}, nil
		}
		if err := builder.appendMembership(item.ID, membership); err != nil {
			return nil, attentionMutationError(true, err)
		}
	case workingset.ItemInvalidated:
		return nil, fmt.Errorf(
			"%w: accepted observation reference is historically invalidated",
			ErrInvalidReconciliation,
		)
	default:
		return nil, fmt.Errorf(
			"%w: observation reference has unregistered state %q",
			ErrInvalidReconciliation, item.State,
		)
	}
	return builder.commands, nil
}

func (builder *attentionCommandBuilder) appendMembership(
	itemID workingset.ItemID,
	membership workingset.Membership,
) error {
	id, err := builder.nextCommandID(
		workingset.CommandRetain, string(itemID)+":"+string(membership.Scope.ID),
	)
	if err != nil {
		return err
	}
	return builder.appendCommand(workingset.CommandRetain, &workingset.RetainCommand{
		CommandID: id, ExpectedVersion: builder.set.Version(), Actor: taskstate.AuthorityCode,
		ItemID: itemID, Scope: membership.Scope, Retention: membership.Retention,
	})
}
