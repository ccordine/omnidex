package worker

import (
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
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
	return r.svc.repo.CompleteStep(r.ctx, command)
}

func (r *nativeRuntimeV3) writeEvidence(record evidence.Record) error {
	record.JobID = r.claim.Job.ID
	record.StepID = r.claim.Step.ID
	return r.svc.repo.WriteEvidence(r.ctx, r.claim.Authority, record)
}

func (r *nativeRuntimeV3) completeWithEvidence(
	contextKey, output, contextValue string,
	records []evidence.Record,
	roleplayFacts []string,
	roleplayKnowledgeCharacterIDs []model.RoleplayCharacterID,
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
	command.RoleplayFacts = append([]string(nil), roleplayFacts...)
	command.RoleplayKnowledgeCharacterIDs = append(
		[]model.RoleplayCharacterID(nil), roleplayKnowledgeCharacterIDs...,
	)
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
	r.contexts[contextKey] = contextValue
	return nil
}
