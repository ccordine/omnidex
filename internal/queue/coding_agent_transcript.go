package queue

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func appendCodingAssistantMessagesTx(ctx context.Context, tx pgx.Tx, jobID int64, text string) error {
	parts, err := codingAssistantMessageParts(text)
	if err != nil {
		return err
	}
	for _, part := range parts {
		if _, err := tx.Exec(ctx, `INSERT INTO ai_channel_messages(channel_id,role,content)
			SELECT metadata->>'channel_id','assistant',$2 FROM jobs
			WHERE id=$1 AND pipeline='chat' AND metadata->>'channel_mode'='assistant'`, jobID, part); err != nil {
			return err
		}
	}
	return nil
}

// The journal retains the exact response. Its transcript projection uses bounded
// UTF-8 messages, atomically, so a long answer cannot roll back completed work.
func codingAssistantMessageParts(text string) ([]string, error) {
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) || strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("Codex transcript requires nonblank PostgreSQL-compatible UTF-8")
	}
	if len(text) <= model.MaxChannelContentBytes {
		return []string{text}, nil
	}
	const prefixBudget = 64
	parts := []string{}
	for len(text) > 0 {
		end := min(len(text), model.MaxChannelContentBytes-prefixBudget)
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		parts = append(parts, text[:end])
		text = text[end:]
	}
	for index := range parts {
		// A prefix also makes an all-whitespace continuation a valid message;
		// none of the original response's bytes are discarded.
		parts[index] = fmt.Sprintf("Response (part %d of %d)\n", index+1, len(parts)) + parts[index]
	}
	return parts, nil
}
