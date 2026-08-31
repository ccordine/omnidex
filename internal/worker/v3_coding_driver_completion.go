package worker

import (
	"fmt"
)

func (s *directCodingSession) Complete() string {
	created, replaced, deleted, moved := 0, 0, 0, 0
	for _, entry := range s.mutationJournal {
		switch entry.Operation {
		case workspaceFileCreate:
			created++
		case workspaceFileReplace:
			replaced++
		case workspaceFileDelete:
			deleted++
		case workspaceFileMove:
			moved++
		}
	}
	summary := fmt.Sprintf(
		"Completed deterministic coding workflow: accepted_desired_files=%d accepted_absences=%d filesystem_delta=%d created=%d replaced=%d deleted=%d moved=%d verification=%s",
		s.plannedFiles,
		s.plannedDeletes,
		len(s.mutationJournal),
		created,
		replaced,
		deleted,
		moved,
		"exact-workspace-state",
	)
	return summary
}
