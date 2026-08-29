package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxScrumBoardRawQuerySize = 4 * 1024
	maxScrumCardPageOffset    = 1_000_000
	defaultScrumBoardColumn   = string(queue.ScrumCardAssigned)
)

type scrumBoardQuery struct {
	ProjectID  int64
	Column     string
	CardOffset int
}

func decodeScrumBoardQuery(request *http.Request) (scrumBoardQuery, error) {
	if request == nil || request.URL == nil {
		return scrumBoardQuery{}, fmt.Errorf("Scrum board request URL is required")
	}
	if len(request.URL.RawQuery) > maxScrumBoardRawQuerySize {
		return scrumBoardQuery{}, fmt.Errorf(
			"Scrum board query exceeds the %d-byte bound", maxScrumBoardRawQuerySize,
		)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return scrumBoardQuery{}, fmt.Errorf("decode Scrum board query: %w", err)
	}
	for key, items := range values {
		switch key {
		case "project_id", "column", "card_offset":
		default:
			return scrumBoardQuery{}, fmt.Errorf("Scrum board has unknown query field %q", key)
		}
		if len(items) != 1 {
			return scrumBoardQuery{}, fmt.Errorf("Scrum board query field %q must occur exactly once", key)
		}
	}
	rawProjectID, present := oneQueryValue(values, "project_id")
	if !present {
		return scrumBoardQuery{}, fmt.Errorf("Scrum board requires project_id")
	}
	projectID, err := strconv.ParseInt(rawProjectID, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != rawProjectID {
		return scrumBoardQuery{}, fmt.Errorf("Scrum board project_id must be one canonical positive integer")
	}
	query := scrumBoardQuery{ProjectID: projectID, Column: defaultScrumBoardColumn}
	if rawColumn, present := oneQueryValue(values, "column"); present {
		column, err := queue.ParseScrumCardColumn(rawColumn)
		if err != nil {
			return scrumBoardQuery{}, err
		}
		query.Column = string(column)
	}
	if rawOffset, present := oneQueryValue(values, "card_offset"); present {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || strconv.Itoa(offset) != rawOffset {
			return scrumBoardQuery{}, fmt.Errorf("Scrum board card_offset must be one canonical integer")
		}
		if offset < 0 || offset > maxScrumCardPageOffset {
			return scrumBoardQuery{}, fmt.Errorf(
				"Scrum board card_offset must be between 0 and %d", maxScrumCardPageOffset,
			)
		}
		query.CardOffset = offset
	}
	return query, nil
}
