package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const maxScrumCardActionQueryBytes = 1024

func decodeScrumCardActionProjectID(request *http.Request, action string) (int64, error) {
	if request == nil || request.URL == nil || action == "" {
		return 0, fmt.Errorf("%s request URL is required", action)
	}
	if len(request.URL.RawQuery) > maxScrumCardActionQueryBytes {
		return 0, fmt.Errorf("%s query exceeds the %d-byte bound", action, maxScrumCardActionQueryBytes)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return 0, fmt.Errorf("decode %s query: %w", action, err)
	}
	for key, items := range values {
		if key != "project_id" {
			return 0, fmt.Errorf("%s has unknown query field %q", action, key)
		}
		if len(items) != 1 {
			return 0, fmt.Errorf("%s query field %q must occur exactly once", action, key)
		}
	}
	raw, present := oneQueryValue(values, "project_id")
	if !present {
		return 0, fmt.Errorf("%s requires project_id", action)
	}
	projectID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != raw {
		return 0, fmt.Errorf("%s project_id must be one canonical positive integer", action)
	}
	return projectID, nil
}
