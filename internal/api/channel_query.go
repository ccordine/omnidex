package api

import (
	"fmt"
	"net/http"
	"net/url"
)

const maxChannelQueryBytes = 4 * 1024

func validateExactQuery(request *http.Request, allowed ...string) error {
	if request == nil || request.URL == nil {
		return fmt.Errorf("channel request URL is required")
	}
	if len(request.URL.RawQuery) > maxChannelQueryBytes {
		return fmt.Errorf("channel query exceeds the %d-byte bound", maxChannelQueryBytes)
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return fmt.Errorf("decode channel query: %w", err)
	}
	accepted := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		accepted[field] = struct{}{}
	}
	for field, items := range values {
		if _, ok := accepted[field]; !ok {
			return fmt.Errorf("channel query has unknown field %q", field)
		}
		if len(items) != 1 {
			return fmt.Errorf("channel query field %q must occur exactly once", field)
		}
	}
	return nil
}
