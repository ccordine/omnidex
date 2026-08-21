package worker

import (
	"context"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type boundContextCandidateSource struct {
	source      objectiveContextCandidateSource
	job         model.Job
	authority   turnAuthority
	preparation *roleplay.SimulationTurnAuthority
	projection  *roleplay.NarrativeSimulationProjection
}

func (source boundContextCandidateSource) Retrieve(
	ctx context.Context,
	terms []string,
) (contextcompiler.CandidateSet, error) {
	return source.source.ContextCandidates(
		ctx, source.job, source.authority, source.preparation, source.projection,
		append([]string{}, terms...),
	)
}

func compileObjectiveTurnContext(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	candidateProvider objectiveContextCandidateSource,
	stationProvider objectiveContextSieveStations,
	preparation *roleplay.SimulationTurnAuthority,
	projection *roleplay.NarrativeSimulationProjection,
) (turnAuthority, int, error) {
	result, err := contextcompiler.Compile(
		ctx,
		contextcompiler.Request{ExactInstruction: authority.Instruction},
		boundContextCandidateSource{
			source: candidateProvider, job: job, authority: authority,
			preparation: preparation, projection: projection,
		},
		contextcompiler.Stations{
			Terms: stationProvider, Relevance: stationProvider, Minification: stationProvider,
		},
	)
	if err != nil {
		return authority, 0, err
	}
	authority.Context = assemblyline.CloneObjectiveContext(result.Context)
	return authority, result.ModelCalls, nil
}
