package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	defaultScrumTagPageSize = 40
	maxScrumTagQueryBytes   = 256
	maxScrumTagRawQuerySize = 4 * 1024
)

type scrumTagCatalogQuery struct {
	ProjectID int64
	Search    string
	Limit     int
}

func decodeScrumTagCatalogQuery(request *http.Request) (scrumTagCatalogQuery, error) {
	if request == nil || request.URL == nil {
		return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog request URL is required")
	}
	if len(request.URL.RawQuery) > maxScrumTagRawQuerySize {
		return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog query exceeds the %d-byte bound", maxScrumTagRawQuerySize)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return scrumTagCatalogQuery{}, fmt.Errorf("decode Scrum tag catalog query: %w", err)
	}
	for key, items := range values {
		switch key {
		case "project_id", "q", "limit":
		default:
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog has unknown query field %q", key)
		}
		if len(items) != 1 {
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog query field %q must occur exactly once", key)
		}
	}
	rawProjectID, present := oneQueryValue(values, "project_id")
	if !present {
		return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog requires project_id")
	}
	projectID, err := strconv.ParseInt(rawProjectID, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != rawProjectID {
		return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog project_id must be one canonical positive integer")
	}
	query := scrumTagCatalogQuery{ProjectID: projectID, Limit: defaultScrumTagPageSize}
	if raw, present := oneQueryValue(values, "q"); present {
		if !utf8.ValidString(raw) {
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog search must be valid UTF-8")
		}
		if strings.ContainsRune(raw, '\x00') {
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog search must not contain NUL")
		}
		if len(raw) > maxScrumTagQueryBytes {
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog search exceeds the %d-byte bound", maxScrumTagQueryBytes)
		}
		query.Search = raw
	}
	if raw, present := oneQueryValue(values, "limit"); present {
		limit, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(limit) != raw {
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog limit must be one canonical integer")
		}
		if limit < 1 || limit > queue.MaxScrumTagPageSize {
			return scrumTagCatalogQuery{}, fmt.Errorf("Scrum tag catalog limit must be between 1 and %d", queue.MaxScrumTagPageSize)
		}
		query.Limit = limit
	}
	return query, nil
}
