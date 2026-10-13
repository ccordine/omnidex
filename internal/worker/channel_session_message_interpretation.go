package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func (consumer *channelSessionMessageConsumer) interpretMessage(
	ctx context.Context, message *queue.ChannelSessionMessage,
) (assemblyline.ConversationObjectiveKind, string, error) {
	if consumer.kind == nil {
		return "", "", fmt.Errorf("session conversation classification is unavailable")
	}
	job := message.AdmittedJob
	job.Instruction = message.Command.Text
	authority, err := newTurnAuthority(job)
	if err != nil {
		return "", "", err
	}
	authority, err = bindObjectiveModelInstruction(authority, assemblyline.ArtifactIdentityProvenance{})
	if err != nil {
		return "", "", err
	}
	input := assemblyline.ConversationObjectiveKindInput{
		ExactInstruction:   authority.ModelInstruction,
		Context:            assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		KnownArtifactPaths: []string{},
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	decision, calls, err := consumer.kind.Classify(ctx, input)
	if err != nil {
		return "", "", err
	}
	if err := validateObjectiveLeafCallCount("active session classification", calls); err != nil {
		return "", "", err
	}
	if err := decision.ValidateFor(input); err != nil {
		return "", "", err
	}
	switch decision.Kind {
	case assemblyline.ObjectiveKindAnswer, assemblyline.ObjectiveKindStory:
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if err := consumer.repository.RequireActiveStepAttempt(ctx, consumer.claim.Authority); err != nil {
			return "", "", err
		}
		authority, err = consumer.projectTaskContext(ctx, authority, message.AdmittedJob)
		if err != nil {
			return "", "", err
		}
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if err := consumer.repository.RequireActiveStepAttempt(ctx, consumer.claim.Authority); err != nil {
			return "", "", err
		}
		result, err := runObjectiveConversationResponse(ctx, authority,
			objectiveTurnResult{Kind: decision.Kind}, consumer.response, "")
		if err != nil {
			return "", "", err
		}
		return decision.Kind, result.Output, nil
	case assemblyline.ObjectiveKindWorkspaceMutation:
		return decision.Kind, "", nil
	default:
		return "", "", fmt.Errorf("session message kind %q has no registered consumer", decision.Kind)
	}
}

// An interpretation failure belongs to this durable message. The accepted
// task continues only after its truthful failure is committed independently.
func (consumer *channelSessionMessageConsumer) failMessage(
	ctx context.Context, message *queue.ChannelSessionMessage, failure error,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errors.Join(failure, err)
	}
	if errors.Is(failure, queue.ErrStaleStepAttempt) ||
		errors.Is(failure, queue.ErrLLMCallTerminalizedByAttempt) ||
		errors.Is(failure, errStepExecutionAuthorityLost) {
		return false, failure
	}
	if err := consumer.repository.RequireActiveStepAttempt(ctx, consumer.claim.Authority); err != nil {
		return false, errors.Join(failure, err)
	}
	_, err := consumer.repository.FailChannelSessionMessage(ctx, queue.FailChannelSessionMessageCommand{
		Authority: consumer.claim.Authority, OperationID: message.Command.OperationID,
		Error: exactStationEvidenceErrorWithinBytes(failure, assemblyline.MaxObjectiveControlFeedbackBytes),
	})
	if err != nil {
		return false, errors.Join(failure, fmt.Errorf("persist failed session message: %w", err))
	}
	consumer.publishResolution(message.Command.OperationID)
	return true, nil
}
