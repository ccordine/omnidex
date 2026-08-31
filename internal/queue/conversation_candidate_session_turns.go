package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type conversationFollowupKind string

const (
	conversationFollowupUserTurn     conversationFollowupKind = "follow_up"
	conversationFollowupRedirect     conversationFollowupKind = "redirect"
	conversationFollowupInterruption conversationFollowupKind = "interruption"
	conversationFollowupCancellation conversationFollowupKind = "cancellation"
)

type conversationFollowup struct {
	operationID LifecycleOperationID
	generation  int64
	phase       int
	kind        conversationFollowupKind
	text        string
	contextText string
	createdAt   time.Time
}

func loadConversationSessionFollowupsTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	jobID int64,
) ([]conversationFollowup, error) {
	rows, err := tx.Query(ctx, `
		SELECT operation_id,generation,phase,kind,text,context_text,created_at
		FROM channel_conversation_followup_events
		WHERE channel_id=$1 AND job_id=$2
		ORDER BY generation DESC,phase DESC,created_at DESC,operation_id DESC
		LIMIT $3
	`, channelID, jobID, MaxChannelSessionTurns+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	followups := make([]conversationFollowup, 0, MaxChannelSessionTurns+1)
	for rows.Next() {
		var followup conversationFollowup
		if err := rows.Scan(
			&followup.operationID,
			&followup.generation,
			&followup.phase,
			&followup.kind,
			&followup.text,
			&followup.contextText,
			&followup.createdAt,
		); err != nil {
			return nil, err
		}
		if err := validateConversationFollowup(followup); err != nil {
			return nil, fmt.Errorf("conversation job %d follow-up: %w", jobID, err)
		}
		followups = append(followups, followup)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(followups) > MaxChannelSessionTurns {
		return nil, fmt.Errorf(
			"conversation job %d exceeds the explicit %d-event follow-up context bound",
			jobID,
			MaxChannelSessionTurns,
		)
	}
	for left, right := 0, len(followups)-1; left < right; left, right = left+1, right-1 {
		followups[left], followups[right] = followups[right], followups[left]
	}
	for index := 1; index < len(followups); index++ {
		if !conversationFollowupOrdered(followups[index-1], followups[index]) {
			return nil, fmt.Errorf(
				"conversation job %d follow-up chronology is not strictly ordered",
				jobID,
			)
		}
	}
	return followups, nil
}

func validateConversationFollowup(followup conversationFollowup) error {
	if _, err := ParseLifecycleOperationID(string(followup.operationID)); err != nil {
		return err
	}
	if followup.generation < 1 || followup.createdAt.IsZero() {
		return fmt.Errorf("follow-up has invalid generation or timestamp authority")
	}
	var expectedContext string
	switch followup.kind {
	case conversationFollowupUserTurn:
		if followup.phase != 1 {
			return fmt.Errorf("follow-up has invalid chronological phase %d", followup.phase)
		}
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, followup.text); err != nil {
			return err
		}
		expectedContext = "\n\nuser follow-up:\n" + followup.text
	case conversationFollowupRedirect:
		if followup.phase != 0 || followup.generation < 2 {
			return fmt.Errorf("redirect has invalid generation or chronological phase")
		}
		if _, _, err := validateSessionReplanFeedback(followup.text); err != nil {
			return err
		}
		expectedContext = "\n\nuser redirect:\n" + followup.text
	case conversationFollowupInterruption:
		if followup.phase != 0 || followup.generation < 2 {
			return fmt.Errorf("interruption has invalid generation or chronological phase")
		}
		if _, _, err := validateInterruptFeedback(followup.text); err != nil {
			return err
		}
		expectedContext = "\n\nuser interruption:\n" + followup.text
	case conversationFollowupCancellation:
		if followup.phase != 2 {
			return fmt.Errorf("cancellation has invalid chronological phase %d", followup.phase)
		}
		if _, err := validateCancelReason(followup.text); err != nil {
			return err
		}
		expectedContext = "\n\nuser cancellation:\n" + followup.text
	default:
		return fmt.Errorf("unregistered kind %q", followup.kind)
	}
	if followup.contextText != expectedContext {
		return fmt.Errorf("context projection differs from exact %s text", followup.kind)
	}
	return nil
}

func conversationFollowupOrdered(left, right conversationFollowup) bool {
	if left.generation != right.generation {
		return left.generation < right.generation
	}
	if left.phase != right.phase {
		return left.phase < right.phase
	}
	if !left.createdAt.Equal(right.createdAt) {
		return left.createdAt.Before(right.createdAt)
	}
	return left.operationID < right.operationID
}

func projectConversationSessionTurns(initial string, followups []conversationFollowup) string {
	if len(followups) == 0 {
		return initial
	}
	var projected strings.Builder
	projected.WriteString(initial)
	for _, followup := range followups {
		projected.WriteString(followup.contextText)
	}
	return projected.String()
}
