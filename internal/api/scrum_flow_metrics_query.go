package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultScrumFlowMetricsPageSize = 25
	maxScrumFlowMetricsPageSize     = 100
	maxScrumFlowMetricsPageOffset   = 1_000_000
	maxScrumFlowMetricsRawQuerySize = 4 * 1024
)

type scrumFlowMetricsQuery struct {
	ProjectID int64
	Limit     int
	Offset    int
}

func decodeScrumFlowMetricsQuery(request *http.Request) (scrumFlowMetricsQuery, error) {
	if request == nil || request.URL == nil {
		return scrumFlowMetricsQuery{}, fmt.Errorf("Scrum flow metrics request URL is required")
	}
	if len(request.URL.RawQuery) > maxScrumFlowMetricsRawQuerySize {
		return scrumFlowMetricsQuery{}, fmt.Errorf(
			"Scrum flow metrics query exceeds the %d-byte bound",
			maxScrumFlowMetricsRawQuerySize,
		)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return scrumFlowMetricsQuery{}, fmt.Errorf("decode Scrum flow metrics query: %w", err)
	}
	for key, items := range values {
		switch key {
		case "project_id", "limit", "offset":
		default:
			return scrumFlowMetricsQuery{}, fmt.Errorf("Scrum flow metrics has unknown query field %q", key)
		}
		if len(items) != 1 {
			return scrumFlowMetricsQuery{}, fmt.Errorf(
				"Scrum flow metrics query field %q must occur exactly once",
				key,
			)
		}
	}
	rawProjectID, present := oneQueryValue(values, "project_id")
	if !present {
		return scrumFlowMetricsQuery{}, fmt.Errorf("Scrum flow metrics requires project_id")
	}
	projectID, err := strconv.ParseInt(rawProjectID, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != rawProjectID {
		return scrumFlowMetricsQuery{}, fmt.Errorf(
			"Scrum flow metrics project_id must be one canonical positive integer",
		)
	}
	query := scrumFlowMetricsQuery{
		ProjectID: projectID,
		Limit:     defaultScrumFlowMetricsPageSize,
	}
	if rawLimit, present := oneQueryValue(values, "limit"); present {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || strconv.Itoa(limit) != rawLimit || limit < 1 || limit > maxScrumFlowMetricsPageSize {
			return scrumFlowMetricsQuery{}, fmt.Errorf(
				"Scrum flow metrics limit must be one canonical integer between 1 and %d",
				maxScrumFlowMetricsPageSize,
			)
		}
		query.Limit = limit
	}
	if rawOffset, present := oneQueryValue(values, "offset"); present {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || strconv.Itoa(offset) != rawOffset || offset < 0 || offset > maxScrumFlowMetricsPageOffset {
			return scrumFlowMetricsQuery{}, fmt.Errorf(
				"Scrum flow metrics offset must be one canonical integer between 0 and %d",
				maxScrumFlowMetricsPageOffset,
			)
		}
		query.Offset = offset
	}
	return query, nil
}
