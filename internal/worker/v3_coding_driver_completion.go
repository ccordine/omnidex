package worker

import (
	"fmt"
)

func (s *directCodingSession) Complete() (string, error) {
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
		"Completed deterministic coding workflow: planned_files=%d planned_deletes=%d accepted_mutations=%d created=%d replaced=%d deleted=%d moved=%d verification=%s",
		s.plannedFiles,
		s.plannedDeletes,
		len(s.mutationJournal),
		created,
		replaced,
		deleted,
		moved,
		"exact-workspace-state",
	)
	return summary, nil
}
