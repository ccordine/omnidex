package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func deterministicImplementationFailureRoute(
	ledger artifacts.ImplementationLedgerArtifact,
	verification artifacts.ImplementationWorkItem,
	failure string,
) (implementationTriageDecision, bool) {
	normalizedFailure := strings.ToLower(filepath.ToSlash(failure))
	selectedIndex := -1
	selectedOffset := len(normalizedFailure) + 1
	for index, item := range ledger.Items {
		if item.Kind != artifacts.ImplementationWorkKindFile {
			continue
		}
		path := strings.ToLower(filepath.ToSlash(filepath.Clean(item.Path)))
		if path == "." || path == "" {
			continue
		}
		offset := implementationFailurePathOffset(normalizedFailure, path)
		if offset >= 0 && offset < selectedOffset {
			selectedIndex = index
			selectedOffset = offset
		}
	}
	if selectedIndex < 0 {
		return implementationTriageDecision{}, false
	}
	owner := ledger.Items[selectedIndex]
	return implementationTriageDecision{
		RoleID:             "failure_triager",
		VerificationItemID: verification.ID,
		OwnerID:            owner.ID,
		Feedback: fmt.Sprintf(
			"Authoritative command output identifies %s. Correct the exact reported failure in that owned file; preserve its contract and do not weaken tests.",
			owner.Path,
		),
	}, true
}

func implementationFailurePathOffset(failure, path string) int {
	for _, suffix := range []string{":", "(", "\"", "'"} {
		if offset := strings.Index(failure, path+suffix); offset >= 0 {
			return offset
		}
	}
	return -1
}
