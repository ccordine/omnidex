package modelgauntlet

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (runner *completeRequirementRunner) callDirectPartition(
	input assemblyline.RequirementPartitionInput,
	kind string,
) (completePartitionAttempt, error) {
	operation, err := runner.nextOperation(kind)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	job, err := assemblyline.NewRequirementPartitionJob(input)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	response, failure, err := runner.callStructured(operation, StageDirect, prompt, schema)
	if err != nil || failure != "" {
		return completePartitionAttempt{Error: failure}, err
	}
	return decodeCompletePartitionCall(response.Content, input)
}

func (runner *completeRequirementRunner) callPerSplitPartition(
	input assemblyline.RequirementPartitionInput,
	kind string,
) (completePartitionAttempt, error) {
	operation, err := runner.nextOperation(kind)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	briefingJob, err := assemblyline.NewRequirementPartitionBriefingJob(input)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	briefingPrompt, briefingSchema, err := assemblyline.RenderPortableJob(briefingJob)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	briefingResponse, failure, err := runner.callStructured(operation+".briefing", StageBriefing, briefingPrompt, briefingSchema)
	if err != nil || failure != "" {
		return completePartitionAttempt{Error: failure}, err
	}
	briefing, decodeErr := assemblyline.DecodeRequirementPartitionBriefing(briefingResponse.Content)
	if decodeErr != nil {
		return completePartitionAttempt{Error: decodeErr.Error()}, nil
	}

	advisoryJob, err := assemblyline.NewRequirementPartitionAdvisoryJob(assemblyline.RequirementPartitionAdvisoryInput{
		Original: input, Lens: briefing.Lens,
	})
	if err != nil {
		return completePartitionAttempt{}, err
	}
	advisoryPrompt, advisorySchema, err := assemblyline.RenderPortableJob(advisoryJob)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	if advisorySchema != nil {
		return completePartitionAttempt{}, fmt.Errorf("per-split advisory renderer returned a response schema")
	}
	advisoryResponse, failure := runner.callAdvisory(operation+".advisory", advisoryPrompt)
	if failure != "" {
		return completePartitionAttempt{Error: failure}, nil
	}
	if strings.TrimSpace(advisoryResponse.Content) == "" {
		return completePartitionAttempt{Error: "per-split advisory returned no final memo content"}, nil
	}

	synthesisJob, err := assemblyline.NewRequirementPartitionSynthesisJob(assemblyline.RequirementPartitionSynthesisInput{
		Original: input, AdvisoryMemo: advisoryResponse.Content,
	})
	if err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	synthesisPrompt, synthesisSchema, err := assemblyline.RenderPortableJob(synthesisJob)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	response, failure, err := runner.callStructured(operation+".synthesis", StageSynthesis, synthesisPrompt, synthesisSchema)
	if err != nil || failure != "" {
		return completePartitionAttempt{Error: failure}, err
	}
	return decodeCompletePartitionCall(response.Content, input)
}

func (runner *completeRequirementRunner) callFinalPass(
	source string,
	direct assemblyline.RequirementPartitionDecision,
) (completePartitionAttempt, error) {
	subject, err := assemblyline.NewRequirementFinalSubject(source, direct)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	advisoryJob, err := assemblyline.NewRequirementFinalAdvisoryJob(subject)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	advisoryPrompt, schema, err := assemblyline.RenderPortableJob(advisoryJob)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	if schema != nil {
		return completePartitionAttempt{}, fmt.Errorf("final advisory renderer returned a response schema")
	}
	advisory, failure := runner.callAdvisoryWithBudget(
		"final.advisory", advisoryPrompt, maxFinalRequirementDeliberationTokens,
	)
	if failure != "" {
		return completePartitionAttempt{Error: failure}, nil
	}
	if strings.TrimSpace(advisory.Content) == "" {
		return completePartitionAttempt{Error: "final advisory returned no final memo content"}, nil
	}
	synthesisJob, err := assemblyline.NewRequirementFinalSynthesisJob(subject, advisoryJob, advisory.Content)
	if err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	prompt, responseSchema, err := assemblyline.RenderPortableJob(synthesisJob)
	if err != nil {
		return completePartitionAttempt{}, err
	}
	response, failure, err := runner.callStructured("final.synthesis", StageSynthesis, prompt, responseSchema)
	if err != nil || failure != "" {
		return completePartitionAttempt{Error: failure}, err
	}
	decision, decodeErr := decodeCompletePartitionDecision(response.Content)
	if decodeErr != nil {
		return completePartitionAttempt{Error: decodeErr.Error()}, nil
	}
	if err := assemblyline.ValidateCompleteRequirementPartition(source, decision); err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	return completePartitionAttempt{Decision: decision}, nil
}

func (runner *completeRequirementRunner) callStructured(
	operation string,
	stage CallStage,
	prompt string,
	schema map[string]any,
) (GenerateResponse, string, error) {
	request, err := structuredAdvisoryRequest(advisoryProtocolConfig{
		StableModel: runner.config.StableModel, ReasoningModel: runner.config.ReasoningModel,
		ContextTokens: runner.config.ContextTokens, KeepAlive: runner.config.KeepAlive,
	}, runner.caseID, runner.variant, stage, prompt, schema, maxStructuredTokens)
	if err != nil {
		return GenerateResponse{}, "", err
	}
	request.Repetition = runner.repetition
	request.Operation = operation
	response, evidence, callErr := callWithEvidence(runner.ctx, runner.generator, request)
	runner.report.Calls = append(runner.report.Calls, evidence)
	if callErr != nil {
		return GenerateResponse{}, "model call failed: " + callErr.Error(), nil
	}
	return response, "", nil
}

func (runner *completeRequirementRunner) callAdvisory(operation, prompt string) (GenerateResponse, string) {
	return runner.callAdvisoryWithBudget(operation, prompt, maxDeliberationTokens)
}

func (runner *completeRequirementRunner) callAdvisoryWithBudget(
	operation, prompt string,
	maxOutputTokens int,
) (GenerateResponse, string) {
	request := GenerateRequest{
		CaseID: runner.caseID, Repetition: runner.repetition, Operation: operation,
		Variant: runner.variant, Stage: StageDeliberation, Model: runner.config.ReasoningModel,
		SystemPrompt: prompt, UserPrompt: "Produce the bounded advisory memo now.", Think: true,
		MaxOutputTokens: maxOutputTokens, ContextTokens: runner.config.ContextTokens,
		KeepAlive: runner.config.KeepAlive,
	}
	response, evidence, err := callWithEvidence(runner.ctx, runner.generator, request)
	runner.report.Calls = append(runner.report.Calls, evidence)
	if err != nil {
		return GenerateResponse{}, "advisory call failed: " + err.Error()
	}
	return response, ""
}

func decodeCompletePartitionCall(
	raw string,
	input assemblyline.RequirementPartitionInput,
) (completePartitionAttempt, error) {
	decision, err := decodeCompletePartitionDecision(raw)
	if err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	if err := decision.ValidateFor(input); err != nil {
		return completePartitionAttempt{Error: err.Error()}, nil
	}
	return completePartitionAttempt{Decision: decision}, nil
}

func decodeCompletePartitionDecision(raw string) (assemblyline.RequirementPartitionDecision, error) {
	var decision assemblyline.RequirementPartitionDecision
	if err := decodeExactJSON(raw, &decision); err != nil {
		return assemblyline.RequirementPartitionDecision{}, fmt.Errorf("invalid requirement partition: %w", err)
	}
	return decision, nil
}
