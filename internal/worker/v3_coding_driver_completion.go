package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (s *directCodingSession) Complete(
	verification directCodingVerification,
) (string, error) {
	if !verification.Passed {
		return "", fmt.Errorf("cannot complete coding workflow from a failed verification result")
	}
	if s.cognition == nil {
		return "", fmt.Errorf("coding completion requires persisted task cognition")
	}
	if err := s.cognition.CompleteObjective(verification); err != nil {
		return "", err
	}
	summary := fmt.Sprintf(
		"Completed deterministic coding workflow: planned_files=%d planned_deletes=%d accepted_mutations=%d %s verification=%s",
		s.plannedFiles,
		s.plannedDeletes,
		s.completion.MutationCount,
		renderDirectCodingMutationJournal(s.mutationJournal),
		strings.Join(verification.Commands, " | "),
	)
	if s.deploymentDisposition == assemblyline.ApplicationServiceDeploymentPersistCurrentHost {
		serviceURL, healthURL, err := directCodingDeploymentURLs(s.deployedEndpoint)
		if err != nil || s.deploymentOperationID == "" || len(s.deploymentReceiptSHA) != 64 {
			return "", fmt.Errorf("coding completion requires one canonical persisted deployment outcome")
		}
		summary += fmt.Sprintf(
			" deployment_operation=%s service_url=%s health_url=%s receipt_sha256=%s",
			s.deploymentOperationID, serviceURL, healthURL, s.deploymentReceiptSHA,
		)
	}
	return summary, nil
}

func renderDirectCodingMutationJournal(
	entries []directCodingMutationJournalEntry,
) string {
	groups := map[workspaceFileOperation][]string{
		workspaceDirectoryEnsure: {},
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
		"directories=[%s] created=[%s] replaced=[%s] deleted=[%s]",
		strings.Join(groups[workspaceDirectoryEnsure], ","),
		strings.Join(groups[workspaceFileCreate], ","),
		strings.Join(groups[workspaceFileReplace], ","),
		strings.Join(groups[workspaceFileDelete], ","),
	)
}
