package cognitionstate

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func acceptedFactEligibleAfterSourceRevision(
	entry taskstate.Entry,
	revision cognition.WorldRevision,
) (bool, error) {
	var metadata struct {
		SourceKind SourceKind `json:"source_kind"`
	}
	if err := json.Unmarshal(entry.Metadata.Bytes(), &metadata); err != nil {
		return false, fmt.Errorf("%w: decode fact source authority: %v", ErrInvalidReconciliation, err)
	}
	if metadata.SourceKind != SourceAcceptedFact {
		return true, nil
	}
	if entry.Authority != taskstate.AuthorityCode || entry.CreatedBy != taskstate.AuthorityCode || len(entry.Refs) == 0 {
		return false, fmt.Errorf("%w: accepted fact lacks code-owned evidence lineage", ErrInvalidReconciliation)
	}
	prefix := "cognition:episode/" + string(revision.EpisodeID) + "/observation/"
	for index, ref := range entry.Refs {
		if ref.Relation != taskstate.RefEvidence || !strings.HasPrefix(ref.URI, prefix) ||
			len(ref.URI) == len(prefix) {
			return false, fmt.Errorf("%w: accepted fact source %d is not exact episode evidence", ErrInvalidReconciliation, index)
		}
		sourceRevision, err := strconv.ParseUint(ref.Version, 10, 64)
		if err != nil || sourceRevision == 0 || sourceRevision > revision.Number {
			return false, fmt.Errorf("%w: accepted fact source %d has an invalid revision", ErrInvalidReconciliation, index)
		}
		if sourceRevision == revision.Number {
			return false, nil
		}
	}
	return true, nil
}
