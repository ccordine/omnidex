package worker

import (
	"context"
	"fmt"

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

func (source boundContextCandidateSource) SearchAvailability(
	ctx context.Context,
) (contextcompiler.SearchAvailability, error) {
	return source.source.ContextSearchAvailability(
		ctx, source.job, source.authority, source.preparation, source.projection,
	)
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
	retrieval *contextcompiler.RetrievalDirective,
) (turnAuthority, int, error) {
	contextInstruction, err := objectiveContextInstruction(authority)
	if err != nil {
		return authority, 0, err
	}
	result, err := contextcompiler.Compile(
		ctx,
		contextcompiler.Request{
			ExactInstruction: contextInstruction,
			Retrieval:        retrieval,
		},
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

// resolveRoleplayTurnRetrieval resolves instruction-level retrieval concepts
// once for an ordered response round. Candidate acquisition and relevance stay
// responder-specific; only the identical semantic query is shared.
func resolveRoleplayTurnRetrieval(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	candidateProvider objectiveContextCandidateSource,
	stationProvider objectiveContextSieveStations,
	preparation roleplay.SimulationTurnAuthority,
) (contextcompiler.RetrievalDirective, int, error) {
	contextInstruction, err := objectiveContextInstruction(authority)
	if err != nil {
		return contextcompiler.RetrievalDirective{}, 0, err
	}
	availability := contextcompiler.SearchUnavailable
	for index, responder := range preparation.Responders {
		responderAuthority, projection, err := roleplayResponderTurnAuthority(
			authority, responder, nil,
		)
		if err != nil {
			return contextcompiler.RetrievalDirective{}, 0, fmt.Errorf(
				"project roleplay responder %d search authority: %w", index, err,
			)
		}
		current, err := (boundContextCandidateSource{
			source: candidateProvider, job: job, authority: responderAuthority,
			preparation: &preparation, projection: &projection,
		}).SearchAvailability(ctx)
		if err != nil {
			return contextcompiler.RetrievalDirective{}, 0, fmt.Errorf(
				"inspect roleplay responder %d search availability: %w", index, err,
			)
		}
		if err := current.Validate(); err != nil {
			return contextcompiler.RetrievalDirective{}, 0, fmt.Errorf(
				"roleplay responder %d: %w", index, err,
			)
		}
		if current == contextcompiler.SearchAvailable {
			availability = current
			break
		}
	}
	directive, calls, err := contextcompiler.ResolveRetrievalDirective(
		ctx, contextInstruction, availability, stationProvider,
	)
	if err != nil {
		return contextcompiler.RetrievalDirective{}, 0, err
	}
	return directive, calls, nil
}

// objectiveContextInstruction separates raw persisted job authority from the
// one code-owned semantic instruction that context-sieve stations may see.
// Roleplay command syntax remains available to deterministic repository and
// transition machinery through authority.Instruction, never to a model.
func objectiveContextInstruction(authority turnAuthority) (string, error) {
	switch authority.ChannelMode {
	case model.ChannelModeAssistant:
		return authority.Instruction, nil
	case model.ChannelModeRoleplay:
		if authority.RoleplayInputKind == roleplay.SimulationTurnExternalCommand {
			command, matched, err := roleplay.ParseResearchCommand(authority.Instruction)
			if err != nil {
				return "", fmt.Errorf("roleplay context research command: %w", err)
			}
			if !matched {
				return "", fmt.Errorf(
					"roleplay external-command context lacks exact research authority",
				)
			}
			return command.Question, nil
		}
		instruction, err := roleplayModelVisibleInstruction(
			authority.RoleplayInputKind, authority.Instruction,
		)
		if err != nil {
			return "", fmt.Errorf("roleplay context instruction: %w", err)
		}
		return instruction, nil
	default:
		return "", fmt.Errorf(
			"context compilation has unsupported channel mode %q", authority.ChannelMode,
		)
	}
}
