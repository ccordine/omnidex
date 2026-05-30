package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func newDataSourceChannelID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("dsc-%d", time.Now().UTC().UnixNano())
	}
	return "dsc-" + hex.EncodeToString(buf)
}

func (r *Repository) ListDataSourceChannels(ctx context.Context, dataSourceID string) ([]model.DataSourceChannel, error) {
	dataSourceID = strings.TrimSpace(dataSourceID)
	rows, err := r.pool.Query(ctx, `
		SELECT id, data_source_id, name, created_at, updated_at
		FROM data_source_channels
		WHERE data_source_id = $1
		ORDER BY updated_at DESC, id ASC
	`, dataSourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.DataSourceChannel{}
	for rows.Next() {
		var item model.DataSourceChannel
		if err := rows.Scan(&item.ID, &item.DataSourceID, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetDataSourceChannel(ctx context.Context, dataSourceID, channelID string) (model.DataSourceChannel, error) {
	var item model.DataSourceChannel
	err := r.pool.QueryRow(ctx, `
		SELECT id, data_source_id, name, created_at, updated_at
		FROM data_source_channels
		WHERE id = $1 AND data_source_id = $2
	`, strings.TrimSpace(channelID), strings.TrimSpace(dataSourceID)).Scan(
		&item.ID, &item.DataSourceID, &item.Name, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return model.DataSourceChannel{}, err
	}
	return item, nil
}

func (r *Repository) CreateDataSourceChannel(ctx context.Context, dataSourceID, name string) (model.DataSourceChannel, error) {
	dataSourceID = strings.TrimSpace(dataSourceID)
	name = strings.TrimSpace(name)
	if dataSourceID == "" {
		return model.DataSourceChannel{}, fmt.Errorf("data_source_id is required")
	}
	if name == "" {
		name = "New conversation"
	}
	now := time.Now().UTC()
	item := model.DataSourceChannel{
		ID:           newDataSourceChannelID(),
		DataSourceID: dataSourceID,
		Name:         name,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO data_source_channels (id, data_source_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, item.ID, item.DataSourceID, item.Name, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return model.DataSourceChannel{}, err
	}
	return item, nil
}

func (r *Repository) DeleteDataSourceChannel(ctx context.Context, dataSourceID, channelID string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM data_source_channels
		WHERE id = $1 AND data_source_id = $2
	`, strings.TrimSpace(channelID), strings.TrimSpace(dataSourceID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) ListDataSourceChannelMessages(ctx context.Context, channelID string, limit int) ([]model.DataSourceChannelMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	channelID = strings.TrimSpace(channelID)
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel_id, role, content, payload, job_id, created_at
		FROM (
			SELECT id, channel_id, role, content, payload, job_id, created_at
			FROM data_source_channel_messages
			WHERE channel_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		) recent
		ORDER BY created_at ASC, id ASC
	`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.DataSourceChannelMessage{}
	for rows.Next() {
		var item model.DataSourceChannelMessage
		if err := rows.Scan(&item.ID, &item.ChannelID, &item.Role, &item.Content, &item.Payload, &item.JobID, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AddDataSourceChannelMessage(ctx context.Context, channelID, role, content string, payload json.RawMessage, jobID *int64) (model.DataSourceChannelMessage, error) {
	channelID = strings.TrimSpace(channelID)
	role = strings.ToLower(strings.TrimSpace(role))
	content = strings.TrimSpace(content)
	if channelID == "" {
		return model.DataSourceChannelMessage{}, fmt.Errorf("channel id is required")
	}
	if role != "user" && role != "assistant" && role != "system" {
		return model.DataSourceChannelMessage{}, fmt.Errorf("unsupported message role")
	}
	if len(payload) == 0 || !json.Valid(payload) {
		payload = json.RawMessage(`{}`)
	}
	var item model.DataSourceChannelMessage
	err := r.pool.QueryRow(ctx, `
		INSERT INTO data_source_channel_messages (channel_id, role, content, payload, job_id)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		RETURNING id, channel_id, role, content, payload, job_id, created_at
	`, channelID, role, content, string(payload), jobID).Scan(
		&item.ID, &item.ChannelID, &item.Role, &item.Content, &item.Payload, &item.JobID, &item.CreatedAt,
	)
	if err != nil {
		return model.DataSourceChannelMessage{}, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE data_source_channels SET updated_at = NOW() WHERE id = $1`, channelID)
	return item, nil
}

func (r *Repository) DataSourceChannelMessageForJob(ctx context.Context, jobID int64) (model.DataSourceChannelMessage, bool, error) {
	if jobID <= 0 {
		return model.DataSourceChannelMessage{}, false, nil
	}
	var item model.DataSourceChannelMessage
	err := r.pool.QueryRow(ctx, `
		SELECT id, channel_id, role, content, payload, job_id, created_at
		FROM data_source_channel_messages
		WHERE job_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, jobID).Scan(&item.ID, &item.ChannelID, &item.Role, &item.Content, &item.Payload, &item.JobID, &item.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return model.DataSourceChannelMessage{}, false, nil
		}
		return model.DataSourceChannelMessage{}, false, err
	}
	return item, true, nil
}
