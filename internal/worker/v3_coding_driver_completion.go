package worker

import (
	"fmt"
	"sort"
	"strings"
)

func (s *directCodingSession) Complete(
	verification directCodingVerification,
) (string, error) {
	if !verification.Passed {
		return "", fmt.Errorf("cannot complete coding workflow from a failed verification result")
	}
	summary := fmt.Sprintf(
		"Completed deterministic coding workflow: planned_files=%d planned_deletes=%d accepted_mutations=%d %s verification=%s",
		s.plannedFiles,
		s.plannedDeletes,
		len(s.mutationJournal),
		renderDirectCodingMutationJournal(s.mutationJournal),
		"exact-workspace-state",
	)
	return summary, nil
}

func renderDirectCodingMutationJournal(
	entries []directCodingMutationJournalEntry,
) string {
	groups := map[workspaceFileOperation][]string{
		workspaceFileCreate:      {},
		workspaceFileReplace:     {},
		workspaceFileDelete:      {},
	}
	for _, entry := range entries {
		if _, registered := groups[entry.Operation]; !registered {
			continue
		}
		groups[entry.Operation] = append(groups[entry.Operation], entry.Path)
	}
	for operation := range groups {
		sort.Strings(groups[operation])
	}
	return fmt.Sprintf(
		"created=[%s] replaced=[%s] deleted=[%s]",
		strings.Join(groups[workspaceFileCreate], ","),
		strings.Join(groups[workspaceFileReplace], ","),
		strings.Join(groups[workspaceFileDelete], ","),
	)
}
