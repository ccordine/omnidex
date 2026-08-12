package cognitiongauntlet

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

// verifyAblationSemanticContextDerivation reconstructs the code-owned state at
// every call boundary. Internal validity of a supplied projection is not
// enough: it must be the one the registered ablation renderer derived from the
// exact world, ledger, Working Set, action history, and frozen budget.
func verifyAblationSemanticContextDerivation(root ablationEvidenceRoot) error {
	if len(root.Transitions) == 0 {
		return fmt.Errorf("ablation context replay lacks its start transition")
	}
	state, err := newAblationStateWithAuthority(
		root.Variant, cognition.EpisodeRef{ID: root.EpisodeID}, root.Actor,
		root.Goal, root.Completion, root.WorldCatalog,
		root.PublicRunAuthority.Budget.WorkingSetBytes,
	)
	if err != nil {
		return fmt.Errorf("initialize ablation context replay: %w", err)
	}
	if !reflect.DeepEqual(state.obligation, root.Obligation) {
		return fmt.Errorf("ablation context replay changed the root obligation")
	}
	if err := state.recordTransition(root.Transitions[0]); err != nil {
		return fmt.Errorf("restore ablation start context: %w", err)
	}
	actions, _, err := indexAblationActionEvidence(root)
	if err != nil {
		return err
	}
	current := root.Transitions[0].Current
	for index, call := range root.Calls {
		cycle := uint32(index + 1)
		context, err := state.context(cycle, ContaminatedEvidencePacket{})
		if err != nil {
			return fmt.Errorf("rederive ablation call %d context: %w", cycle, err)
		}
		projection, snapshot, err := prepareAblationPolicyInput(
			state, root.PublicRunAuthority.Budget, context, current, cycle, index,
		)
		if err != nil {
			return fmt.Errorf("rederive ablation call %d input: %w", cycle, err)
		}
		if !reflect.DeepEqual(projection, call.Projection.Projection) ||
			!reflect.DeepEqual(newSemanticRuntimeSnapshot(snapshot), call.Snapshot) {
			return fmt.Errorf("ablation call %d differs from code-derived context", cycle)
		}
		action, exists := actions[cycle]
		if !exists {
			continue
		}
		if action.Trace.Failure != nil {
			state.appendAction(
				action.Trace.Action.Request,
				action.Trace.Failure.PublicMessage,
				true,
			)
			continue
		}
		if action.Trace.Transition == nil {
			return fmt.Errorf("ablation call %d action lacks an exact outcome", cycle)
		}
		state.appendAction(
			action.Trace.Action.Request,
			action.Trace.Transition.PublicOutcome,
			false,
		)
		if err := state.recordTransition(*action.Trace.Transition); err != nil {
			return fmt.Errorf("advance ablation call %d context: %w", cycle, err)
		}
		current = action.Trace.Transition.Current
	}
	if current != root.Terminal.Revision {
		return fmt.Errorf("ablation context replay stopped at another revision")
	}
	return verifyAblationSemanticContextBudget(root, state, current)
}

func verifyAblationSemanticContextBudget(
	root ablationEvidenceRoot,
	state *ablationState,
	current cognition.WorldRevision,
) error {
	if root.ContextBudget == nil {
		return nil
	}
	cycle := uint32(len(root.Calls) + 1)
	context, err := state.context(cycle, ContaminatedEvidencePacket{})
	if err != nil {
		return fmt.Errorf("rederive failed ablation context: %w", err)
	}
	_, _, err = prepareAblationPolicyInput(
		state, root.PublicRunAuthority.Budget, context, current, cycle, len(root.Calls),
	)
	var exact *ablationContextBudgetFailure
	if !errors.As(err, &exact) || exact == nil {
		return fmt.Errorf("ablation context budget evidence does not fail at the code boundary: %v", err)
	}
	want := root.ContextBudget
	if !reflect.DeepEqual(exact.projection, want.Projection.Projection) ||
		!reflect.DeepEqual(exact.snapshot, want.Snapshot) ||
		exact.modelInputBytes != want.ModelInputBytes {
		return fmt.Errorf("ablation context budget evidence differs from code-derived overflow")
	}
	return nil
}
