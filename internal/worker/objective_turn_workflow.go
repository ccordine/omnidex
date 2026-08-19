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
	candidateProvider objectiveConversationCandidateProvider,
	contextStation objectiveContextSelectionStation,
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
		if authority.RoleplayInputKind == roleplay.SimulationTurnExternalCommand {
			return runObjectiveRoleplayResearchTurn(ctx, authority, workflows.RoleplayResearch)
		}
		return runObjectiveRoleplayTurn(
			ctx, authority, 0, conversationStation,
			workflows.RoleplaySimulation, workflows.RoleplayCanon,
		)
	}
	authority, contextCalls, err := resolveObjectiveConversationContext(
		ctx, job, authority, candidateProvider, contextStation,
	)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	memoryProvider, ok := candidateProvider.(objectiveMemoryContextCandidateProvider)
	if !ok {
		return objectiveTurnResult{}, fmt.Errorf("objective context provider lacks memory authority")
	}
	memorySelector, _ := contextStation.(objectiveMemoryContextSelectionStation)
	authority, memoryCalls, err := resolveObjectiveMemoryContext(
		ctx, job, authority, memoryProvider, memorySelector,
	)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	input := assemblyline.ConversationObjectiveKindInput{
		ExactInstruction:          authority.Instruction,
		Context:                   assemblyline.CloneObjectiveContext(authority.Context),
		DatabaseEvidenceAvailable: authority.DataSourceID != "",
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
	if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
		return objectiveTurnResult{}, fmt.Errorf(
			"conversation objective kind station reported %d calls outside the bounded correction budget", receipt.Calls,
		)
	}
	kindCalls := receipt.Calls
	if err := decision.ValidateFor(input); err != nil {
		return objectiveTurnResult{}, err
	}
	result := objectiveTurnResult{
		ObjectiveID: objectiveTurnID(authority, decision.Kind), Kind: decision.Kind,
		InstructionSHA256: authority.SHA256, ModelCalls: contextCalls + memoryCalls + kindCalls,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	if decision.Kind == assemblyline.ObjectiveKindWorkspaceMutation {
		return runObjectiveWorkspaceMutation(ctx, authority, result, workflows.WorkspaceMutation)
	}
	if decision.Kind == assemblyline.ObjectiveKindAnswer || decision.Kind == assemblyline.ObjectiveKindStory {
		return runObjectiveConversationResponse(ctx, authority, result, conversationStation)
	}
	if decision.Kind == assemblyline.ObjectiveKindExternalAnswer {
		return runObjectiveExternalAnswer(ctx, authority, result, workflows.ExternalAnswer)
	}
	if decision.Kind == assemblyline.ObjectiveKindDatabaseRead {
		return runObjectiveDatabaseRead(ctx, authority, result, answerStation, workflows.DatabaseRead)
	}
	repositoryStations, ok := answerStation.(objectiveRepositoryGroundingStation)
	if !ok || repositoryStations == nil {
		return result, fmt.Errorf("repository-read objective requires grounded answer, independent review, and correction stations")
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
		ExactRequirement: authority.Instruction,
		Context:          assemblyline.CloneObjectiveContext(authority.Context),
		Evidence:         modelEvidence,
	}
	grounded, err := runObjectiveRepositoryGroundedClosure(
		ctx,
		answerInput,
		repositoryStations,
		objectiveRepositoryGroundedClosureOptions{
			ObjectiveID: result.ObjectiveID,
			Generation:  job.CurrentGeneration,
			Advisory:    workflows.ObjectiveAdvisory,
		},
	)
	if err != nil {
		return result, err
	}
	result.ModelCalls += grounded.ModelCalls
	result.Advisory = grounded.Advisory
	citations, err := selectObjectiveCitations(acquisition.Evidence, grounded.Answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	result.Output = grounded.Answer.Text
	result.Citations = citations
	result.Complete = true
	return result, nil
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
	if strings.TrimSpace(answer.Text) == "" || answer.Text != strings.TrimSpace(answer.Text) ||
		len(answer.Text) > maxObjectiveOutputBytes || answer.ModelCalls != 1 ||
		answer.Rendered == "" || answer.Rendered != strings.TrimSpace(answer.Rendered) ||
		len(answer.Rendered) > maxObjectiveOutputBytes ||
		!validObjectiveTextSHA(answer.Rendered, answer.RenderedSHA256) || len(answer.Paragraphs) == 0 {
		return result, fmt.Errorf("roleplay research returned invalid bounded completion authority")
	}
	if err := validateObjectiveRoleplayResearchTurn(authority, answer.Research); err != nil {
		return result, err
	}
	citations, err := selectObjectiveCitations(answer.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	research := answer.Research
	result.Output = answer.Rendered
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
	if acquisition.ModelCalls < 1 {
		return result, fmt.Errorf("database-read workflow returned no bounded semantic work")
	}
	modelEvidence, err := objectiveModelEvidence(acquisition.Evidence)
	if err != nil {
		return result, err
	}
	if answerStation == nil {
		return result, fmt.Errorf("database-read objective requires a grounded answer station")
	}
	input := assemblyline.GroundedAnswerInput{
		RequirementID: result.RequirementID, ExactRequirement: authority.Instruction,
		Context: assemblyline.CloneObjectiveContext(authority.Context), Evidence: modelEvidence,
	}
	if _, err := assemblyline.NewGroundedAnswerJob(input); err != nil {
		return result, err
	}
	answer, receipt, err := answerStation.Answer(ctx, input)
	if err != nil {
		return result, err
	}
	if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
		return result, fmt.Errorf("database grounded answer reported %d calls outside the bounded correction budget", receipt.Calls)
	}
	if err := answer.ValidateFor(input); err != nil {
		return result, err
	}
	citations, err := selectObjectiveCitations(acquisition.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	result.ModelCalls += acquisition.ModelCalls + receipt.Calls
	result.Output = answer.Text
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
	if strings.TrimSpace(answer.Text) == "" || answer.Text != strings.TrimSpace(answer.Text) ||
		len(answer.Text) > maxObjectiveOutputBytes || answer.ModelCalls < 1 ||
		answer.Rendered == "" || answer.Rendered != strings.TrimSpace(answer.Rendered) ||
		len(answer.Rendered) > maxObjectiveOutputBytes ||
		!validObjectiveTextSHA(answer.Rendered, answer.RenderedSHA256) || len(answer.Paragraphs) == 0 {
		return result, fmt.Errorf("external-answer workflow returned invalid bounded completion authority")
	}
	citations, err := selectObjectiveCitations(answer.Evidence, answer.EvidenceIDs)
	if err != nil {
		return result, err
	}
	result.Output = answer.Rendered
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
