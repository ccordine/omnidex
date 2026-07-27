package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func implementationSeparationViolations(ledger artifacts.ImplementationLedgerArtifact, criteria []string) []string {
	criteriaText := strings.ToLower(strings.Join(criteria, " "))
	if !strings.Contains(criteriaText, "separat") ||
		!strings.Contains(criteriaText, "domain") ||
		!strings.Contains(criteriaText, "storage") ||
		!strings.Contains(criteriaText, "command") {
		return nil
	}

	conceptOwners := map[string][]string{"domain": {}, "storage": {}, "command": {}}
	entrypoints := make([]artifacts.ImplementationWorkItem, 0, 1)
	for index, item := range ledger.Items {
		if item.Kind != artifacts.ImplementationWorkKindFile {
			continue
		}
		switch item.Discipline {
		case artifacts.ImplementationDisciplineDomain:
			conceptOwners["domain"] = append(conceptOwners["domain"], item.ID)
		case artifacts.ImplementationDisciplineStorage:
			conceptOwners["storage"] = append(conceptOwners["storage"], item.ID)
		case artifacts.ImplementationDisciplineInterface:
			conceptOwners["command"] = append(conceptOwners["command"], item.ID)
		case artifacts.ImplementationDisciplineEntrypoint:
			entrypoints = append(entrypoints, ledger.Items[index])
		}
	}

	violations := make([]string, 0, 6)
	for _, concept := range []string{"domain", "storage", "command"} {
		if len(conceptOwners[concept]) == 0 {
			violations = append(violations, fmt.Sprintf("separation criterion requires a dedicated %s provider file", concept))
		}
	}
	byID := implementationItemIndexes(ledger)
	for _, storageID := range conceptOwners["storage"] {
		closure := implementationDependencyClosure(ledger.Items[byID[storageID]], ledger, byID)
		if !implementationClosureContainsAny(closure, conceptOwners["domain"]) {
			violations = append(violations, fmt.Sprintf("storage work item %q must depend on a domain provider", storageID))
		}
	}
	for _, commandID := range conceptOwners["command"] {
		closure := implementationDependencyClosure(ledger.Items[byID[commandID]], ledger, byID)
		if !implementationClosureContainsAny(closure, conceptOwners["domain"]) || !implementationClosureContainsAny(closure, conceptOwners["storage"]) {
			violations = append(violations, fmt.Sprintf("interface work item %q must depend on domain and storage providers", commandID))
		}
	}

	for _, item := range entrypoints {
		responsibility := strings.ToLower(item.Responsibility)
		for _, overlap := range []string{"command parsing", "task management", "storage logic", "domain logic", "json persistence"} {
			if strings.Contains(responsibility, overlap) {
				violations = append(violations, fmt.Sprintf("entrypoint work item %q overlaps the dedicated %s responsibility", item.ID, overlap))
			}
		}
	}
	return violations
}

func implementationClosureContainsAny(closure map[string]struct{}, candidates []string) bool {
	for _, candidate := range candidates {
		if _, present := closure[candidate]; present {
			return true
		}
	}
	return false
}
