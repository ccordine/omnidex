package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/repository/cognitionenv"
)

func startRepositoryCognitionEpisode(
	session *directCodingSession,
	store *cognitionstore.Store,
	environment *cognitionenv.Environment,
	investigation cognitionenv.Investigation,
	episode cognition.EpisodeRef,
	brain cognitionpolicy.AttestedBrain,
	budget cognition.RuntimeBudget,
	start cognition.Transition,
) error {
	if session == nil || session.runtime == nil || session.runtime.svc == nil ||
		session.runtime.svc.repo == nil || session.runtime.claim == nil || store == nil ||
		environment == nil {
		return fmt.Errorf("repository cognition episode start is uninitialized")
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(brain.Ref, budget); err != nil {
		return fmt.Errorf("validate repository cognition runtime budget: %w", err)
	}
	check, err := investigation.Completion().Resolve(investigation.Goal())
	if err != nil {
		return err
	}
	if len(start.Observations) != 1 ||
		start.Observations[0].Kind != cognitionenv.ObservationNeed {
		return fmt.Errorf("repository cognition start omitted its exact accepted need evidence")
	}
	initialEvidence := []cognition.EvidenceRef{start.Observations[0].EvidenceRef()}
	planGeneration := cognition.InitialObligationGeneration
	rootID, err := cognition.DeriveObligationID(
		episode.ID, planGeneration, "", investigation.Goal(), check,
	)
	if err != nil {
		return err
	}
	_, err = store.StartEpisode(
		session.runtime.ctx,
		queue.CognitionEpisodeStart{
			Authority: session.runtime.claim.Authority, EpisodeID: episode.ID,
			AttestedBrain: brain, Scenario: investigation.Ref(), Goal: investigation.Goal(),
			Completion: investigation.Completion(), ActionCatalog: investigation.Catalog(), Budget: budget,
			Root: cognition.ObligationSpec{
				ID: rootID, Desired: investigation.Goal(),
				DependsOn: []cognition.ObligationID{}, SupportingRefs: initialEvidence,
				CompletionCheck: check,
			},
			Transition: start,
		},
	)
	if err != nil {
		return fmt.Errorf("start repository cognition episode: %w", err)
	}
	return nil
}

var _ cognitionruntime.CompletionEvaluator = (*cognitionenv.Environment)(nil)
