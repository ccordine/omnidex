package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const MaxDataSourcePageSize = 100

type DataSourcePageRequest struct {
	Limit  int
	Offset int
}

type DataSourcePage struct {
	Items   []DataSourceRecord
	Offset  int
	HasMore bool
}

type DataSourceChannelPage struct {
	Items   []model.DataSourceChannel
	Offset  int
	HasMore bool
}

func (request DataSourcePageRequest) validate() error {
	if request.Limit < 1 || request.Limit > MaxDataSourcePageSize {
		return fmt.Errorf("data-source page limit must be between 1 and %d", MaxDataSourcePageSize)
	}
	if request.Offset < 0 {
		return fmt.Errorf("data-source page offset must be non-negative")
	}
	return nil
}

func (r *Repository) ListDataSourcesPage(ctx context.Context, request DataSourcePageRequest) (DataSourcePage, error) {
	if err := request.validate(); err != nil {
		return DataSourcePage{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+dataSourceSelectColumns+`
		FROM data_sources
		ORDER BY sort_order ASC
		LIMIT $1 OFFSET $2
	`, request.Limit+1, request.Offset)
	if err != nil {
		return DataSourcePage{}, err
	}
	defer rows.Close()
	items := make([]DataSourceRecord, 0, request.Limit+1)
	for rows.Next() {
		item, err := scanDataSource(rows)
		if err != nil {
			return DataSourcePage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return DataSourcePage{}, err
	}
	hasMore := len(items) > request.Limit
	if hasMore {
		items = items[:request.Limit]
	}
	return DataSourcePage{Items: items, Offset: request.Offset, HasMore: hasMore}, nil
}

func (r *Repository) ListDataSourceChannelsPage(ctx context.Context, dataSourceID string, request DataSourcePageRequest) (DataSourceChannelPage, error) {
	if err := request.validate(); err != nil {
		return DataSourceChannelPage{}, err
	}
	dataSourceID = strings.TrimSpace(dataSourceID)
	if dataSourceID == "" {
		return DataSourceChannelPage{}, fmt.Errorf("data source id is required")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, data_source_id, name, created_at, updated_at
		FROM data_source_channels
		WHERE data_source_id = $1
		ORDER BY updated_at DESC, id ASC
		LIMIT $2 OFFSET $3
	`, dataSourceID, request.Limit+1, request.Offset)
	if err != nil {
		return DataSourceChannelPage{}, err
	}
	defer rows.Close()
	items := make([]model.DataSourceChannel, 0, request.Limit+1)
	for rows.Next() {
		var item model.DataSourceChannel
		if err := rows.Scan(&item.ID, &item.DataSourceID, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return DataSourceChannelPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return DataSourceChannelPage{}, err
	}
	hasMore := len(items) > request.Limit
	if hasMore {
		items = items[:request.Limit]
	}
	return DataSourceChannelPage{Items: items, Offset: request.Offset, HasMore: hasMore}, nil
}
