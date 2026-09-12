package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/webresearch"
)

func runObjectiveTurn(
	ctx context.Context,
	job model.Job,
	candidateProvider objectiveContextCandidateSource,
	contextStation objectiveContextSieveStations,
	kindStation objectiveKindStation,
	conversationStation objectiveConversationStation,
	answerStation objectiveAnswerStation,
	workflows objectiveWorkflows,
) (objectiveTurnResult, error) {
	if ctx == nil {
		return objectiveTurnResult{}, fmt.Errorf("conversation objective requires a context")
	}
	if err := ctx.Err(); err != nil {
		return objectiveTurnResult{}, err
	}
	authority, err := newTurnAuthority(job)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	if authority.ChannelMode == model.ChannelModeRoleplay {
		provenance, err := resolveObjectiveModelPathProvenance(workflows)
		if err != nil {
			return objectiveTurnResult{}, err
		}
		authority, err = bindObjectiveModelInstruction(authority, provenance)
		if err != nil {
			return objectiveTurnResult{}, err
		}
		if workflows.RoleplaySimulation == nil {
			return objectiveTurnResult{}, fmt.Errorf("roleplay character context projection is unavailable")
		}
		preparation, projection, err := workflows.RoleplaySimulation(
			ctx, authority.RoleplaySimulationPreparationID, authority.JobID,
		)
		if err != nil {
			return objectiveTurnResult{}, err
		}
		if err := preparation.Validate(); err != nil {
			return objectiveTurnResult{}, fmt.Errorf("roleplay turn preparation: %w", err)
		}
		if err := projection.Validate(); err != nil {
			return objectiveTurnResult{}, fmt.Errorf("roleplay narrative authority: %w", err)
		}
		if err := requireObjectiveRoleplayPreparation(authority, preparation); err != nil {
			return objectiveTurnResult{}, err
		}
		if authority.RoleplayInputKind == roleplay.SimulationTurnExternalCommand {
			authority, contextCalls, err := compileObjectiveTurnContext(
				ctx, job, authority, candidateProvider, contextStation,
				&preparation, &projection, nil,
			)
			if err != nil {
				return objectiveTurnResult{}, err
			}
			result, err := runObjectiveRoleplayResearchTurn(ctx, authority, workflows.RoleplayResearch)
			result.ModelCalls += contextCalls
			return result, err
		}
		return runObjectiveRoleplayTurn(
			ctx, job, authority, candidateProvider, contextStation, conversationStation,
			preparation,
			workflows.RoleplayCanon,
			workflows.RoleplayCanonDelta,
			workflows.RoleplayOngoingAction,
		)
	}
	authority, err = bindObjectiveModelInstruction(
		authority, assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	input := assemblyline.ConversationObjectiveKindInput{
		ExactInstruction:          authority.ModelInstruction,
		Context:                   assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		DatabaseEvidenceAvailable: authority.DataSourceID != "",
		KnownArtifactPaths:        []string{},
	}
	if kindStation == nil {
		return objectiveTurnResult{}, fmt.Errorf("conversation objective kind station is unavailable")
	}
	decision, dispatches, err := kindStation.Classify(ctx, input)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return objectiveTurnResult{}, err
	}
	kindCalls := dispatches
	result := objectiveTurnResult{
		ObjectiveID: objectiveTurnID(authority), Kind: decision.Kind,
		ModelCalls: kindCalls,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	if decision.Kind == assemblyline.ObjectiveKindWorkspaceMutation {
		if workflows.WorkspaceContinuity == nil {
			return result, fmt.Errorf("workspace mutation continuity authority is unavailable")
		}
		continuity, err := workflows.WorkspaceContinuity(ctx, job)
		if err != nil {
			return result, err
		}
		authority.Context = assemblyline.CloneObjectiveContext(continuity.ReplanContext())
		authority.SessionContext = continuity.SessionContext()
		result.ObjectiveID = objectiveTurnID(authority)
		result.RequirementID = objectiveRequirementID(result.ObjectiveID)
		return runObjectiveWorkspaceMutation(ctx, authority, result, workflows.WorkspaceMutation)
	}
	authority, contextCalls, err := compileObjectiveTurnContext(
		ctx, job, authority, candidateProvider, contextStation, nil, nil, nil,
	)
	if err != nil {
		return result, err
	}
	result.ModelCalls += contextCalls
	if decision.Kind == assemblyline.ObjectiveKindAnswer || decision.Kind == assemblyline.ObjectiveKindStory {
		return runObjectiveConversationResponse(ctx, authority, result, conversationStation, "")
	}
	if decision.Kind == assemblyline.ObjectiveKindDatabaseRead {
		return runObjectiveDatabaseRead(ctx, authority, result, answerStation, workflows.DatabaseRead)
	}
	return result, fmt.Errorf("conversation objective kind %q has no code-owned workflow", decision.Kind)
}

func resolveObjectiveModelPathProvenance(
	workflows objectiveWorkflows,
) (assemblyline.ArtifactIdentityProvenance, error) {
	if workflows.ResolveModelPathProvenance == nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"objective artifact provenance resolver is unavailable",
		)
	}
	return workflows.ResolveModelPathProvenance()
}

func runObjectiveRoleplayResearchTurn(
	ctx context.Context,
	authority turnAuthority,
	run func(context.Context, turnAuthority) (objectiveRoleplayResearchAnswer, error),
) (objectiveTurnResult, error) {
	result := objectiveTurnResult{
		ObjectiveID: objectiveTurnID(authority),
		Kind:        assemblyline.ObjectiveKindExternalAnswer,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	if run == nil {
		return result, fmt.Errorf("roleplay research workflow is unavailable")
	}
	answer, err := run(ctx, authority)
	result.ModelCalls = answer.ModelCalls
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if answer.ModelCalls < 0 || answer.ModelCalls > maximumObjectiveRoleplayResearchModelCalls {
		return result, fmt.Errorf(
			"roleplay research reported %d calls outside 0..%d",
			answer.ModelCalls, maximumObjectiveRoleplayResearchModelCalls,
		)
	}
	if answer.Sources.JobID != authority.JobID {
		return result, fmt.Errorf("roleplay research sources belong to a different job")
	}
	sources, _, err := answer.Sources.Acquired.Project()
	if err != nil {
		return result, err
	}
	if err := webresearch.ValidateCompletionArtifact(answer.Artifact, sources); err != nil {
		return result, err
	}
	if err := validateObjectiveRoleplayResearchTurn(authority, answer.Research); err != nil {
		return result, err
	}
	paragraphs := make([]string, len(answer.Artifact.Paragraphs))
	for index, paragraph := range answer.Artifact.Paragraphs {
		paragraphs[index] = paragraph.Text
	}
	if err := validateObjectiveModelInput(
		authority, "roleplay research model answer", strings.Join(paragraphs, "\n\n"),
	); err != nil {
		return result, err
	}
	projected, err := projectObjectiveRoleplayResearchEvidence(answer.Sources)
	if err != nil {
		return result, err
	}
	citations, _, err := bindObjectiveRoleplayResearchCitations(projected, answer.Artifact)
	if err != nil {
		return result, err
	}
	research := answer.Research
	result.Output, err = restoreObjectiveCodeRenderedArtifact(
		authority, "roleplay research answer", answer.Artifact.Rendered,
	)
	if err != nil {
		return result, err
	}
	result.Citations = citations
	result.CitationsRendered = true
	result.RoleplayResearch = &research
	result.Complete = true
	return result, nil
}

func runObjectiveDatabaseRead(
	ctx context.Context,
	authority turnAuthority,
	result objectiveTurnResult,
	answerStation objectiveAnswerStation,
	resolve func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error),
) (objectiveTurnResult, error) {
	if authority.DataSourceID == "" {
		return result, fmt.Errorf("database-read objective has no explicit data-source binding")
	}
	if resolve == nil {
		return result, fmt.Errorf("database-read workflow is unavailable")
	}
	acquisition, err := resolve(ctx, authority, result.RequirementID)
	result.ModelCalls += acquisition.ModelCalls
	if err != nil {
		return result, err
	}
	modelEvidence, err := objectiveModelEvidence(acquisition.Evidence)
	if err != nil {
		return result, err
	}
	if answerStation == nil {
		return result, fmt.Errorf("database-read objective requires a grounded answer station")
	}
	input := assemblyline.GroundedAnswerInput{
		RequirementID: result.RequirementID, ExactRequirement: authority.ModelInstruction,
		Context: assemblyline.CloneObjectiveContext(authority.Context), Evidence: modelEvidence,
		KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
	}
	if err := input.Validate(); err != nil {
		return result, err
	}
	answer, dispatches, err := answerStation.Answer(ctx, input)
	result.ModelCalls += dispatches
	if err != nil {
		return result, err
	}
	if err := validateObjectiveGroundedAnswerCalls(dispatches, input); err != nil {
		return result, fmt.Errorf("database grounded answer: %w", err)
	}
	if err := answer.ValidateFor(input); err != nil {
		return result, err
	}
	citations, err := selectObjectiveCitations(acquisition.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	result.Output, err = restoreObjectiveModelText(
		authority, "database grounded answer", answer.Text,
	)
	if err != nil {
		return result, err
	}
	result.Citations = citations
	result.Complete = true
	return result, nil
}

func runObjectiveWorkspaceMutation(
	ctx context.Context,
	authority turnAuthority,
	result objectiveTurnResult,
	mutate func(context.Context, turnAuthority) (string, error),
) (objectiveTurnResult, error) {
	if mutate == nil {
		return result, fmt.Errorf("workspace mutation workflow is unavailable")
	}
	output, err := mutate(ctx, authority)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.Output = output
	return result, nil
}
