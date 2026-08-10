package queue

import (
	"context"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func directCognitionCompletionDependents(
	graph cognition.ObligationGraphSnapshot,
	completed cognition.ObligationID,
) []cognition.ObligationID {
	result := make([]cognition.ObligationID, 0)
	for _, obligation := range graph.Obligations {
		if obligation.Status == cognition.ObligationSatisfied ||
			obligation.Status == cognition.ObligationFailed ||
			obligation.Status == cognition.ObligationSuperseded {
			continue
		}
		for _, dependency := range obligation.DependsOn {
			if dependency == completed {
				result = append(result, obligation.ID)
				break
			}
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func persistCognitionCompletionHandoffTx(
	ctx context.Context,
	tx pgx.Tx,
	authority cognitionProgressAuthority,
	header taskLedgerHeader,
	completed cognition.ObligationID,
	dependents []cognition.ObligationID,
	evidence []cognition.EvidenceRef,
) error {
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, authority.Actor.Generation, true)
	if err != nil {
		return err
	}
	mutations, err := cognitionstate.BuildCompletionEvidenceHandoff(
		set, completed, dependents, evidence,
	)
	if err != nil {
		return fmt.Errorf("build cognition completion evidence handoff: %w", err)
	}
	for index, mutation := range mutations {
		if _, err := applyWorkingSetCommandTx(
			ctx, tx, authority.Actor, mutation.Command(), mutation.Descriptor(),
		); err != nil {
			return fmt.Errorf("persist cognition completion handoff %d: %w", index, err)
		}
	}
	return requireCognitionHandoffWorkingSet(
		ctx, tx, header, authority, completed, dependents, evidence,
	)
}

func requireCognitionHandoffWorkingSet(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	authority cognitionProgressAuthority,
	completed cognition.ObligationID,
	dependents []cognition.ObligationID,
	evidence []cognition.EvidenceRef,
) error {
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, authority.Actor.Generation, false)
	if err != nil {
		return err
	}
	completedMembership, err := cognitionstate.AttentionMembership(
		cognition.AttentionScopeObligation, set.Scope, completed, "",
	)
	if err != nil {
		return err
	}
	for _, item := range set.Items {
		for _, membership := range item.Memberships {
			if membership.Scope == completedMembership.Scope {
				return fmt.Errorf("%w: completed cognition scope retained resident state", ErrCognitionConflict)
			}
		}
	}
	for _, ref := range evidence {
		if err := requireCognitionHandoffEvidence(set, dependents, ref); err != nil {
			return err
		}
	}
	return nil
}

func requireCognitionHandoffEvidence(
	set workingset.Snapshot,
	dependents []cognition.ObligationID,
	ref cognition.EvidenceRef,
) error {
	wantRef := cognitionEvidenceTaskRefs([]cognition.EvidenceRef{ref})[0]
	for _, item := range set.Items {
		if item.Ref != wantRef || item.Role != workingset.RoleEvidence || item.State != workingset.ItemResident {
			continue
		}
		for _, dependent := range dependents {
			membership, err := cognitionstate.AttentionMembership(
				cognition.AttentionScopeObligation, set.Scope, dependent, "",
			)
			if err != nil {
				return err
			}
			found := false
			for _, existing := range item.Memberships {
				if existing.Scope == membership.Scope {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: completion evidence lacks dependent membership", ErrCognitionConflict)
			}
		}
		return nil
	}
	return fmt.Errorf("%w: handed-off completion evidence is not resident", ErrCognitionConflict)
}
