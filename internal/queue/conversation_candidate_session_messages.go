package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// ConversationSessionExchangeAuthority binds a reply to its immutable queued
// question, which has an operation identity rather than a channel user ID.
type ConversationSessionExchangeAuthority struct {
	OperationID       model.LifecycleOperationID
	JobID             int64
	Generation        int64
	QuestionText      string
	ResponseMessageID int64
	ResponseText      string
	ResolutionKind    model.ChannelSessionMessageState
}

func (exchange ConversationSessionExchangeAuthority) SourceID() string {
	return "channel-session-message-" + string(exchange.OperationID)
}

func (exchange ConversationSessionExchangeAuthority) ContextRecord() ContextSearchRecord {
	label := "assistant response"
	if exchange.ResolutionKind == model.ChannelSessionMessageFailed {
		label = "assistant failure"
	}
	return ContextSearchRecord{
		Namespace: "conversation_exchange", SourceID: exchange.SourceID(),
		Content: fmt.Sprintf("user message:\n%s\n%s:\n%s", exchange.QuestionText, label, exchange.ResponseText),
	}
}

// This projection recognizes only replies owned by immutable intake and
// resolution receipts. It is shared by adjacency and term-search queries so
// an unrelated assistant message can never be skipped as a queued reply.
const conversationSessionRepliesCTE = `session_replies AS (
	SELECT intake.operation_id,intake.command_payload AS intake,
	       resolution.payload_json AS resolution,
	       reply.id AS response_message_id,reply.channel_id,reply.content
	FROM lifecycle_operation_registry AS intake
	JOIN evidence AS resolution
	  ON resolution.kind='channel_session_message'
	 AND resolution.source_type='lifecycle_operation'
	 AND resolution.message_operation_id=intake.operation_id
	 AND resolution.source_ref=intake.operation_id
	 AND resolution.job_id=intake.message_job_id
	JOIN ai_channel_messages AS reply
	  ON reply.id=resolution.message_reply_id
	 AND reply.channel_id=intake.message_channel_id AND reply.role='assistant'
	WHERE intake.kind='channel_session_message'
	  AND intake.message_channel_id=$1 AND reply.id<=$2
	  AND intake.command_payload->>'operation_id'=intake.operation_id
	  AND intake.command_payload->'command'->>'operation_id'=intake.operation_id
	  AND resolution.payload_json->>'operation_id'=intake.operation_id
	  AND resolution.payload_json->>'kind' IN ('answer','story','failed')
	  AND resolution.payload_json->'job'->>'id'=intake.message_job_id::text
	  AND resolution.payload_json->'job'->>'current_generation'=intake.message_generation::text
	  AND resolution.payload_json->'authority'->>'job_id'=intake.message_job_id::text
	  AND resolution.payload_json->'authority'->>'generation'=intake.message_generation::text
	  AND resolution.payload_json->'authority'->>'step_id'=resolution.step_id::text
)`

func loadConversationSessionExchangeTx(ctx context.Context, tx pgx.Tx, assistant model.ChannelMessage) (ConversationSessionExchangeAuthority, bool, error) {
	var operationID model.LifecycleOperationID
	var intakeRaw, resolutionRaw []byte
	err := tx.QueryRow(ctx, `WITH `+conversationSessionRepliesCTE+`
	 SELECT operation_id,intake,resolution FROM session_replies
	 WHERE channel_id=$1 AND response_message_id=$2`, assistant.ChannelID, assistant.ID).Scan(&operationID, &intakeRaw, &resolutionRaw)
	if err == pgx.ErrNoRows {
		return ConversationSessionExchangeAuthority{}, false, nil
	}
	if err != nil {
		return ConversationSessionExchangeAuthority{}, false, err
	}
	exchange, err := conversationSessionExchangeAuthority(operationID, intakeRaw, resolutionRaw, assistant)
	return exchange, err == nil, err
}

func conversationSessionExchangeAuthority(operationID model.LifecycleOperationID, intakeRaw, resolutionRaw []byte, assistant model.ChannelMessage) (ConversationSessionExchangeAuthority, error) {
	var intake ChannelSessionMessage
	var resolution channelSessionMessageResolution
	if err := json.Unmarshal(intakeRaw, &intake); err != nil {
		return ConversationSessionExchangeAuthority{}, fmt.Errorf("decode queued conversation intake: %w", err)
	}
	if err := json.Unmarshal(resolutionRaw, &resolution); err != nil {
		return ConversationSessionExchangeAuthority{}, fmt.Errorf("decode queued conversation resolution: %w", err)
	}
	if _, err := model.ParseLifecycleOperationID(string(operationID)); err != nil {
		return ConversationSessionExchangeAuthority{}, err
	}
	if intake.OperationID != operationID || intake.Command.OperationID != operationID ||
		intake.Command.ExpectedJobID == nil || intake.Command.ExpectedGeneration == nil ||
		intake.AdmittedJob.ID != *intake.Command.ExpectedJobID || intake.AdmittedJob.CurrentGeneration != *intake.Command.ExpectedGeneration ||
		resolution.OperationID != operationID || resolution.Job.ID != intake.AdmittedJob.ID || resolution.Job.CurrentGeneration != intake.AdmittedJob.CurrentGeneration ||
		resolution.ResponseMessageID == nil || *resolution.ResponseMessageID != assistant.ID || resolution.Authority == nil ||
		resolution.Authority.JobID != intake.AdmittedJob.ID || resolution.Authority.Generation != intake.AdmittedJob.CurrentGeneration ||
		intake.Command.ChannelID != assistant.ChannelID || assistant.Role != model.ChannelMessageRoleAssistant {
		return ConversationSessionExchangeAuthority{}, fmt.Errorf("queued reply %d differs from its exact operation authority", assistant.ID)
	}
	if err := validateStepAttemptAuthority(*resolution.Authority); err != nil {
		return ConversationSessionExchangeAuthority{}, err
	}
	binding, exists, err := channelBindingForJob(intake.AdmittedJob)
	if err != nil {
		return ConversationSessionExchangeAuthority{}, err
	}
	if !exists || binding.Mode != model.ChannelModeAssistant || binding.ChannelID != string(assistant.ChannelID) ||
		binding.ClientCWD != intake.Command.WorkspaceRoot || binding.ClientWorkspaceIdentity != intake.Command.WorkspaceIdentity {
		return ConversationSessionExchangeAuthority{}, fmt.Errorf("queued reply %d has inconsistent channel/workspace ownership", assistant.ID)
	}
	kind := model.ChannelSessionMessageState(resolution.Kind)
	switch kind {
	case model.ChannelSessionMessageAnswered, model.ChannelSessionMessageStory, model.ChannelSessionMessageFailed:
	default:
		return ConversationSessionExchangeAuthority{}, fmt.Errorf("queued reply %d has unregistered resolution %q", assistant.ID, kind)
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, intake.Command.Text); err != nil {
		return ConversationSessionExchangeAuthority{}, err
	}
	if err := model.ValidateChannelMessage(assistant.Role, assistant.Content); err != nil {
		return ConversationSessionExchangeAuthority{}, err
	}
	return ConversationSessionExchangeAuthority{OperationID: operationID, JobID: intake.AdmittedJob.ID,
		Generation: intake.AdmittedJob.CurrentGeneration, QuestionText: intake.Command.Text,
		ResponseMessageID: assistant.ID, ResponseText: assistant.Content, ResolutionKind: kind}, nil
}
