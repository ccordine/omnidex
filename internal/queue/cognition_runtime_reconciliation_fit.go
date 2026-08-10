package queue

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type cognitionReconciliationFitInput struct {
	Episode     CognitionEpisode
	Current     cognition.Obligation
	Authority   model.StepAttemptAuthority
	Prepared    cognitionruntime.PreparedSnapshot
	State       cognitionstate.ProjectionState
	Graph       cognition.ObligationGraphSnapshot
	Ledger      taskstate.MaterializedState
	Set         workingset.Snapshot
	Evidence    []cognitionstate.EvidenceMaterial
	FactSources cognitionFactProjectionSources
	Required    []cognition.AttentionRequest
	Requested   []cognition.AttentionRequest
}

func fitCognitionReconciliationPlan(
	input cognitionReconciliationFitInput,
) (cognitionstate.ReconciliationPlan, error) {
	rejected := make([]cognition.EvidenceRef, 0)
	for {
		plan, err := cognitionstate.BuildDefaultReconciliation(cognitionstate.ReconciliationInput{
			State: input.State, ObligationGraph: input.Graph, Ledger: input.Ledger,
			WorkingSet: input.Set, Evidence: input.Evidence, RequiredAttention: input.Required,
			Attention: input.Requested, CapacityRejected: rejected,
		})
		if err != nil {
			return cognitionstate.ReconciliationPlan{}, err
		}
		err = measureCognitionReconciliationPlan(input, plan)
		if err == nil {
			return plan, nil
		}
		if !errors.Is(err, ErrCognitionEnvelopeBudget) {
			return cognitionstate.ReconciliationPlan{}, err
		}
		ref, found := lastAcceptedDurableRetain(plan.AdvisoryOutcomes(), rejected)
		if !found {
			return cognitionstate.ReconciliationPlan{}, err
		}
		rejected = append(rejected, ref)
	}
}

func measureCognitionReconciliationPlan(
	input cognitionReconciliationFitInput,
	plan cognitionstate.ReconciliationPlan,
) error {
	set, err := workingset.Restore(input.Set)
	if err != nil {
		return err
	}
	for _, mutation := range plan.Commands() {
		if _, err := set.Apply(mutation.Command()); err != nil {
			return fmt.Errorf("preview cognition attention command: %w", err)
		}
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: plan.Descriptor().SourceSHA256, Spec: plan.ContextSpec(),
		WorkingSet: set, Materials: plan.Materials(),
	})
	if err != nil {
		return err
	}
	_, err = fitCognitionPolicyProjection(cognitionProjectionFitInput{
		Episode: input.Episode, Current: input.Current, Attempt: cognitionAttempt(input.Authority),
		Budget: input.Prepared.Snapshot.Budget(), CompletionEvidence: input.Prepared.CompletionEvidenceRefs,
		FactSources: input.FactSources,
		Set:         set, WorkID: plan.Descriptor().SourceSHA256, Spec: plan.ContextSpec(),
		Materials: plan.Materials(), Initial: projection,
	})
	return err
}

func lastAcceptedDurableRetain(
	outcomes []cognitionstate.AdvisoryOutcome,
	rejected []cognition.EvidenceRef,
) (cognition.EvidenceRef, bool) {
	already := make(map[cognition.EvidenceRef]struct{}, len(rejected))
	for _, ref := range rejected {
		already[ref] = struct{}{}
	}
	for index := len(outcomes) - 1; index >= 0; index-- {
		outcome := outcomes[index]
		if outcome.Disposition != cognitionstate.AdvisoryAccepted ||
			outcome.Request.Operation != cognition.AttentionRetain ||
			outcome.Request.Scope == cognition.AttentionScopeDecision {
			continue
		}
		if _, exists := already[outcome.Request.TargetRef]; !exists {
			return outcome.Request.TargetRef, true
		}
	}
	return cognition.EvidenceRef{}, false
}
