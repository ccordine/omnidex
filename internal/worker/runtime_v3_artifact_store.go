package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func (r *nativeRuntimeV3) complete(contextKey, output, contextValue string) error {
	output = strings.TrimSpace(output)
	contextValue = strings.TrimSpace(contextValue)
	if contextValue == "" {
		contextValue = output
	}
	r.contexts[contextKey] = contextValue
	command, err := completeClaimedStepCommand(r.claim, output, contextKey, contextValue)
	if err != nil {
		return err
	}
	return r.svc.repo.CompleteStep(r.ctx, command)
}

func (r *nativeRuntimeV3) writeArtifact(kind string, payload any) error {
	envelope, err := artifacts.MarshalPayload(kind, "1", payload)
	if err != nil {
		return err
	}
	envelope.JobID = r.claim.Job.ID
	envelope.StepID = r.claim.Step.ID
	return r.svc.repo.WriteArtifact(r.ctx, envelope)
}

func (r *nativeRuntimeV3) writeAcceptedIntentArtifact(
	payload artifacts.IntentArtifact,
) error {
	envelope, err := artifacts.MarshalPayload(artifacts.KindIntent, "1", payload)
	if err != nil {
		return err
	}
	envelope.JobID = r.claim.Job.ID
	envelope.StepID = r.claim.Step.ID
	return r.svc.repo.WriteAcceptedIntentArtifact(r.ctx, envelope)
}

func (r *nativeRuntimeV3) writeEvidence(record evidence.Record) error {
	record.JobID = r.claim.Job.ID
	record.StepID = r.claim.Step.ID
	return r.svc.repo.WriteEvidence(r.ctx, record)
}

func (r *nativeRuntimeV3) readIntentArtifact() (artifacts.IntentArtifact, error) {
	return requireArtifactPayload[artifacts.IntentArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindIntent)
}

func (r *nativeRuntimeV3) readPlanArtifact() (artifacts.PlanArtifact, error) {
	return requireArtifactPayload[artifacts.PlanArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindPlan)
}

func (r *nativeRuntimeV3) readCapabilityAudit() (artifacts.CapabilityAuditArtifact, error) {
	return requireArtifactPayload[artifacts.CapabilityAuditArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindCapabilityAudit)
}

func (r *nativeRuntimeV3) readWorkspaceArtifact() (artifacts.WorkspaceArtifact, error) {
	return requireArtifactPayload[artifacts.WorkspaceArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindWorkspace)
}

func (r *nativeRuntimeV3) readRetrievalArtifact() (artifacts.RetrievalArtifact, error) {
	return requireArtifactPayload[artifacts.RetrievalArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindRetrieval)
}

func (r *nativeRuntimeV3) readWebArtifact() (artifacts.WebEvidenceArtifact, error) {
	return requireArtifactPayload[artifacts.WebEvidenceArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindWebEvidence)
}

func (r *nativeRuntimeV3) readAnalysisArtifact() (artifacts.AnalysisArtifact, error) {
	return requireArtifactPayload[artifacts.AnalysisArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindAnalysis)
}

func (r *nativeRuntimeV3) readResponseDraftArtifact() (artifacts.ResponseDraftArtifact, error) {
	return requireArtifactPayload[artifacts.ResponseDraftArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindResponseDraft)
}

func (r *nativeRuntimeV3) readVerificationArtifact() (artifacts.VerificationArtifact, error) {
	return requireArtifactPayload[artifacts.VerificationArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindVerification)
}

func decodeToolOutput[T any](result toolruntime.Result) (T, error) {
	var zero T
	raw, err := json.Marshal(result.Output)
	if err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var payload T
	if err := decoder.Decode(&payload); err != nil {
		return zero, fmt.Errorf("decode tool output: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return zero, fmt.Errorf("decode tool output: %w", err)
	}
	return payload, nil
}
