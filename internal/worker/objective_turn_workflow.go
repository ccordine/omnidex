package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
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
	if _, err := assemblyline.NewConversationObjectiveKindJob(input); err != nil {
		return objectiveTurnResult{}, err
	}
	decision, receipt, err := kindStation.Classify(ctx, input)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return objectiveTurnResult{}, err
	}
	kindCalls := receipt.Calls
	if err := decision.ValidateFor(input); err != nil {
		return objectiveTurnResult{}, err
	}
	result := objectiveTurnResult{
		ObjectiveID: objectiveTurnID(authority, decision.Kind), Kind: decision.Kind,
		InstructionSHA256: authority.SHA256, ModelCalls: kindCalls,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	if decision.Kind == assemblyline.ObjectiveKindWorkspaceMutation {
		return runObjectiveWorkspaceMutation(ctx, authority, result, workflows.WorkspaceMutation)
	}
	if decision.Kind == assemblyline.ObjectiveKindRepositoryRead {
		provenance, err := resolveObjectiveModelPathProvenance(workflows)
		if err != nil {
			return result, err
		}
		authority, err = bindObjectiveModelInstruction(authority, provenance)
		if err != nil {
			return result, err
		}
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
	if decision.Kind == assemblyline.ObjectiveKindExternalAnswer {
		return runObjectiveExternalAnswer(ctx, authority, result, workflows.ExternalAnswer)
	}
	if decision.Kind == assemblyline.ObjectiveKindDatabaseRead {
		return runObjectiveDatabaseRead(ctx, authority, result, answerStation, workflows.DatabaseRead)
	}
	if answerStation == nil {
		return result, fmt.Errorf("repository-read objective requires one grounded-answer station")
	}
	acquisition, err := acquireObjectiveEvidence(ctx, authority, decision.Kind, workflows)
	if err != nil {
		return result, err
	}
	acquisitionCalls, err := objectiveRepositoryEvidenceCallTotal(acquisition)
	if err != nil {
		return result, err
	}
	result.ModelCalls += acquisitionCalls
	if err := ctx.Err(); err != nil {
		return result, err
	}
	modelEvidence, err := objectiveModelEvidence(acquisition.Evidence)
	if err != nil {
		return result, err
	}
	answerInput := assemblyline.GroundedAnswerInput{
		RequirementID:    result.RequirementID,
		ExactRequirement: acquisition.GroundedRequirement,
		Context:          assemblyline.CloneObjectiveContext(authority.Context),
		Evidence:         modelEvidence,
		KnownArtifactPaths: append(
			[]string(nil), acquisition.KnownArtifactPaths...,
		),
	}
	grounded, err := runObjectiveRepositoryGroundedClosure(
		ctx,
		answerInput,
		answerStation,
	)
	if err != nil {
		return result, err
	}
	result.ModelCalls += grounded.ModelCalls
	citations, err := selectObjectiveCitations(acquisition.Evidence, grounded.Answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	result.Output, err = assemblyline.RestoreArtifactIdentities(
		grounded.Answer.Text, acquisition.ArtifactIdentities,
	)
	if err != nil {
		return result, fmt.Errorf("restore repository answer artifact identities: %w", err)
	}
	result.Citations = citations
	result.Complete = true
	return result, nil
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
		ObjectiveID: objectiveTurnID(authority, assemblyline.ObjectiveKindExternalAnswer),
		Kind:        assemblyline.ObjectiveKindExternalAnswer, InstructionSHA256: authority.SHA256,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	if run == nil {
		return result, fmt.Errorf("roleplay research workflow is unavailable")
	}
	answer, err := run(ctx, authority)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	webReceipt, err := answer.WebCallLedger.ValidateForMaximum(
		"roleplay research completion", maximumObjectiveRoleplayResearchModelCalls,
	)
	if err != nil {
		return result, err
	}
	if answer.ModelCalls != webReceipt.Calls {
		return result, fmt.Errorf(
			"roleplay research reported %d calls but its exact ledger proves %d",
			answer.ModelCalls, webReceipt.Calls,
		)
	}
	if strings.TrimSpace(answer.Text) == "" || answer.Text != strings.TrimSpace(answer.Text) ||
		len(answer.Text) > maxObjectiveOutputBytes ||
		answer.ModelCalls > maximumObjectiveRoleplayResearchModelCalls ||
		answer.Rendered == "" || answer.Rendered != strings.TrimSpace(answer.Rendered) ||
		len(answer.Rendered) > maxObjectiveOutputBytes ||
		!validObjectiveTextSHA(answer.Rendered, answer.RenderedSHA256) || len(answer.Paragraphs) == 0 {
		return result, fmt.Errorf("roleplay research returned invalid bounded completion authority")
	}
	if err := validateObjectiveRoleplayResearchTurn(authority, answer.Research); err != nil {
		return result, err
	}
	if err := validateObjectiveModelInput(
		authority, "roleplay research model answer", answer.Text,
	); err != nil {
		return result, err
	}
	citations, err := selectObjectiveCitations(answer.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	research := answer.Research
	result.Output, err = restoreObjectiveCodeRenderedArtifact(
		authority, "roleplay research answer", answer.Rendered,
	)
	if err != nil {
		return result, err
	}
	result.Citations = citations
	result.CitationsRendered = true
	result.ModelCalls = answer.ModelCalls
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
	if err != nil {
		return result, err
	}
	databaseReceipt, err := acquisition.DatabaseCallLedger.totalForSuccess()
	if err != nil {
		return result, fmt.Errorf("database-read workflow receipt: %w", err)
	}
	if acquisition.ModelCalls != databaseReceipt.Calls {
		return result, fmt.Errorf(
			"database-read workflow reported %d model calls but its exact receipt ledger proves %d",
			acquisition.ModelCalls, databaseReceipt.Calls,
		)
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
	answer, receipt, err := answerStation.Answer(ctx, input)
	if err != nil {
		return result, err
	}
	if err := validateObjectiveGroundedAnswerReceipt(receipt, input); err != nil {
		return result, fmt.Errorf("database grounded answer: %w", err)
	}
	if err := answer.ValidateFor(input); err != nil {
		return result, err
	}
	citations, err := selectObjectiveCitations(acquisition.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	result.ModelCalls += acquisition.ModelCalls + receipt.Calls
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

func runObjectiveExternalAnswer(
	ctx context.Context,
	authority turnAuthority,
	result objectiveTurnResult,
	run func(context.Context, turnAuthority) (objectiveExternalAnswer, error),
) (objectiveTurnResult, error) {
	if run == nil {
		return result, fmt.Errorf("external-answer workflow is unavailable")
	}
	answer, err := run(ctx, authority)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	webReceipt, err := answer.WebCallLedger.ValidateForMaximum(
		"external-answer completion", maxObjectiveWebSemanticCalls,
	)
	if err != nil {
		return result, err
	}
	if answer.ModelCalls != webReceipt.Calls {
		return result, fmt.Errorf(
			"external-answer workflow reported %d calls but its exact ledger proves %d",
			answer.ModelCalls, webReceipt.Calls,
		)
	}
	if strings.TrimSpace(answer.Text) == "" || answer.Text != strings.TrimSpace(answer.Text) ||
		len(answer.Text) > maxObjectiveOutputBytes ||
		answer.Rendered == "" || answer.Rendered != strings.TrimSpace(answer.Rendered) ||
		len(answer.Rendered) > maxObjectiveOutputBytes ||
		!validObjectiveTextSHA(answer.Rendered, answer.RenderedSHA256) || len(answer.Paragraphs) == 0 {
		return result, fmt.Errorf("external-answer workflow returned invalid bounded completion authority")
	}
	citations, err := selectObjectiveCitations(answer.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	if err := validateObjectiveModelInput(
		authority, "external grounded model answer", answer.Text,
	); err != nil {
		return result, err
	}
	result.Output, err = restoreObjectiveCodeRenderedArtifact(
		authority, "external grounded answer", answer.Rendered,
	)
	if err != nil {
		return result, err
	}
	result.Citations = citations
	result.CitationsRendered = true
	result.ModelCalls += answer.ModelCalls
	result.Complete = true
	return result, nil
}

func validObjectiveTextSHA(value, digest string) bool {
	sum := sha256.Sum256([]byte(value))
	return digest == hex.EncodeToString(sum[:])
}

func cloneGroundedAnswerInput(input assemblyline.GroundedAnswerInput) assemblyline.GroundedAnswerInput {
	copy := input
	copy.Context = assemblyline.CloneObjectiveContext(input.Context)
	copy.Evidence = append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...)
	copy.KnownArtifactPaths = append([]string(nil), input.KnownArtifactPaths...)
	return copy
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
	if strings.TrimSpace(output) == "" || len(output) > maxObjectiveOutputBytes {
		return result, fmt.Errorf("workspace mutation returned an empty or oversized completion artifact")
	}
	result.Output = output
	result.Complete = true
	return result, nil
}

func acquireObjectiveEvidence(
	ctx context.Context,
	authority turnAuthority,
	kind assemblyline.ConversationObjectiveKind,
	workflows objectiveWorkflows,
) (objectiveEvidenceAcquisition, error) {
	switch kind {
	case assemblyline.ObjectiveKindRepositoryRead:
		if workflows.RepositoryRead == nil {
			return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read workflow is unavailable")
		}
		return workflows.RepositoryRead(ctx, authority)
	default:
		return objectiveEvidenceAcquisition{}, fmt.Errorf("conversation objective kind %q has no code-owned workflow", kind)
	}
}
