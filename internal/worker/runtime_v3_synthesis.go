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
	delegated, err := r.collectSubtaskResults()
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
	modelName := r.svc.v3SpecialistModel(r.claim.Job, "analysis_specialist", specialist.RoleAnalysisSpecialist, r.svc.models.Analyze)
	output, err := r.invokeSpecialist("v3_analysis", "analysis_specialist", modelName, invocation, nil)
	if err != nil {
		return err
	}
	artifact, err := decodeV3TypedOutput[artifacts.AnalysisArtifact](output)
	if err != nil {
		return err
	}
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	if artifact.Summary == "" || genericNonAnswer(artifact.Summary) {
		return fmt.Errorf("analysis specialist returned a non-substantive analysis")
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
	records, err := r.svc.repo.ListEvidenceByJob(r.ctx, r.claim.Job.ID, 256)
	if err != nil {
		return fmt.Errorf("list evidence for response composition: %w", err)
	}
	delegated, err := r.collectSubtaskResults()
	if err != nil {
		return err
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
	modelName := r.svc.v3SpecialistModel(r.claim.Job, "response_composer", specialist.RoleResponseSpecialist, r.svc.models.Response)
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
	if draft == "" || genericNonAnswer(draft) {
		return fmt.Errorf("response composer returned a non-substantive response")
	}
	artifact := artifacts.ResponseDraftArtifact{Response: draft}
	if err := r.writeArtifact(artifacts.KindResponseDraft, artifact); err != nil {
		return err
	}
	return r.complete("response_draft", artifact.Response, artifact.Response)
}
