package api

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	scrumChannelDefaultPageSize  = 50
	scrumChannelMaxPageSize      = 100
	scrumRealtimeChannelPageSize = 25
)

type scrumChannelMessagePage struct {
	Messages     []ScrumChatMessage
	BeforeCursor string
	HasMore      bool
	Total        int
}

func parseScrumChannelPageLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return scrumChannelDefaultPageSize, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 || limit > scrumChannelMaxPageSize {
		return 0, fmt.Errorf("channel limit must be between 1 and %d", scrumChannelMaxPageSize)
	}
	return limit, nil
}

func scrumChannelMessagePageFor(card ScrumCard, limit int, before string) (scrumChannelMessagePage, error) {
	if limit <= 0 || limit > scrumChannelMaxPageSize {
		return scrumChannelMessagePage{}, fmt.Errorf("channel limit must be between 1 and %d", scrumChannelMaxPageSize)
	}
	messages, err := displayScrumChannelMessages(card)
	if err != nil {
		return scrumChannelMessagePage{}, err
	}
	end := len(messages)
	before = strings.TrimSpace(before)
	if before != "" {
		found := false
		for index, message := range messages {
			if scrumChatMessageID(message) == before {
				end = index
				found = true
				break
			}
		}
		if !found {
			return scrumChannelMessagePage{}, fmt.Errorf("channel cursor %q was not found", before)
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := append([]ScrumChatMessage(nil), messages[start:end]...)
	beforeCursor := ""
	if len(page) > 0 {
		beforeCursor = scrumChatMessageID(page[0])
	}
	return scrumChannelMessagePage{
		Messages:     page,
		BeforeCursor: beforeCursor,
		HasMore:      start > 0,
		Total:        len(messages),
	}, nil
}

func scrumChannelBusy(card ScrumCard) bool {
	switch strings.TrimSpace(card.PlayState) {
	case scrumPlayRunning, scrumPlayQueued:
		return true
	default:
		return false
	}
}

func scrumCardChannelPayload(card ScrumCard, limit int) ScrumCard {
	page, err := scrumChannelMessagePageFor(card, limit, "")
	if err != nil {
		panic(fmt.Sprintf("build bounded scrum channel payload: %v", err))
	}
	card.Chat = page.Messages
	card.ChatCount = page.Total
	card.ConsoleLog = ""
	return card
}

func appendScrumChatMessage(existing []ScrumChatMessage, role, content string) []ScrumChatMessage {
	content = sanitizeScrumChannelText(content)
	if strings.TrimSpace(content) == "" {
		return existing
	}
	role = normalizeScrumChannelRole(role)
	return append(existing, ScrumChatMessage{
		ID:        newScrumChatMessageID(role, content),
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func newScrumChatMessageID(role, content string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s\n%d\n%s", role, time.Now().UTC().UnixNano(), content)))
	return "chatmsg_" + hex.EncodeToString(sum[:])[:16]
}

func scrumChatMessageID(msg ScrumChatMessage) string {
	if strings.TrimSpace(msg.ID) != "" {
		return strings.TrimSpace(msg.ID)
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s\n%s\n%s", msg.Role, msg.CreatedAt, msg.Content)))
	return "chatmsg_" + hex.EncodeToString(sum[:])[:16]
}

func normalizeScrumChannelRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "user", "assistant", "system", "error", "tool", "thinking", "status":
		return role
	default:
		return ""
	}
}

func appendScrumChannelEvent(card ScrumCard, role, content string) ScrumCard {
	card.Chat = appendScrumChatMessage(card.Chat, role, content)
	card.ConsoleLog = appendScrumConsole(card.ConsoleLog, content)
	return card
}

func sanitizeScrumChannelText(s string) string {
	return queue.SanitizeUTF8Text(s)
}

func sanitizeScrumChannelBytes(b []byte) []byte {
	return queue.SanitizeUTF8Bytes(b)
}

func truncateScrumChannelText(s string, maxPrefixBytes int, suffix string) string {
	return queue.TruncateUTF8Text(s, maxPrefixBytes, suffix)
}

func syncRunningJobChannelChat(card ScrumCard, job model.JobDetails) (ScrumCard, bool, error) {
	if err := validateScrumSyncAuthority(card, job); err != nil {
		return card, false, err
	}
	output, err := collectScrumAgentOutput(job)
	if err != nil {
		return card, false, err
	}
	syncedLen := card.AgentStreamChatCursor
	if syncedLen > int64(len(output)) {
		return card, false, fmt.Errorf("Scrum chat cursor %d exceeds exact job output bytes %d", syncedLen, len(output))
	}
	updated := card
	changed := false

	if output != "" && syncedLen < int64(len(output)) {
		delta := output[int(syncedLen):]
		if delta != "" {
			parsed, err := appendParsedAgentStreamLines(updated.Chat, delta)
			if err != nil {
				return card, false, err
			}
			updated.Chat = parsed
			updated.AgentStreamChatCursor = int64(len(output))
			changed = true
		}
	}

	if syncedCtx, ok, err := syncRunningJobStepContexts(updated, job); err != nil {
		return card, false, err
	} else if ok {
		updated = syncedCtx
		changed = true
	}
	if !changed {
		return card, false, nil
	}
	return updated, true, nil
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
			before := len(updated.Chat)
			updated.Chat = appendScrumChatMessage(updated.Chat, msg.Role, msg.Content)
			changed = changed || len(updated.Chat) != before
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

func hydrateCardChannelChat(card ScrumCard) ScrumCard {
	if len(card.Chat) > 0 {
		return card
	}
	displayLog := strings.TrimSpace(card.ConsoleLog)
	if displayLog == "" {
		return card
	}
	updated := card
	for _, block := range splitConsoleLogBlocks(displayLog) {
		role := "system"
		if strings.HasPrefix(strings.ToLower(block), "agent stream:") || strings.HasPrefix(strings.ToLower(block), "agent output:") {
			role = "assistant"
		}
		updated.Chat = appendScrumChatMessage(updated.Chat, role, block)
	}
	return updated
}

func splitConsoleLogBlocks(displayLog string) []string {
	lines := strings.Split(displayLog, "\n")
	blocks := make([]string, 0)
	current := make([]string, 0)
	flush := func() {
		if len(current) == 0 {
			return
		}
		block := strings.TrimSpace(strings.Join(current, "\n"))
		if block != "" {
			blocks = append(blocks, block)
		}
		current = current[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}
