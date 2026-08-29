package api

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	scrumChannelDefaultPageSize  = 50
	scrumChannelMaxPageSize      = queue.MaxScrumChannelPageSize
	scrumRealtimeChannelPageSize = 25
)

type scrumChannelMessagePage struct {
	Messages     []ScrumChatMessage
	BeforeCursor string
	HasMore      bool
	Total        int64
}

func parseScrumChannelCursor(raw string) (int64, error) {
	if raw != strings.TrimSpace(raw) {
		return 0, fmt.Errorf("Scrum channel cursor is malformed")
	}
	if raw == "" {
		return -1, nil
	}
	const prefix = "scrumchat_v1_"
	if !strings.HasPrefix(raw, prefix) {
		return 0, fmt.Errorf("Scrum channel cursor is malformed")
	}
	ordinal, err := strconv.ParseInt(strings.TrimPrefix(raw, prefix), 36, 64)
	if err != nil || ordinal <= 0 || ordinal > maxScrumChannelCursorOrdinal {
		return 0, fmt.Errorf("Scrum channel cursor is malformed")
	}
	encoded, err := encodeScrumChannelCursor(ordinal, true)
	if err != nil || encoded != raw {
		return 0, fmt.Errorf("Scrum channel cursor is malformed")
	}
	return ordinal, nil
}

const maxScrumChannelCursorOrdinal int64 = 9007199254740991

func encodeScrumChannelCursor(start int64, hasMore bool) (string, error) {
	if start < 0 || start > maxScrumChannelCursorOrdinal || (hasMore && start == 0) {
		return "", fmt.Errorf("Scrum channel cursor ordinal %d is outside exact transport authority", start)
	}
	if !hasMore {
		return "", nil
	}
	return "scrumchat_v1_" + strconv.FormatInt(start, 36), nil
}

func appendScrumChatMessage(existing []ScrumChatMessage, role, content string) ([]ScrumChatMessage, error) {
	messageID, err := queue.NewScrumMessageID(rand.Reader)
	if err != nil {
		return nil, err
	}
	message := ScrumChatMessage{
		ID: messageID, Role: role, Content: content,
	}
	if _, err := scrumChannelMessageAppends([]ScrumChatMessage{message}); err != nil {
		return nil, err
	}
	return append(existing, message), nil
}

func appendScrumChannelEvent(card ScrumCard, role, content string) (ScrumCard, error) {
	messages, err := appendScrumChatMessage(card.PendingChannelMessages, role, content)
	if err != nil {
		return card, err
	}
	card.PendingChannelMessages = messages
	return card, nil
}

func scrumChannelMessageAppends(messages []ScrumChatMessage) ([]queue.ScrumCardMessageAppend, error) {
	appends := make([]queue.ScrumCardMessageAppend, 0, len(messages))
	for index, message := range messages {
		if message.ID == "" {
			return nil, fmt.Errorf("Scrum message %d requires one server-owned identity", index+1)
		}
		appends = append(appends, queue.ScrumCardMessageAppend{
			ID: message.ID, Role: message.Role, Content: message.Content,
			Status: message.Status, OperationID: message.OperationID,
		})
		if _, err := queue.ValidateScrumCardMessageAppend(appends[len(appends)-1]); err != nil {
			return nil, fmt.Errorf("Scrum message %d is outside canonical row authority: %w", index+1, err)
		}
	}
	return appends, nil
}

func pendingScrumMessageContains(messages []ScrumChatMessage, content string) bool {
	for _, message := range messages {
		if message.Content == content {
			return true
		}
	}
	return false
}

func syncRunningJobChannelChat(card ScrumCard, job model.JobDetails) (ScrumCard, bool, error) {
	if err := validateScrumSyncAuthority(card, job); err != nil {
		return card, false, err
	}
	if syncedCtx, ok, err := syncRunningJobStepContexts(card, job); err != nil {
		return card, false, err
	} else if ok {
		return syncedCtx, true, nil
	}
	return card, false, nil
}

func syncRunningJobStepContexts(card ScrumCard, job model.JobDetails) (ScrumCard, bool, error) {
	if err := validateScrumSyncAuthority(card, job); err != nil {
		return card, false, err
	}
	if len(job.Contexts) == 0 {
		return card, false, nil
	}
	syncedID := card.StepContextCursor
	updated := card
	changed := false
	maxID := syncedID
	previousID := int64(0)
	for _, ctxValue := range job.Contexts {
		if ctxValue.ID <= 0 {
			return card, false, fmt.Errorf("Scrum step context cursor authority requires positive context IDs")
		}
		if previousID != 0 && ctxValue.ID <= previousID {
			return card, false, fmt.Errorf("Scrum step contexts must be ordered by strictly increasing typed IDs")
		}
		previousID = ctxValue.ID
		if ctxValue.ID <= syncedID {
			continue
		}
		for _, msg := range stepContextToActivity(ctxValue) {
			before := len(updated.PendingChannelMessages)
			var err error
			updated.PendingChannelMessages, err = appendScrumChatMessage(updated.PendingChannelMessages, msg.Role, msg.Content)
			if err != nil {
				return card, false, err
			}
			changed = changed || len(updated.PendingChannelMessages) != before
		}
		if ctxValue.ID > maxID {
			maxID = ctxValue.ID
		}
	}
	if maxID > syncedID {
		updated.StepContextCursor = maxID
		changed = true
	}
	if !changed {
		return card, false, nil
	}
	return updated, true, nil
}
