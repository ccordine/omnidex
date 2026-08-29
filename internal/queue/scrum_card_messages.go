package queue

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	MaxScrumChannelPageSize             = 100
	MaxScrumChannelMessageBytes         = 4 << 20
	MaxScrumChannelPageBytes            = 4 << 20
	maxScrumJSONEscapeFactor            = 6
	MaxScrumChannelEncodedMessagesBytes = MaxScrumChannelPageBytes*maxScrumJSONEscapeFactor +
		MaxScrumChannelPageSize*1024
)

type ScrumCardMessage struct {
	Ordinal         int64
	ID              string
	Role            string
	Content         string
	CreatedAt       time.Time
	SourceCreatedAt string
	TimestampOrigin string
	Status          string
	OperationID     string
}

type ScrumCardMessageAppend struct {
	ID          string
	Role        string
	Content     string
	Status      string
	OperationID string
}

type ScrumChannelPage struct {
	Messages  []ScrumCardMessage
	PlayState string
	Start     int64
	Total     int64
	HasMore   bool
}

type scrumCardMessageQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const scrumCardMessageTailQuery = `
	WITH candidates AS MATERIALIZED (
		SELECT ordinal,message_id,role,content,created_at,source_created_at,
		       timestamp_origin,status,content_bytes,
		       COALESCE(operation_id,'') AS operation_id
		FROM scrum_card_messages
		WHERE project_id=$1 AND card_id=$2 AND ordinal<=$3
		ORDER BY ordinal DESC
		LIMIT $4
	), bounded AS (
		SELECT candidates.*,
		       SUM(content_bytes) OVER (ORDER BY ordinal DESC) AS cumulative_bytes
		FROM candidates
	)
	SELECT ordinal,message_id,role,content,created_at,source_created_at,
	       timestamp_origin,status,operation_id
	FROM bounded
	WHERE cumulative_bytes<=$5
	ORDER BY ordinal DESC
`

func (r *Repository) ScrumChannelPage(
	ctx context.Context,
	projectID int64,
	cardID string,
	limit int,
	before int64,
) (ScrumChannelPage, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 ||
		cardID == "" || cardID != strings.TrimSpace(cardID) {
		return ScrumChannelPage{}, fmt.Errorf("PostgreSQL, context, project, and card are required for Scrum channel paging")
	}
	if limit < 1 || limit > MaxScrumChannelPageSize {
		return ScrumChannelPage{}, fmt.Errorf("Scrum channel limit must be between 1 and %d", MaxScrumChannelPageSize)
	}
	if before != -1 && before <= 0 {
		return ScrumChannelPage{}, fmt.Errorf("Scrum channel cursor must be omitted or one canonical positive ordinal")
	}
	var total, pageEnd int64
	var playState string
	err := r.pool.QueryRow(ctx, `
		SELECT channel_message_count,play_state,
		 CASE WHEN $3<0 THEN channel_message_count ELSE $3 END
		FROM scrum_cards WHERE project_id=$1 AND id=$2
	`, projectID, cardID, before).Scan(&total, &playState, &pageEnd)
	if err != nil {
		return ScrumChannelPage{}, fmt.Errorf("load Scrum channel authority: %w", err)
	}
	if err := validateStoredScrumPlayState(playState); err != nil {
		return ScrumChannelPage{}, fmt.Errorf("Scrum card %q: %w", cardID, err)
	}
	if pageEnd < 0 || pageEnd > total {
		return ScrumChannelPage{}, fmt.Errorf("Scrum card or channel cursor was not found")
	}
	messages, start, err := loadScrumCardMessageTail(
		ctx, r.pool, projectID, cardID, pageEnd, limit, MaxScrumChannelPageBytes,
	)
	if err != nil {
		return ScrumChannelPage{}, err
	}
	return ScrumChannelPage{
		Messages: messages, PlayState: playState,
		Start: start, Total: total, HasMore: start > 0,
	}, nil
}

func loadScrumCardMessageTail(
	ctx context.Context,
	query scrumCardMessageQueryer,
	projectID int64,
	cardID string,
	total int64,
	limit int,
	maxBytes int64,
) ([]ScrumCardMessage, int64, error) {
	if total < 0 || limit < 1 || limit > MaxScrumChannelPageSize || maxBytes < 1 {
		return nil, 0, fmt.Errorf("Scrum message tail request is outside bounded authority")
	}
	rows, err := query.Query(
		ctx, scrumCardMessageTailQuery, projectID, cardID, total, limit, maxBytes,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("load byte-bounded Scrum channel rows: %w", err)
	}
	defer rows.Close()
	reversed := make([]ScrumCardMessage, 0, limit)
	contentBytes := 0
	for rows.Next() {
		var message ScrumCardMessage
		if err := rows.Scan(
			&message.Ordinal, &message.ID, &message.Role, &message.Content,
			&message.CreatedAt, &message.SourceCreatedAt, &message.TimestampOrigin,
			&message.Status, &message.OperationID,
		); err != nil {
			return nil, 0, err
		}
		contentBytes += len(message.Content)
		reversed = append(reversed, message)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if int64(contentBytes) > maxBytes {
		return nil, 0, fmt.Errorf("Scrum message tail exceeds its %d-byte authority", maxBytes)
	}
	messages := make([]ScrumCardMessage, len(reversed))
	for index := range reversed {
		messages[len(reversed)-1-index] = reversed[index]
	}
	start := total
	if len(messages) > 0 {
		start = messages[0].Ordinal - 1
	}
	if total > 0 && len(messages) == 0 {
		return nil, 0, fmt.Errorf("Scrum message tail could not fit one canonical row")
	}
	return messages, start, nil
}

func insertScrumCardMessageTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	cardID string,
	message ScrumCardMessageAppend,
) (ScrumCardMessage, error) {
	message, err := normalizeScrumCardMessageAppend(message)
	if err != nil {
		return ScrumCardMessage{}, err
	}
	var stored ScrumCardMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO scrum_card_messages(
		 project_id,card_id,message_id,role,content,status,operation_id
		) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''))
		RETURNING ordinal,message_id,role,content,created_at,source_created_at,
		          timestamp_origin,status,COALESCE(operation_id,'')
	`, projectID, cardID, message.ID, message.Role, message.Content, message.Status, message.OperationID,
	).Scan(
		&stored.Ordinal, &stored.ID, &stored.Role, &stored.Content,
		&stored.CreatedAt, &stored.SourceCreatedAt, &stored.TimestampOrigin,
		&stored.Status, &stored.OperationID,
	)
	if err != nil {
		return ScrumCardMessage{}, fmt.Errorf("append Scrum card message %q: %w", message.ID, err)
	}
	return stored, nil
}

func normalizeScrumCardMessageAppend(message ScrumCardMessageAppend) (ScrumCardMessageAppend, error) {
	for name, value := range map[string]string{
		"ID": message.ID, "role": message.Role, "status": message.Status,
		"operation ID": message.OperationID,
	} {
		if value != strings.TrimSpace(value) {
			return ScrumCardMessageAppend{}, fmt.Errorf("Scrum message %s is not canonical", name)
		}
	}
	if !validScrumMessageID(message.ID) {
		return ScrumCardMessageAppend{}, fmt.Errorf("Scrum message ID %q is not canonical", message.ID)
	}
	switch message.Role {
	case "user", "assistant", "system", "error", "tool", "thinking", "status":
	default:
		return ScrumCardMessageAppend{}, fmt.Errorf("Scrum message role %q is not registered", message.Role)
	}
	if !utf8.ValidString(message.Content) || strings.ContainsRune(message.Content, '\x00') {
		return ScrumCardMessageAppend{}, fmt.Errorf("Scrum message content must be valid UTF-8 without NUL")
	}
	if message.Content == "" || len(message.Content) > MaxScrumChannelMessageBytes {
		return ScrumCardMessageAppend{}, fmt.Errorf(
			"Scrum message content must contain 1..%d bytes", MaxScrumChannelMessageBytes,
		)
	}
	switch message.Status {
	case "", "running", "completed", "failed":
	default:
		return ScrumCardMessageAppend{}, fmt.Errorf("Scrum message status %q is not registered", message.Status)
	}
	if message.OperationID != "" {
		if _, err := ParseLifecycleOperationID(message.OperationID); err != nil {
			return ScrumCardMessageAppend{}, err
		}
	}
	return message, nil
}

// ValidateScrumCardMessageAppend exposes the canonical row validator without
// permitting callers to derive or rewrite any message field.
func ValidateScrumCardMessageAppend(message ScrumCardMessageAppend) (ScrumCardMessageAppend, error) {
	return normalizeScrumCardMessageAppend(message)
}

func validScrumMessageID(value string) bool {
	if len(value) < 1 || len(value) > 256 {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'A' && character <= 'Z') ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			(index > 0 && strings.ContainsRune("_.:-", rune(character))) {
			continue
		}
		return false
	}
	return true
}
