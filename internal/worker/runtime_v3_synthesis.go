package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialist"
)

func (r *nativeRuntimeV3) runAnalysis() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	delegated, err := r.collectSubtaskResults()
	if err != nil {
		return err
	}
	if artifact, handled, buildErr := buildDeterministicV3CodingAnalysis(intent, delegated); handled {
		if buildErr != nil {
			return buildErr
		}
		if err := r.writeArtifact(artifacts.KindAnalysis, artifact); err != nil {
			return err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "coding_analysis_derived", "source=accepted_subtask_result model_calls=0")
		return r.complete("analysis", artifact.Summary, artifact.Summary)
	}
	workspaceArtifact, err := r.readWorkspaceArtifact()
	if err != nil {
		return err
	}
	retrievalArtifact, err := r.readRetrievalArtifact()
	if err != nil {
		return err
	}
	webArtifact, err := r.readWebArtifact()
	if err != nil {
		return err
	}
	projection := projectV3Memory(intent, retrievalArtifact, projectTag(r.claim.Job), sessionTagForJob(r.claim.Job), r.svc.retrievalLimit)
	payload := map[string]any{
		"intent":            intent,
		"workspace":         workspaceArtifact,
		"memory_references": projection,
		"web_evidence":      webArtifact,
		"subtask_results":   delegated,
	}
	invocation, err := r.invocationFor(
		"analysis_specialist",
		"synthesize_grounded_findings",
		intent.UserGoal,
		90,
		intent.CompletionCriteria,
		[]string{artifactRef(artifacts.KindIntent, r.claim.Job.ID), artifactRef(artifacts.KindWorkspace, r.claim.Job.ID), artifactRef(artifacts.KindRetrieval, r.claim.Job.ID), artifactRef(artifacts.KindWebEvidence, r.claim.Job.ID)},
		payload,
	)
	if err != nil {
		return err
	}
	modelName := r.svc.v3SpecialistModel(r.claim.Job, r.routing, "analysis_specialist", specialist.RoleAnalysisSpecialist, r.routing.Analyze)
	output, err := r.invokeSpecialist("v3_analysis", "analysis_specialist", modelName, invocation, nil)
	if err != nil {
		return err
	}
	artifact, err := decodeV3TypedOutput[artifacts.AnalysisArtifact](output)
	if err != nil {
		return err
	}
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	if artifact.Summary == "" {
		return fmt.Errorf("analysis specialist returned an empty analysis")
	}
	if err := r.writeArtifact(artifacts.KindAnalysis, artifact); err != nil {
		return err
	}
	return r.complete("analysis", artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runResponseDraft() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	analysisArtifact, err := r.readAnalysisArtifact()
	if err != nil {
		return err
	}
	delegated, err := r.collectSubtaskResults()
	if err != nil {
		return err
	}
	if artifact, handled, buildErr := buildDeterministicV3CodingResponse(intent, analysisArtifact, delegated); handled {
		if buildErr != nil {
			return buildErr
		}
		if err := r.writeArtifact(artifacts.KindResponseDraft, artifact); err != nil {
			return err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "coding_response_derived", "source=accepted_coding_summary model_calls=0")
		return r.complete("response_draft", artifact.Response, artifact.Response)
	}
	records, err := r.svc.repo.ListCurrentEvidenceByJob(r.ctx, r.claim.Job.ID, 256)
	if err != nil {
		return fmt.Errorf("list evidence for response composition: %w", err)
	}
	verificationInput := buildV3VerificationInput(intent, "", records)
	payload := map[string]any{
		"intent":          intent,
		"analysis":        analysisArtifact,
		"evidence":        verificationInput.Evidence,
		"subtask_results": delegated,
	}
	invocation, err := r.invocationFor(
		"response_composer",
		"compose_evidence_grounded_response",
		intent.UserGoal,
		80,
		intent.CompletionCriteria,
		[]string{artifactRef(artifacts.KindIntent, r.claim.Job.ID), artifactRef(artifacts.KindAnalysis, r.claim.Job.ID)},
		payload,
	)
	if err != nil {
		return err
	}
	modelName := r.svc.v3SpecialistModel(r.claim.Job, r.routing, "response_composer", specialist.RoleResponseSpecialist, r.routing.Response)
	output, err := r.invokeSpecialist("v3_response_draft", "response_composer", modelName, invocation, nil)
	if err != nil {
		return err
	}
	typedOutput, err := decodeV3TypedOutput[struct {
		Response string `json:"response"`
	}](output)
	if err != nil {
		return err
	}
	draft := strings.TrimSpace(typedOutput.Response)
	if draft == "" {
		return fmt.Errorf("response composer returned an empty response")
	}
	artifact := artifacts.ResponseDraftArtifact{Response: draft}
	if err := r.writeArtifact(artifacts.KindResponseDraft, artifact); err != nil {
		return err
	}
	return r.complete("response_draft", artifact.Response, artifact.Response)
}
