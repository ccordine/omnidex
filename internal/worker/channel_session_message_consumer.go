package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type channelSessionMessageRepository interface {
	RequireActiveStepAttempt(context.Context, model.StepAttemptAuthority) error
	NextChannelSessionMessage(context.Context, model.StepAttemptAuthority) (*queue.ChannelSessionMessage, error)
	ResolveChannelSessionMessage(context.Context, queue.ResolveChannelSessionMessageCommand) (queue.ChannelSessionMessageResult, error)
	FailChannelSessionMessage(context.Context, queue.FailChannelSessionMessageCommand) (queue.ChannelSessionMessageResult, error)
}

type channelSessionMessageConsumer struct {
	mu           sync.Mutex
	claim        *model.ClaimedStep
	repository   channelSessionMessageRepository
	kind         objectiveKindStation
	response     objectiveConversationStation
	minification contextcompiler.MinificationStation
	resolved     func(model.LifecycleOperationID)
}

func newChannelSessionMessageConsumer(runtime *nativeRuntimeV3) (*channelSessionMessageConsumer, error) {
	if runtime == nil || runtime.svc == nil || runtime.claim == nil {
		return nil, fmt.Errorf("session conversation requires a claimed runtime")
	}
	var origin struct {
		WorkspaceIdentity string `json:"client_workspace_identity"`
	}
	if len(runtime.claim.Job.Metadata) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(runtime.claim.Job.Metadata, &origin); err != nil {
		return nil, fmt.Errorf("decode session conversation origin: %w", err)
	}
	if origin.WorkspaceIdentity == "" {
		return nil, nil
	}
	authority, err := newTurnAuthority(runtime.claim.Job)
	if err != nil {
		return nil, err
	}
	if authority.ChannelMode != model.ChannelModeAssistant {
		return nil, fmt.Errorf("CLI session conversation requires an assistant channel")
	}
	// A response shares this exact running attempt but owns no parent workflow
	// or conversation consumer. Its portable calls cannot recursively drain.
	child := &nativeRuntimeV3{svc: runtime.svc, ctx: runtime.ctx, claim: runtime.claim, action: runtime.action}
	return &channelSessionMessageConsumer{
		claim: runtime.claim, repository: runtime.svc.repo,
		kind:         portableObjectiveKindStation{runtime: child},
		response:     portableObjectiveConversationStation{runtime: child},
		minification: portableObjectiveContextSieveStations{runtime: child},
		resolved: func(operation model.LifecycleOperationID) {
			runtime.svc.emitStepEvent(runtime.claim.Authority, "session_message_resolved", fmt.Sprintf("operation=%s", operation))
		},
	}, nil
}

// consume resolves one durable turn at one boundary. Queue authority remains
// the source of pending work; the mutex only serializes this attempt's calls.
func (consumer *channelSessionMessageConsumer) consume(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("session conversation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if consumer.claim == nil || consumer.repository == nil {
		return false, fmt.Errorf("session conversation requires exact attempt and queue authority")
	}
	if err := consumer.repository.RequireActiveStepAttempt(ctx, consumer.claim.Authority); err != nil {
		return false, err
	}
	message, err := consumer.repository.NextChannelSessionMessage(ctx, consumer.claim.Authority)
	if err != nil || message == nil {
		return false, err
	}
	if message.AdmittedJob.ID != consumer.claim.Job.ID ||
		message.AdmittedJob.CurrentGeneration != consumer.claim.Authority.Generation {
		return false, fmt.Errorf("%w: pending session message differs from the running job generation", queue.ErrStaleStepAttempt)
	}
	kind, response, err := consumer.interpretMessage(ctx, message)
	if err != nil {
		return consumer.failMessage(ctx, message, err)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := consumer.repository.RequireActiveStepAttempt(ctx, consumer.claim.Authority); err != nil {
		return false, err
	}
	result, err := consumer.repository.ResolveChannelSessionMessage(ctx, queue.ResolveChannelSessionMessageCommand{
		Authority: consumer.claim.Authority, OperationID: message.Command.OperationID,
		Kind: kind, Response: response,
	})
	if err != nil {
		return false, err
	}
	consumer.publishResolution(message.Command.OperationID)
	if result.Replanned {
		return true, fmt.Errorf("%w: session message redirected the same job", queue.ErrStaleStepAttempt)
	}
	return true, nil
}

func (consumer *channelSessionMessageConsumer) publishResolution(operation model.LifecycleOperationID) {
	if consumer.resolved != nil {
		consumer.resolved(operation)
	}
}
