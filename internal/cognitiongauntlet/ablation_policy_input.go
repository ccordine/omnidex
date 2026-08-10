package cognitiongauntlet

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
)

var errAblationContextBudget = errors.New("cognition ablation context budget exhausted")

func prepareAblationPolicyInput(
	state *ablationState,
	budget RunBudget,
	context ablationContext,
	revision cognition.WorldRevision,
	cycle uint32,
	modelCalls int,
) (contextbuilder.Projection, cognition.RuntimeSnapshot, error) {
	maxEvidence := len(context.Materials) - 1
	for {
		projection, err := buildAblationProjection(
			state.episode, state.actor, state.variant, cycle,
			budget.ContextBytes, context.Materials, maxEvidence,
		)
		if err != nil {
			if errors.Is(err, contextbuilder.ErrBudgetExceeded) {
				return contextbuilder.Projection{}, cognition.RuntimeSnapshot{},
					fmt.Errorf("%w: %v", errAblationContextBudget, err)
			}
			return contextbuilder.Projection{}, cognition.RuntimeSnapshot{}, err
		}
		evidence := selectedAblationEvidence(projection, context.Evidence)
		if len(evidence) > budget.Decision.MaxEvidenceRefs {
			return contextbuilder.Projection{}, cognition.RuntimeSnapshot{},
				fmt.Errorf("ablation projection exposed too many evidence references")
		}
		runtimeBudget, err := budget.RuntimeBudget()
		if err != nil {
			return contextbuilder.Projection{}, cognition.RuntimeSnapshot{}, err
		}
		runtimeBudget.RemainingPolicyCalls = uint32(budget.ModelCalls - modelCalls)
		catalog := state.catalog
		if state.variant == VariantRawShell {
			catalog, err = rawShellCatalog()
			if err != nil {
				return contextbuilder.Projection{}, cognition.RuntimeSnapshot{}, err
			}
		}
		snapshot, err := cognition.NewRuntimeSnapshot(
			state.goal, revision, state.obligation, catalog, state.actor,
			ablationProjectionRef(projection), runtimeBudget, evidence,
		)
		if err != nil {
			return contextbuilder.Projection{}, cognition.RuntimeSnapshot{}, err
		}
		envelope, err := cognitionpolicy.MeasureEnvelope(snapshot, projection)
		if err != nil {
			return contextbuilder.Projection{}, cognition.RuntimeSnapshot{}, err
		}
		if envelope.Bytes <= runtimeBudget.MaxInputBytes &&
			envelope.EstimatedTokens <= runtimeBudget.MaxInputTokens {
			return projection, snapshot, nil
		}
		if state.variant != VariantLedgerProjection || maxEvidence == 0 {
			return contextbuilder.Projection{}, cognition.RuntimeSnapshot{}, fmt.Errorf(
				"%w: exact envelope needs %d bytes/%d tokens; frozen limits are %d bytes/%d tokens",
				errAblationContextBudget, envelope.Bytes, envelope.EstimatedTokens,
				runtimeBudget.MaxInputBytes, runtimeBudget.MaxInputTokens,
			)
		}
		maxEvidence--
	}
}
