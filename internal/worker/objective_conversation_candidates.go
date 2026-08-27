package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type runtimeConversationCandidateProvider struct {
	runtime *nativeRuntimeV3
}

func (provider runtimeConversationCandidateProvider) ContextSearchAvailability(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	preparation *roleplay.SimulationTurnAuthority,
	projection *roleplay.NarrativeSimulationProjection,
) (contextcompiler.SearchAvailability, error) {
	return (boundObjectiveContextProvider{
		runtime: provider.runtime, job: job, authority: authority,
		preparation: preparation, projection: projection,
	}).SearchAvailability(ctx)
}

func (provider runtimeConversationCandidateProvider) ContextCandidates(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	preparation *roleplay.SimulationTurnAuthority,
	projection *roleplay.NarrativeSimulationProjection,
	terms []string,
) (contextcompiler.CandidateSet, error) {
	return (boundObjectiveContextProvider{
		runtime: provider.runtime, job: job, authority: authority,
		preparation: preparation, projection: projection,
	}).Retrieve(ctx, terms)
}
