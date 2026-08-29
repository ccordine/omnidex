package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

const (
	maxRoleplayConversationCandidateExchanges = 4
	maxRoleplayConversationCandidateBytes     = 160 * 1024
)

type roleplayConversationExchangeAuthority struct {
	UserMessageID  int64
	JobID          int64
	JobInstruction string
	JobResult      string
}

// RoleplayConversationCandidateAuthorities returns complete prior response
// rounds observed by one exact current responder. A character is an observer
// only when the immutable preparation for that prior round lists it as a scene
// participant.
func (r *Repository) RoleplayConversationCandidateAuthorities(
	ctx context.Context,
	job model.Job,
	viewpointID model.RoleplayCharacterID,
) (ConversationCandidateAuthoritySet, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf(
			"roleplay conversation candidates require PostgreSQL and context",
		)
	}
	if err := viewpointID.Validate(); err != nil {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf("roleplay conversation viewpoint: %w", err)
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	if !exists || binding.Mode != model.ChannelModeRoleplay {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf(
			"roleplay conversation candidates require a roleplay channel job",
		)
	}
	if !roleplayConversationResponderExists(binding.RoleplayResponders, viewpointID) {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf(
			"roleplay conversation viewpoint is not a responder in the current round",
		)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireCurrentConversationUserAuthorityTx(ctx, tx, binding, job); err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	exchanges, err := loadVisibleRoleplayConversationExchangesTx(
		ctx, tx, binding, viewpointID,
	)
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	set := ConversationCandidateAuthoritySet{}
	totalBytes := 0
	for _, exchange := range exchanges {
		var user model.ChannelMessage
		if err := tx.QueryRow(ctx, `
			SELECT id,channel_id,role,content,created_at
			FROM ai_channel_messages
			WHERE id=$1 AND channel_id=$2
		`, exchange.UserMessageID, binding.ChannelID).Scan(
			&user.ID, &user.ChannelID, &user.Role, &user.Content, &user.CreatedAt,
		); err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		if user.Role != model.ChannelMessageRoleUser {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"roleplay conversation message %d is not a user authority", user.ID,
			)
		}
		if exchange.JobInstruction != user.Content {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"roleplay conversation job %d instruction differs from user message %d",
				exchange.JobID, user.ID,
			)
		}
		if err := model.ValidateChannelMessage(user.Role, user.Content); err != nil {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"invalid stored conversation candidate %d: %w", user.ID, err,
			)
		}
		userTurn, err := loadConversationRoleplayUserTurn(
			ctx, tx, binding.ChannelID, user.ID, user.Content,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		totalBytes += len(user.Content)
		set.Turns = append(set.Turns, ConversationCandidateTurn{
			MessageID: user.ID, Role: ConversationCandidateUser,
			SpeakerName: userTurn.PersonaName, RoleplayUserTurn: &userTurn, Content: user.Content,
		})
		results, resultBytes, err := loadRoleplayConversationResponseRoundTx(
			ctx, tx, binding.ChannelID, exchange,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		totalBytes += resultBytes
		if totalBytes > maxRoleplayConversationCandidateBytes {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"roleplay conversation candidates exceed the %d-byte bound",
				maxRoleplayConversationCandidateBytes,
			)
		}
		for _, result := range results {
			set.Turns = append(set.Turns, ConversationCandidateTurn{
				MessageID: result.MessageID, Role: ConversationCandidateAssistant,
				PairedUserMessageID: user.ID, SpeakerName: result.SpeakerName, Content: result.Content,
			})
			set.AssistantResults = append(set.AssistantResults, result)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	return set, nil
}

func roleplayConversationResponderExists(
	responders []roleplay.SimulationResponderRoute,
	viewpointID model.RoleplayCharacterID,
) bool {
	for _, responder := range responders {
		if responder.CharacterID == string(viewpointID) {
			return true
		}
	}
	return false
}

func requireCurrentConversationUserAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	binding channelCompletionBinding,
	job model.Job,
) error {
	var role model.ChannelMessageRole
	var content string
	err := tx.QueryRow(ctx, `
		SELECT role,content FROM ai_channel_messages WHERE id=$1 AND channel_id=$2
	`, binding.UserMessageID, binding.ChannelID).Scan(&role, &content)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("current channel user message authority is absent")
	}
	if err != nil {
		return err
	}
	if role != model.ChannelMessageRoleUser || content != job.Instruction {
		return fmt.Errorf("current channel message differs from exact job authority")
	}
	return nil
}

func loadVisibleRoleplayConversationExchangesTx(
	ctx context.Context,
	tx pgx.Tx,
	binding channelCompletionBinding,
	viewpointID model.RoleplayCharacterID,
) ([]roleplayConversationExchangeAuthority, error) {
	rows, err := tx.Query(ctx, `
			SELECT preparation.user_message_id,binding.job_id,job.instruction,job.result
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=preparation.operation_id
		JOIN jobs AS job ON job.id=binding.job_id
		WHERE preparation.channel_id=$1 AND preparation.user_message_id<$2
		  AND preparation.result->'participant_character_ids' ? $3
		  AND job.pipeline='chat' AND job.status='completed'
		ORDER BY preparation.user_message_id DESC
		LIMIT $4
	`, binding.ChannelID, binding.UserMessageID, viewpointID,
		maxRoleplayConversationCandidateExchanges)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	exchanges := make([]roleplayConversationExchangeAuthority, 0, maxRoleplayConversationCandidateExchanges)
	for rows.Next() {
		var exchange roleplayConversationExchangeAuthority
		if err := rows.Scan(
			&exchange.UserMessageID, &exchange.JobID,
			&exchange.JobInstruction, &exchange.JobResult,
		); err != nil {
			return nil, err
		}
		exchanges = append(exchanges, exchange)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(exchanges)-1; left < right; left, right = left+1, right-1 {
		exchanges[left], exchanges[right] = exchanges[right], exchanges[left]
	}
	return exchanges, nil
}

func loadRoleplayConversationResponseRoundTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	exchange roleplayConversationExchangeAuthority,
) ([]ConversationAssistantResultAuthority, int, error) {
	rows, err := tx.Query(ctx, `
		SELECT response.message_id,response.content,response.speaker_name,
		       response.response_position,response.completion_kind
		FROM (
			SELECT message.id AS message_id,message.content,
			       character.name AS speaker_name,completion.response_position,
			       'fictional'::text AS completion_kind
			FROM job_lifecycle_operations AS operation
			JOIN roleplay_turn_completions AS completion
			  ON completion.operation_id=operation.operation_id
			JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
			JOIN roleplay_characters AS character
			  ON character.world_id=completion.world_id
			 AND character.id=completion.viewpoint_character_id
			WHERE operation.job_id=$1 AND operation.kind='complete_step'
			  AND operation.result_job_status='completed'
			  AND operation.result_step_status='completed'
			  AND message.channel_id=$2 AND message.role='assistant'
			UNION ALL
			SELECT message.id,message.content,character.name,0,'research'::text
			FROM roleplay_research_completions AS completion
			JOIN job_lifecycle_operations AS operation
			  ON operation.operation_id=completion.operation_id
			JOIN roleplay_research_turns AS research
			  ON research.preparation_id=completion.preparation_id
			JOIN roleplay_characters AS character
			  ON character.world_id=research.world_id AND character.id=research.character_id
			JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
			WHERE completion.job_id=$1 AND operation.job_id=$1
			  AND operation.kind='complete_step' AND operation.result_job_status='completed'
			  AND operation.result_step_status='completed'
			  AND message.channel_id=$2 AND message.role='assistant'
		) AS response
		ORDER BY response.response_position,response.message_id
	`, exchange.JobID, channelID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	results := make([]ConversationAssistantResultAuthority, 0, roleplay.MaxSceneParticipants)
	contents := make([]string, 0, roleplay.MaxSceneParticipants)
	totalBytes := 0
	completionKind := ""
	for rows.Next() {
		var messageID int64
		var content, speakerName, kind string
		var position int
		if err := rows.Scan(&messageID, &content, &speakerName, &position, &kind); err != nil {
			return nil, 0, err
		}
		if position != len(results) || (completionKind != "" && completionKind != kind) {
			return nil, 0, fmt.Errorf(
				"roleplay conversation job %d has contradictory response order", exchange.JobID,
			)
		}
		completionKind = kind
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, content); err != nil {
			return nil, 0, fmt.Errorf("invalid stored conversation candidate %d: %w", messageID, err)
		}
		if err := model.ValidateChannelMessageSpeaker(model.ChannelMessageRoleAssistant, speakerName); err != nil {
			return nil, 0, fmt.Errorf("invalid roleplay conversation speaker for message %d: %w", messageID, err)
		}
		contents = append(contents, content)
		totalBytes += len(content)
		results = append(results, ConversationAssistantResultAuthority{
			UserMessageID: exchange.UserMessageID, MessageID: messageID, JobID: exchange.JobID,
			SpeakerName: speakerName, Content: content,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(results) == 0 || len(results) > roleplay.MaxSceneParticipants ||
		(completionKind == "research" && len(results) != 1) ||
		strings.Join(contents, "\n\n") != exchange.JobResult {
		return nil, 0, fmt.Errorf(
			"roleplay conversation job %d has no unique exact response round", exchange.JobID,
		)
	}
	return results, totalBytes, nil
}
