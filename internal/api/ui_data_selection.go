package api

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func (s *Server) uiDataSourceSelection(ctx context.Context, page []queue.DataSourceRecord, id string) (queue.DataSourceRecord, bool, error) {
	if id == "" {
		return queue.DataSourceRecord{}, false, nil
	}
	if item, ok := uiFindDataSource(page, id); ok {
		return item, true, nil
	}
	item, err := s.repo.GetDataSource(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return queue.DataSourceRecord{}, false, nil
		}
		return queue.DataSourceRecord{}, false, err
	}
	return item, true, nil
}

func (s *Server) uiDataChannelSelection(ctx context.Context, sourceID string, page []model.DataSourceChannel, id string) (model.DataSourceChannel, bool, error) {
	if id == "" {
		return model.DataSourceChannel{}, false, nil
	}
	if item, ok := findUIDataChannel(page, id); ok {
		return item, true, nil
	}
	item, err := s.repo.GetDataSourceChannel(ctx, sourceID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.DataSourceChannel{}, false, nil
		}
		return model.DataSourceChannel{}, false, err
	}
	return item, true, nil
}
