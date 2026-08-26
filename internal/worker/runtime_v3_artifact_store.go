package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
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
	if r.svc.completeStep == nil {
		return fmt.Errorf("native completion requires the worker step completer")
	}
	return r.svc.completeStep(r.ctx, command)
}

func (r *nativeRuntimeV3) writeEvidence(record evidence.Record) error {
	_, err := r.writeEvidenceReturningID(record)
	return err
}

func (r *nativeRuntimeV3) writeEvidenceReturningID(record evidence.Record) (int64, error) {
	record.JobID = r.claim.Job.ID
	record.StepID = r.claim.Step.ID
	return r.svc.repo.WriteEvidenceReturningID(r.ctx, r.claim.Authority, record)
}

func (r *nativeRuntimeV3) completeWithEvidence(
	contextKey, output, contextValue string,
	records []evidence.Record,
	roleplayResponses []queue.RoleplayResponseCompletion,
) error {
	output = strings.TrimSpace(output)
	contextValue = strings.TrimSpace(contextValue)
	if contextValue == "" {
		contextValue = output
	}
	command, err := completeClaimedStepCommand(r.claim, output, contextKey, contextValue)
	if err != nil {
		return err
	}
	command.RoleplayResponses = append([]queue.RoleplayResponseCompletion(nil), roleplayResponses...)
	bound := make([]evidence.Record, len(records))
	for index, record := range records {
		record.JobID = r.claim.Job.ID
		record.StepID = r.claim.Step.ID
		bound[index] = record
	}
	if err := r.svc.repo.CompleteStepWithEvidence(r.ctx, queue.CompleteStepEvidenceCommand{
		CompleteStepCommand: command,
		Evidence:            bound,
	}); err != nil {
		return err
	}
	r.svc.notifyJobFinishedForStep(r.ctx, command.StepID)
	r.contexts[contextKey] = contextValue
	return nil
}
