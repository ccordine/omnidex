package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
