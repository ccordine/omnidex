package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	toolruntime "github.com/gryph/omnidex/internal/tools"
	"github.com/gryph/omnidex/internal/verification"
)

func (r *nativeRuntimeV3) runVerification() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	draft, err := r.readResponseDraftArtifact()
	if err != nil {
		return err
	}
	if _, directCoding := buildV3CodingCoordinatorPlan(intent); directCoding {
		evidenceRecords, err := r.svc.repo.ListCurrentEvidenceByJob(r.ctx, r.claim.Job.ID, 256)
		if err != nil {
			return fmt.Errorf("list evidence for deterministic coding verification: %w", err)
		}
		artifact, handled, err := buildDeterministicV3CodingVerification(intent, evidenceRecords)
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("deterministic coding verification declined a coding coordinator intent")
		}
		if err := r.writeArtifact(artifacts.KindVerification, artifact); err != nil {
			return err
		}
		summary := strings.Join([]string{
			"verdict=" + artifact.Verdict,
			fmt.Sprintf("objectives=%d", len(artifact.ObjectiveCoverage)),
			"authority=deterministic_evidence",
		}, "\n")
		r.svc.emitStepEvent(r.claim.Step.ID, "coding_evidence_verified", "model_calls=0 "+safeLine(summary, "verification completed"))
		return r.complete("verification", summary, summary)
	}
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "verifier", toolruntime.Call{
		Name:  "evidence.inspect",
		Input: map[string]any{"job_id": r.claim.Job.ID},
	})
	if err != nil {
		return err
	}
	if _, err := decodeToolOutput[struct {
		Summary string           `json:"summary"`
		Records []map[string]any `json:"records"`
	}](result); err != nil {
		return err
	}
	evidenceRecords, err := r.svc.repo.ListCurrentEvidenceByJob(r.ctx, r.claim.Job.ID, 256)
	if err != nil {
		return fmt.Errorf("list evidence for independent verification: %w", err)
	}
	supportedClaims, unsupportedClaims, err := r.persistDeterministicClaimAssessment(draft.Response, independentV3EvidenceRecords(evidenceRecords))
	if err != nil {
		return err
	}
	verificationInput := buildV3VerificationInput(intent, draft.Response, evidenceRecords)
	payload := map[string]any{
		"objective_ledger":    verificationInput.ObjectiveLedger,
		"completion_criteria": verificationInput.CompletionCriteria,
		"draft_response":      verificationInput.DraftResponse,
		"evidence":            verificationInput.Evidence,
	}
	criteria := append([]string{"challenge the response independently of planner conclusions and memory", "account for every objective using observed evidence"}, intent.CompletionCriteria...)
	invocation, err := r.invocationFor(
		"verifier",
		"independent_completion_challenge",
		"Independently determine whether the response proves the authoritative objectives were completed",
		100,
		criteria,
		[]string{artifactRef(artifacts.KindIntent, r.claim.Job.ID), artifactRef(artifacts.KindResponseDraft, r.claim.Job.ID)},
		payload,
	)
	if err != nil {
		return err
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "independent_challenge_started", fmt.Sprintf("objectives=%d evidence=%d", len(intent.Objectives), len(verificationInput.Evidence)))
	modelName := r.svc.v3SpecialistModel(r.claim.Job, r.routing, "verifier", specialist.RoleReviewVerificationSpecialist, r.routing.Analyze)
	output, err := r.invokeSpecialist("v3_independent_verification", "verifier", modelName, invocation, nil)
	if err != nil {
		return err
	}
	artifact, err := decodeV3TypedOutput[artifacts.VerificationArtifact](output)
	if err != nil {
		return err
	}
	if !artifact.IndependentChallenge {
		return fmt.Errorf("verifier did not perform the required independent challenge")
	}
	artifact.SupportedClaims = uniqueStrings(append(artifact.SupportedClaims, supportedClaims...))
	artifact.UnsupportedClaims = uniqueStrings(append(artifact.UnsupportedClaims, unsupportedClaims...))
	if intent.RequiresAction && !hasV3ExecutionEvidence(evidenceRecords) {
		artifact.MissingEvidence = uniqueStrings(append(artifact.MissingEvidence, "action completion has no execution evidence"))
	}
	if intentRequiresV3Capability(intent, capabilityWorkspaceWrite) && !hasSuccessfulV3GeneratedDiff(evidenceRecords) {
		artifact.MissingEvidence = uniqueStrings(append(artifact.MissingEvidence, "workspace.write has no successful generated-diff evidence"))
	}
	if intentRequiresV3Capability(intent, capabilityCommandExecute) && !hasSuccessfulV3CommandEvidence(evidenceRecords) {
		artifact.MissingEvidence = uniqueStrings(append(artifact.MissingEvidence, "command.execute has no successful command evidence"))
	}
	for _, command := range unresolvedV3CommandFailures(evidenceRecords) {
		artifact.MissingEvidence = uniqueStrings(append(artifact.MissingEvidence, "latest verification command failed: "+command))
	}
	if verificationHasBlockingFindings(artifact) && artifact.Verdict == artifacts.VerificationVerdictPass {
		artifact.Verdict = artifacts.VerificationVerdictRevise
		r.svc.emitStepEvent(r.claim.Step.ID, "independent_challenge_overruled_pass", "deterministic evidence checks found blocking gaps")
	}
	if err := validateV3VerificationArtifact(intent, artifact, evidenceRecords); err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindVerification, artifact); err != nil {
		return err
	}
	if err := r.writeEvidence(evidence.Record{Kind: evidence.KindModelJudgment, SourceType: "runtime_v3", SourceRef: "independent_verification", Summary: "Independent verification completed with verdict " + artifact.Verdict + ".", Confidence: 1, SupportsClaims: artifact.SupportedClaims, Warnings: append(append([]string{}, artifact.UnsupportedClaims...), artifact.MissingEvidence...)}); err != nil {
		return err
	}
	summary := strings.Join([]string{"verdict=" + artifact.Verdict, fmt.Sprintf("supported_claims=%d", len(artifact.SupportedClaims)), fmt.Sprintf("unsupported_claims=%d", len(artifact.UnsupportedClaims)), fmt.Sprintf("missing_evidence=%d", len(artifact.MissingEvidence))}, "\n")
	r.svc.emitStepEvent(r.claim.Step.ID, "independent_challenge_completed", safeLine(summary, "verification completed"))
	return r.complete("verification", summary, summary)
}

func (r *nativeRuntimeV3) persistDeterministicClaimAssessment(draft string, records []evidence.Record) ([]string, []string, error) {
	assessments := verification.AssessClaims(draft, records, 16)
	supported := make([]string, 0, len(assessments))
	unsupported := make([]string, 0, len(assessments))
	claims := make([]model.ClaimRecord, 0, len(assessments))
	for _, assessment := range assessments {
		status := "unsupported"
		if assessment.Supported {
			status = "supported"
			supported = append(supported, assessment.Text)
		} else {
			unsupported = append(unsupported, assessment.Text)
		}
		claims = append(claims, model.ClaimRecord{JobID: r.claim.Job.ID, StepID: r.claim.Step.ID, Text: assessment.Text, NormalizedText: assessment.Normalized, Status: status, Confidence: assessment.SupportScore})
	}
	saved, err := r.svc.repo.WriteClaims(r.ctx, claims)
	if err != nil {
		return nil, nil, err
	}
	links := make([]model.ClaimSupportRecord, 0, len(assessments)*2)
	for index, claim := range saved {
		for _, evidenceID := range assessments[index].EvidenceRefs {
			links = append(links, model.ClaimSupportRecord{ClaimID: claim.ID, EvidenceID: evidenceID, SupportScore: assessments[index].SupportScore, Rationale: assessments[index].Rationale})
		}
	}
	if err := r.svc.repo.WriteClaimSupports(r.ctx, links); err != nil {
		return nil, nil, err
	}
	return supported, unsupported, nil
}
