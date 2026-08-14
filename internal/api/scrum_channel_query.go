package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const maxScrumChannelRawQuerySize = 4 * 1024

type scrumChannelQuery struct {
	ProjectID int64
	Limit     int
	Before    string
}

func decodeScrumChannelQuery(request *http.Request) (scrumChannelQuery, error) {
	if request == nil || request.URL == nil {
		return scrumChannelQuery{}, fmt.Errorf("Scrum channel request URL is required")
	}
	if len(request.URL.RawQuery) > maxScrumChannelRawQuerySize {
		return scrumChannelQuery{}, fmt.Errorf("Scrum channel query exceeds the %d-byte bound", maxScrumChannelRawQuerySize)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return scrumChannelQuery{}, fmt.Errorf("decode Scrum channel query: %w", err)
	}
	for key, items := range values {
		switch key {
		case "project_id", "limit", "before":
		default:
			return scrumChannelQuery{}, fmt.Errorf("Scrum channel has unknown query field %q", key)
		}
		if len(items) != 1 {
			return scrumChannelQuery{}, fmt.Errorf("Scrum channel query field %q must occur exactly once", key)
		}
	}
	rawProject, present := oneQueryValue(values, "project_id")
	if !present {
		return scrumChannelQuery{}, fmt.Errorf("Scrum channel requires project_id")
	}
	projectID, err := strconv.ParseInt(rawProject, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != rawProject {
		return scrumChannelQuery{}, fmt.Errorf("Scrum channel project_id must be one canonical positive integer")
	}
	query := scrumChannelQuery{ProjectID: projectID, Limit: scrumChannelDefaultPageSize}
	if raw, present := oneQueryValue(values, "limit"); present {
		limit, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(limit) != raw || limit < 1 || limit > scrumChannelMaxPageSize {
			return scrumChannelQuery{}, fmt.Errorf("Scrum channel limit must be one canonical integer between 1 and %d", scrumChannelMaxPageSize)
		}
		query.Limit = limit
	}
	if raw, present := oneQueryValue(values, "before"); present {
		if _, err := parseScrumChannelCursor(raw); err != nil || raw == "" {
			return scrumChannelQuery{}, fmt.Errorf("Scrum channel before cursor is malformed")
		}
		query.Before = raw
	}
	return query, nil
}
