package queue

import (
	"fmt"
	"strings"
)

const (
	maxContextSearchTerms     = 3
	maxContextSearchTermBytes = 256
	maxContextSearchRecords   = 24
)

// ContextSearchRecord is an exact server-owned record returned by a fixed
// retrieval provider. It is not model control data; the worker replaces its
// source identity with a per-call opaque candidate ID before semantic
// relevance selection.
type ContextSearchRecord struct {
	Namespace string
	SourceID  string
	Content   string
}

func validateContextSearchRequest(terms []string, limit int) error {
	if len(terms) > maxContextSearchTerms {
		return fmt.Errorf("context search exceeds the %d-term bound", maxContextSearchTerms)
	}
	seen := make(map[string]struct{}, len(terms))
	for index, term := range terms {
		if term == "" || term != strings.TrimSpace(term) || len(term) > maxContextSearchTermBytes {
			return fmt.Errorf("context search term %d must contain 1..%d trimmed bytes", index, maxContextSearchTermBytes)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("context search term %q is duplicated", term)
		}
		seen[identity] = struct{}{}
	}
	if limit < 1 || limit > maxContextSearchRecords {
		return fmt.Errorf("context search record limit must be within 1..%d", maxContextSearchRecords)
	}
	return nil
}
