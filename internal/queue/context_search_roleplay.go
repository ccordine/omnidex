package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type searchedRoleplayResponseAuthority struct {
	Position       int    `json:"position"`
	CompletionKind string `json:"completion_kind"`
	MessageID      int64  `json:"message_id"`
	SpeakerName    string `json:"speaker_name"`
	Content        string `json:"content"`
}

type searchedRoleplayExchangeAuthority struct {
	JobID         int64                               `json:"job_id"`
	Instruction   string                              `json:"instruction"`
	JobResult     string                              `json:"job_result"`
	ResultPresent bool                                `json:"result_present"`
	UserMessageID int64                               `json:"user_message_id"`
	UserContent   string                              `json:"user_content"`
	UserTurn      roleplay.UserTurnAuthority          `json:"user_turn"`
	Responses     []searchedRoleplayResponseAuthority `json:"responses"`
}

// SearchRoleplayContextRecords searches only frozen, viewpoint-visible
// authority. Transcript hits are projected as one complete completed exchange;
// a matching user or response fragment can never enter relevance independently.
func (r *Repository) SearchRoleplayContextRecords(
	ctx context.Context,
	worldID string,
	viewpointID model.RoleplayCharacterID,
	sceneID string,
	createdBefore time.Time,
	terms []string,
	limit int,
) ([]ContextSearchRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("roleplay context search requires PostgreSQL and context")
	}
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(sceneID) == "" ||
		createdBefore.IsZero() {
		return nil, fmt.Errorf("roleplay context search requires frozen world, scene, and time authority")
	}
	if err := viewpointID.Validate(); err != nil {
		return nil, err
	}
	if err := validateContextSearchRequest(terms, limit); err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return []ContextSearchRecord{}, nil
	}
	rows, err := r.pool.Query(
		ctx, roleplayContextSearchQuery,
		worldID, viewpointID, sceneID, createdBefore, terms, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search frozen roleplay context: %w", err)
	}
	defer rows.Close()
	records := make([]ContextSearchRecord, 0, limit)
	for rows.Next() {
		var record ContextSearchRecord
		var conversationAuthority []byte
		if err := rows.Scan(
			&record.Namespace, &record.SourceID, &record.Content, &conversationAuthority,
		); err != nil {
			return nil, err
		}
		if strings.TrimSpace(record.SourceID) == "" || strings.TrimSpace(record.Content) == "" {
			return nil, fmt.Errorf("roleplay context search returned invalid exact authority")
		}
		if record.Namespace == "conversation_exchange" {
			expectedSource, expectedContent, err := validateSearchedRoleplayExchange(
				conversationAuthority,
			)
			if err != nil {
				return nil, err
			}
			if record.SourceID != expectedSource || record.Content != expectedContent {
				return nil, fmt.Errorf(
					"searched roleplay exchange differs from its exact completed round authority",
				)
			}
		} else if conversationAuthority != nil {
			return nil, fmt.Errorf("non-conversation roleplay context carried exchange authority")
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func validateSearchedRoleplayExchange(raw []byte) (string, string, error) {
	var authority searchedRoleplayExchangeAuthority
	if len(raw) == 0 {
		return "", "", fmt.Errorf("searched roleplay exchange omitted completed round authority")
	}
	if err := json.Unmarshal(raw, &authority); err != nil {
		return "", "", fmt.Errorf("decode searched roleplay exchange authority: %w", err)
	}
	if authority.JobID < 1 || authority.UserMessageID < 1 || !authority.ResultPresent ||
		authority.Instruction != authority.UserContent {
		return "", "", fmt.Errorf(
			"searched roleplay user message %d differs from its exact completed job authority",
			authority.UserMessageID,
		)
	}
	if err := model.ValidateChannelMessage(
		model.ChannelMessageRoleUser, authority.UserContent,
	); err != nil {
		return "", "", fmt.Errorf(
			"searched roleplay user message %d is invalid: %w", authority.UserMessageID, err,
		)
	}
	if err := authority.UserTurn.Validate(); err != nil ||
		authority.UserTurn.ExactText != authority.UserContent {
		return "", "", fmt.Errorf(
			"searched roleplay user message %d differs from typed user-turn authority",
			authority.UserMessageID,
		)
	}
	if len(authority.Responses) < 1 || len(authority.Responses) > roleplay.MaxSceneParticipants {
		return "", "", fmt.Errorf(
			"searched roleplay job %d has no complete bounded response round", authority.JobID,
		)
	}
	responseContents := make([]string, len(authority.Responses))
	contextParts := make([]string, 0, len(authority.Responses)+1)
	if authority.UserTurn.ContributionKind != roleplay.UserContributionCommand {
		contextParts = append(contextParts, fmt.Sprintf(
			"%s [%s] contribution:\n%s",
			authority.UserTurn.PersonaName,
			authority.UserTurn.ContributionKind,
			authority.UserContent,
		))
	}
	completionKind := authority.Responses[0].CompletionKind
	previousMessageID := authority.UserMessageID
	for index, response := range authority.Responses {
		if response.Position != index || response.MessageID <= previousMessageID ||
			response.CompletionKind != completionKind {
			return "", "", fmt.Errorf(
				"searched roleplay job %d has contradictory response order", authority.JobID,
			)
		}
		if completionKind != "fictional" && completionKind != "research" {
			return "", "", fmt.Errorf(
				"searched roleplay job %d has unsupported completion kind", authority.JobID,
			)
		}
		if err := model.ValidateChannelMessage(
			model.ChannelMessageRoleAssistant, response.Content,
		); err != nil {
			return "", "", fmt.Errorf(
				"searched roleplay assistant message %d is invalid: %w", response.MessageID, err,
			)
		}
		if err := model.ValidateChannelMessageSpeaker(
			model.ChannelMessageRoleAssistant, response.SpeakerName,
		); err != nil {
			return "", "", fmt.Errorf(
				"searched roleplay assistant message %d speaker is invalid: %w",
				response.MessageID, err,
			)
		}
		responseContents[index] = response.Content
		contextParts = append(contextParts, response.SpeakerName+" response:\n"+response.Content)
		previousMessageID = response.MessageID
	}
	if (completionKind == "research" && len(authority.Responses) != 1) ||
		strings.Join(responseContents, "\n\n") != authority.JobResult {
		return "", "", fmt.Errorf(
			"searched roleplay job %d has no unique exact response round", authority.JobID,
		)
	}
	return fmt.Sprintf(
		"channel-message-%d-through-%d",
		authority.UserMessageID, authority.Responses[len(authority.Responses)-1].MessageID,
	), strings.Join(contextParts, "\n"), nil
}
