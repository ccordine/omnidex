package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/evidence"
)

const (
	lifecycleOperationCommandSchema = "omnidex.lifecycle-operation-command.v1"
	maxLifecycleOutputBytes         = 4 << 20
	maxObjectiveCompletionEvidence  = 32
	maxObjectiveEvidenceSetBytes    = 128 << 10
)

type lifecycleOperationDescriptor struct {
	ID      LifecycleOperationID
	Kind    LifecycleOperationKind
	SHA256  string
	Payload []byte
}

func describeLifecycleOperation(
	id LifecycleOperationID,
	kind LifecycleOperationKind,
	command any,
) (lifecycleOperationDescriptor, error) {
	if _, err := ParseLifecycleOperationID(string(id)); err != nil {
		return lifecycleOperationDescriptor{}, err
	}
	if !registeredLifecycleOperationKind(kind) {
		return lifecycleOperationDescriptor{}, fmt.Errorf("unregistered lifecycle operation kind %q", kind)
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return lifecycleOperationDescriptor{}, fmt.Errorf("encode %s lifecycle operation: %w", kind, err)
	}
	return lifecycleOperationDescriptor{
		ID: id, Kind: kind, Payload: payload,
		SHA256: lifecycleIdentityDigest(lifecycleOperationCommandSchema, string(kind), string(payload)),
	}, nil
}

func normalizeCompleteStepCommand(command CompleteStepCommand) (CompleteStepCommand, error) {
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return CompleteStepCommand{}, err
	}
	if command.StepID <= 0 {
		return CompleteStepCommand{}, fmt.Errorf("complete-step command requires a positive step ID")
	}
	if command.StepID != command.Authority.StepID {
		return CompleteStepCommand{}, fmt.Errorf("%w: complete-step identity disagrees with attempt authority", ErrStaleStepAttempt)
	}
	if err := validateLifecycleText("step output", command.Output, maxLifecycleOutputBytes, true); err != nil {
		return CompleteStepCommand{}, err
	}
	command.ContextKey = strings.TrimSpace(command.ContextKey)
	if err := validateLifecycleText("step context key", command.ContextKey, maxStepContextKeyBytes, true); err != nil {
		return CompleteStepCommand{}, err
	}
	if err := validateLifecycleText("step context value", command.ContextValue, maxStepContextValueBytes, true); err != nil {
		return CompleteStepCommand{}, err
	}
	if command.ContextKey == "" && command.ContextValue != "" {
		return CompleteStepCommand{}, fmt.Errorf("step context value requires a nonempty context key")
	}
	if (len(command.RoleplayResponses) != 0 || command.RoleplayUserCanon != nil ||
		command.RoleplayUserOngoingAction != nil) &&
		command.ContextKey != "objective_result" {
		return CompleteStepCommand{}, fmt.Errorf(
			"roleplay completion authority requires terminal objective-result completion authority",
		)
	}
	responses, err := normalizeRoleplayResponseCompletions(command.RoleplayResponses)
	if err != nil {
		return CompleteStepCommand{}, err
	}
	command.RoleplayResponses = responses
	userCanon, err := normalizeRoleplayUserCanonCompletion(command.RoleplayUserCanon)
	if err != nil {
		return CompleteStepCommand{}, err
	}
	command.RoleplayUserCanon = userCanon
	if userCanon != nil {
		if len(responses) == 0 {
			return CompleteStepCommand{}, fmt.Errorf(
				"roleplay user canon requires an ordered response round",
			)
		}
		responseFacts := make(map[string]struct{})
		for _, response := range responses {
			for _, fact := range response.Facts {
				responseFacts[fact] = struct{}{}
			}
		}
		for _, fact := range userCanon.Facts {
			if _, duplicate := responseFacts[fact]; duplicate {
				return CompleteStepCommand{}, fmt.Errorf(
					"roleplay canon fact %q is duplicated across user and response sources", fact,
				)
			}
		}
	}
	userAction, err := normalizeRoleplayUserOngoingActionCompletion(
		command.RoleplayUserOngoingAction,
	)
	if err != nil {
		return CompleteStepCommand{}, err
	}
	command.RoleplayUserOngoingAction = userAction
	if userAction != nil {
		if len(responses) == 0 {
			return CompleteStepCommand{}, fmt.Errorf(
				"roleplay user ongoing action requires an ordered response round",
			)
		}
		for _, response := range responses {
			if response.CharacterID == userAction.CharacterID {
				return CompleteStepCommand{}, fmt.Errorf(
					"roleplay user ongoing-action character cannot also be a responder",
				)
			}
		}
	}
	if len(responses) != 0 && command.Output != RenderRoleplayResponseRound(responses) {
		return CompleteStepCommand{}, fmt.Errorf("roleplay completion output differs from its ordered response round")
	}
	return command, nil
}

func normalizeCompleteStepEvidenceCommand(
	command CompleteStepEvidenceCommand,
) (CompleteStepEvidenceCommand, [][]byte, error) {
	normalized, err := normalizeCompleteStepCommand(command.CompleteStepCommand)
	if err != nil {
		return CompleteStepEvidenceCommand{}, nil, err
	}
	if normalized.ContextKey != "objective_result" {
		return CompleteStepEvidenceCommand{}, nil, fmt.Errorf(
			"objective evidence completion requires context key %q", "objective_result",
		)
	}
	if len(command.Evidence) > maxObjectiveCompletionEvidence {
		return CompleteStepEvidenceCommand{}, nil, fmt.Errorf(
			"objective evidence completion exceeds the %d-record limit",
			maxObjectiveCompletionEvidence,
		)
	}
	records := make([]evidence.Record, len(command.Evidence))
	payloads := make([][]byte, len(command.Evidence))
	total := 0
	for index, record := range command.Evidence {
		payload, recordErr := normalizeObjectiveCompletionEvidence(
			record, normalized.Authority.JobID, normalized.StepID,
		)
		if recordErr != nil {
			return CompleteStepEvidenceCommand{}, nil, fmt.Errorf(
				"objective evidence record %d: %w", index, recordErr,
			)
		}
		total += len(payload)
		if total > maxObjectiveEvidenceSetBytes {
			return CompleteStepEvidenceCommand{}, nil, fmt.Errorf(
				"objective evidence set exceeds the %d-byte limit", maxObjectiveEvidenceSetBytes,
			)
		}
		records[index] = record
		payloads[index] = payload
	}
	if records == nil {
		records = make([]evidence.Record, 0)
		payloads = make([][]byte, 0)
	}
	return CompleteStepEvidenceCommand{
		CompleteStepCommand: normalized,
		Evidence:            records,
	}, payloads, nil
}

func normalizeFailStepCommand(command FailStepCommand) (FailStepCommand, error) {
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return FailStepCommand{}, err
	}
	if command.StepID <= 0 {
		return FailStepCommand{}, fmt.Errorf("fail-step command requires a positive step ID")
	}
	if command.StepID != command.Authority.StepID {
		return FailStepCommand{}, fmt.Errorf("%w: fail-step identity disagrees with attempt authority", ErrStaleStepAttempt)
	}
	if err := validateLifecycleText("step failure reason", command.Error, maxLifecycleOutputBytes, false); err != nil {
		return FailStepCommand{}, err
	}
	command.Error = strings.TrimSpace(command.Error)
	return command, nil
}

func normalizeSubmitFeedbackCommand(command SubmitJobFeedbackCommand) (SubmitJobFeedbackCommand, error) {
	if command.JobID <= 0 {
		return SubmitJobFeedbackCommand{}, fmt.Errorf("feedback command requires a positive job ID")
	}
	feedback, _, err := validateLifecycleFeedback(command.Feedback, "job feedback")
	if err != nil {
		return SubmitJobFeedbackCommand{}, err
	}
	command.Feedback = feedback
	return command, nil
}

func normalizeReplanJobCommand(command ReplanJobCommand) (ReplanJobCommand, string, error) {
	if command.JobID <= 0 {
		return ReplanJobCommand{}, "", fmt.Errorf("replan command requires a positive job ID")
	}
	feedback, feedbackSHA, err := validateReplanFeedback(command.Feedback)
	if err != nil {
		return ReplanJobCommand{}, "", err
	}
	command.Feedback = feedback
	return command, feedbackSHA, nil
}

func normalizeCancelJobCommand(command CancelJobCommand) (CancelJobCommand, error) {
	if command.JobID <= 0 {
		return CancelJobCommand{}, fmt.Errorf("cancel command requires a positive job ID")
	}
	reason, err := validateCancelReason(command.Reason)
	if err != nil {
		return CancelJobCommand{}, err
	}
	command.Reason = reason
	return command, nil
}

func validateLifecycleText(name, value string, maximum int, allowEmpty bool) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must be PostgreSQL-compatible UTF-8", name)
	}
	if !allowEmpty && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds the %d-byte limit", name, maximum)
	}
	return nil
}

func registeredLifecycleOperationKind(kind LifecycleOperationKind) bool {
	switch kind {
	case LifecycleCompleteStep, LifecycleFailStep, LifecycleSubmitFeedback, LifecycleReplanJob,
		LifecycleCancelJob:
		return true
	default:
		return false
	}
}
