package queue

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

const (
	maxContextSearchQueries = 1
	maxContextSearchRecords = 24
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
	if len(terms) > maxContextSearchQueries {
		return fmt.Errorf("context search exceeds the %d-query bound", maxContextSearchQueries)
	}
	for index, term := range terms {
		if strings.TrimSpace(term) == "" || len(term) > model.MaxFreeFormTurnBytes ||
			!utf8.ValidString(term) || strings.ContainsRune(term, '\x00') {
			return fmt.Errorf(
				"context search query %d must contain 1..%d valid UTF-8 bytes without NUL",
				index, model.MaxFreeFormTurnBytes,
			)
		}
	}
	if limit < 1 || limit > maxContextSearchRecords {
		return fmt.Errorf("context search record limit must be within 1..%d", maxContextSearchRecords)
	}
	return nil
}
