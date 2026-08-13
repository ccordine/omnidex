package api

import (
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	dataSourceAPIPageSize = 50
	dataSourceUIPageSize  = 20
)

func dataSourcePageRequest(r *http.Request, fallbackLimit int) (queue.DataSourcePageRequest, error) {
	limit, err := exactChannelQueryInteger(r, "limit", fallbackLimit, 1, queue.MaxDataSourcePageSize)
	if err != nil {
		return queue.DataSourcePageRequest{}, err
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		return queue.DataSourcePageRequest{}, err
	}
	return queue.DataSourcePageRequest{Limit: limit, Offset: offset}, nil
}

func fixedDataSourcePageRequest(r *http.Request, offsetKey string, limit int) (queue.DataSourcePageRequest, error) {
	offset, err := exactChannelQueryInteger(r, offsetKey, 0, 0, 1<<30)
	if err != nil {
		return queue.DataSourcePageRequest{}, err
	}
	return queue.DataSourcePageRequest{Limit: limit, Offset: offset}, nil
}

func dataSourceNextOffset(offset, count int, hasMore bool) *int {
	if !hasMore {
		return nil
	}
	next := offset + count
	return &next
}
