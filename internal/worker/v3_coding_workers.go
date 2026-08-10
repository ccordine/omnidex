package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingWorkerRuntime(session *directCodingSession) typedWorkerRuntime {
	if session == nil || session.runtime == nil || session.runtime.svc == nil || session.runtime.claim == nil {
		return typedWorkerRuntime{}
	}
	return typedWorkerRuntime{
		Context:        session.runtime.ctx,
		MaxAttempts:    maxTypedWorkerAttempts,
		MaxConcurrency: session.runtime.svc.fragmentConcurrency,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if err := rejectOfflineExperimentJob(job.Kind); err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompt, responseSchema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			contextProjectionID, err := prepareRepositoryShadowContext(session, job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			session.runtime.svc.emitStepEvent(
				session.runtime.claim.Authority,
				"coding_portable_dispatched",
				fmt.Sprintf("kind=%s work=%s payload=%dB model=%s", job.Kind, job.ID[:12], len(job.Payload), safeEventToken(model, "unknown")),
			)
			raw, err := session.runtime.svc.llmGeneratePortableWithSchemaTrace(
				session.runtime.ctx,
				session.runtime.claim.Authority,
				job,
				portableModelScope(responseSchema),
				model,
				prompt,
				responseSchema,
				contextProjectionID,
			)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
		Emit: func(event typedWorkerEvent) {
			session.runtime.svc.emitStepEvent(
				session.runtime.claim.Authority,
				"coding_worker_"+string(event.State),
				renderDirectCodingWorkerEvent(event),
			)
		},
	}
}

func rejectOfflineExperimentJob(kind assemblyline.WorkKind) error {
	switch kind {
	case assemblyline.WorkRequirementBriefing,
		assemblyline.WorkRequirementAdvisory,
		assemblyline.WorkRequirementSynthesis,
		assemblyline.WorkRequirementFinalAdvisory,
		assemblyline.WorkRequirementFinalSynthesis,
		assemblyline.WorkRetrievalBriefing,
		assemblyline.WorkRetrievalAdvisory,
		assemblyline.WorkRetrievalSynthesis:
		return fmt.Errorf("work kind %q belongs to an offline advisory experiment and is forbidden in production", kind)
	default:
		return nil
	}
}

func portableModelScope(responseSchema map[string]any) string {
	if responseSchema == nil {
		return "portable_fragment_worker"
	}
	return "portable_semantic_worker"
}

func (s *directCodingSession) workerModel(skillID, roleID string) (string, error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.claim == nil {
		return "", fmt.Errorf("direct coding worker model routing is unavailable")
	}
	modelName := s.runtime.svc.v3SpecialistModel(s.runtime.claim.Job, s.runtime.routing, skillID, roleID, "")
	return requireDirectCodingModel(roleID, modelName)
}

func renderDirectCodingWorkerEvent(event typedWorkerEvent) string {
	parts := []string{
		"kind=" + safeLine(string(event.Kind), "unknown"),
		"subject=" + safeEventToken(event.Subject, "unknown"),
		"model=" + safeEventToken(event.Model, "unknown"),
	}
	if event.Attempt > 0 || event.MaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt=%d/%d", event.Attempt, event.MaxAttempts))
	}
	if event.PromptBytes > 0 {
		parts = append(parts, fmt.Sprintf(
			"context=prompt:%dB,capabilities:%dB,current:%dB,correction:%dB",
			event.PromptBytes, event.CapabilityBytes, event.CurrentBytes, event.CorrectionBytes,
		))
	}
	if detail := strings.TrimSpace(event.Detail); detail != "" {
		parts = append(parts, "error="+safeLine(detail, "unknown"))
	}
	return strings.Join(parts, " ")
}

func safeEventToken(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return strings.NewReplacer(" ", "_", "\t", "_", "\n", "_").Replace(value)
}
