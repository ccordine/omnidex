package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type appliedWorkspaceCompletionError struct {
	err error
}

func (failure *appliedWorkspaceCompletionError) Error() string {
	return fmt.Sprintf("workspace is applied and verified but completion projection failed: %v", failure.err)
}

func (failure *appliedWorkspaceCompletionError) Unwrap() error {
	return failure.err
}

func (r *nativeRuntimeV3) complete(output string) error {
	output = strings.TrimSpace(output)
	command, err := completeClaimedStepCommand(r.claim, output, "")
	if err != nil {
		return err
	}
	if r.svc.completeStep == nil {
		return fmt.Errorf("native completion requires the worker step completer")
	}
	return r.svc.completeStep(r.ctx, command)
}

func (r *nativeRuntimeV3) completeAppliedWorkspace(output string) error {
	if err := r.complete(output); err != nil {
		return &appliedWorkspaceCompletionError{err: err}
	}
	return nil
}

func (r *nativeRuntimeV3) completeWithEvidence(
	contextKey, output string,
	records []evidence.Record,
	roleplayResponses []queue.RoleplayResponseCompletion,
	roleplayUserCanon *queue.RoleplayUserCanonCompletion,
	roleplayUserOngoingAction *queue.RoleplayUserOngoingActionCompletion,
) error {
	output = strings.TrimSpace(output)
	command, err := completeClaimedStepCommand(r.claim, output, contextKey)
	if err != nil {
		return err
	}
	command.RoleplayResponses = append([]queue.RoleplayResponseCompletion(nil), roleplayResponses...)
	if roleplayUserCanon != nil {
		command.RoleplayUserCanon = &queue.RoleplayUserCanonCompletion{
			Facts: append([]string{}, roleplayUserCanon.Facts...),
			KnowledgeCharacterIDs: append(
				[]model.RoleplayCharacterID{}, roleplayUserCanon.KnowledgeCharacterIDs...,
			),
		}
	}
	if roleplayUserOngoingAction != nil {
		copy := *roleplayUserOngoingAction
		if copy.PreviousOngoingAction != nil {
			value := *copy.PreviousOngoingAction
			copy.PreviousOngoingAction = &value
		}
		if copy.OngoingAction != nil {
			value := *copy.OngoingAction
			copy.OngoingAction = &value
		}
		command.RoleplayUserOngoingAction = &copy
	}
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
	return nil
}
