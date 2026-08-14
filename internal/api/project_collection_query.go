package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultProjectPageSize = 100
	maxProjectPageSize     = 100
	maxProjectPageOffset   = 1_000_000
	maxProjectRawQuerySize = 4 * 1024
)

type projectCollectionQuery struct {
	Limit  int
	Offset int
}

func decodeProjectCollectionQuery(request *http.Request) (projectCollectionQuery, error) {
	if request == nil || request.URL == nil {
		return projectCollectionQuery{}, fmt.Errorf("project collection request URL is required")
	}
	if len(request.URL.RawQuery) > maxProjectRawQuerySize {
		return projectCollectionQuery{}, fmt.Errorf("project collection query exceeds the %d-byte bound", maxProjectRawQuerySize)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return projectCollectionQuery{}, fmt.Errorf("decode project collection query: %w", err)
	}
	for key, items := range values {
		switch key {
		case "limit", "offset":
		default:
			return projectCollectionQuery{}, fmt.Errorf("project collection has unknown query field %q", key)
		}
		if len(items) != 1 {
			return projectCollectionQuery{}, fmt.Errorf("project collection query field %q must occur exactly once", key)
		}
	}
	query := projectCollectionQuery{Limit: defaultProjectPageSize}
	if raw, present := oneQueryValue(values, "limit"); present {
		limit, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(limit) != raw || limit < 1 || limit > maxProjectPageSize {
			return projectCollectionQuery{}, fmt.Errorf("project collection limit must be one canonical integer between 1 and %d", maxProjectPageSize)
		}
		query.Limit = limit
	}
	if raw, present := oneQueryValue(values, "offset"); present {
		offset, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(offset) != raw || offset < 0 || offset > maxProjectPageOffset {
			return projectCollectionQuery{}, fmt.Errorf("project collection offset must be one canonical integer between 0 and %d", maxProjectPageOffset)
		}
		query.Offset = offset
	}
	return query, nil
}
