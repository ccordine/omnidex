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
	contextScope, err := objectiveContextScope(authority)
	if err != nil {
		return authority, 0, err
	}
	result, err := contextcompiler.Compile(
		ctx,
		contextcompiler.Request{
			ExactInstruction:   contextInstruction,
			ModelInstruction:   authority.ModelInstruction,
			Retrieval:          retrieval,
			Scope:              contextScope,
			KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
		},
		boundContextCandidateSource{
			source: candidateProvider, job: job, authority: authority,
			preparation: preparation, projection: projection,
		},
		contextcompiler.Stations{
			Relevance: stationProvider, Minification: stationProvider,
		},
	)
	if err != nil {
		return authority, 0, err
	}
	authority.Context = assemblyline.CloneObjectiveContext(result.Context)
	return authority, result.ModelCalls, nil
}

// resolveRoleplayTurnRetrieval resolves fixed search availability once for an
// ordered response round. Each responder compilation uses its exact
// authoritative instruction as the sole query when search is available.
func resolveRoleplayTurnRetrieval(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	candidateProvider objectiveContextCandidateSource,
	preparation roleplay.SimulationTurnAuthority,
) (contextcompiler.RetrievalDirective, error) {
	contextInstruction, err := objectiveContextInstruction(authority)
	if err != nil {
		return contextcompiler.RetrievalDirective{}, err
	}
	if preparation.InputKind == roleplay.SimulationTurnAction {
		return contextcompiler.RetrievalDirective{
			Availability: contextcompiler.SearchUnavailable,
		}, nil
	}
	availability := contextcompiler.SearchUnavailable
	for index, responder := range preparation.Responders {
		responderAuthority, projection, err := roleplayResponderTurnAuthority(
			authority, responder, nil,
		)
		if err != nil {
			return contextcompiler.RetrievalDirective{}, fmt.Errorf(
				"project roleplay responder %d search authority: %w", index, err,
			)
		}
		current, err := (boundContextCandidateSource{
			source: candidateProvider, job: job, authority: responderAuthority,
			preparation: &preparation, projection: &projection,
		}).SearchAvailability(ctx)
		if err != nil {
			return contextcompiler.RetrievalDirective{}, fmt.Errorf(
				"inspect roleplay responder %d search availability: %w", index, err,
			)
		}
		if err := current.Validate(); err != nil {
			return contextcompiler.RetrievalDirective{}, fmt.Errorf(
				"roleplay responder %d: %w", index, err,
			)
		}
		if current == contextcompiler.SearchAvailable {
			availability = current
			break
		}
	}
	directive, err := contextcompiler.ResolveRetrievalDirective(
		ctx, contextInstruction, assemblyline.ContextScopeRoleplaySimulation,
		availability,
	)
	if err != nil {
		return contextcompiler.RetrievalDirective{}, err
	}
	return directive, nil
}

func objectiveContextScope(authority turnAuthority) (assemblyline.ContextScope, error) {
	switch authority.ChannelMode {
	case model.ChannelModeAssistant:
		return "", nil
	case model.ChannelModeRoleplay:
		return assemblyline.ContextScopeRoleplaySimulation, nil
	default:
		return "", fmt.Errorf(
			"context compilation has unsupported channel mode %q", authority.ChannelMode,
		)
	}
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
