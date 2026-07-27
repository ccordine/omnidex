package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func implementationTopologyViolations(ledger artifacts.ImplementationLedgerArtifact, criteria []string) []string {
	byID := implementationItemIndexes(ledger)
	violations := implementationDisciplineDependencyViolations(ledger, byID)
	violations = append(violations, implementationBootstrapViolations(ledger, byID)...)
	violations = append(violations, implementationCriterionOwnerViolations(ledger, criteria)...)
	violations = append(violations, implementationEntrypointViolations(ledger, byID)...)
	return violations
}

func implementationArtifactCoverageViolations(ledger artifacts.ImplementationLedgerArtifact, criteria []string) []string {
	criteriaText := strings.ToLower(strings.Join(criteria, " "))
	requiresTests := strings.Contains(criteriaText, "test")
	requiresReadme := strings.Contains(criteriaText, "readme")
	hasTestFile := false
	hasReadmeFile := false
	verificationRunsTests := false
	for _, item := range ledger.Items {
		if item.Kind == artifacts.ImplementationWorkKindFile {
			hasTestFile = hasTestFile || item.Discipline == artifacts.ImplementationDisciplineTest
			base := strings.ToLower(filepath.Base(item.Path))
			hasReadmeFile = hasReadmeFile || item.Discipline == artifacts.ImplementationDisciplineDocumentation && strings.HasPrefix(base, "readme")
		}
		if item.Kind == artifacts.ImplementationWorkKindVerification && item.Command != nil {
			verificationRunsTests = verificationRunsTests || isV3TestCommand(item.Command.Program, item.Command.Args)
		}
	}
	violations := []string(nil)
	if requiresTests && !hasTestFile {
		violations = append(violations, "test acceptance criteria require a dedicated test file work item")
	}
	if requiresTests && !verificationRunsTests {
		violations = append(violations, "test acceptance criteria require an authoritative test verification command")
	}
	if requiresReadme && !hasReadmeFile {
		violations = append(violations, "README acceptance criteria require a dedicated README documentation file work item")
	}
	return violations
}

func implementationDisciplineDependencyViolations(ledger artifacts.ImplementationLedgerArtifact, byID map[string]int) []string {
	rank := map[string]int{
		artifacts.ImplementationDisciplineBootstrap:     0,
		artifacts.ImplementationDisciplineDomain:        1,
		artifacts.ImplementationDisciplineStorage:       2,
		artifacts.ImplementationDisciplineInterface:     3,
		artifacts.ImplementationDisciplineEntrypoint:    4,
		artifacts.ImplementationDisciplineTest:          5,
		artifacts.ImplementationDisciplineDocumentation: 5,
		artifacts.ImplementationDisciplineVerification:  6,
	}
	violations := make([]string, 0)
	for _, item := range ledger.Items {
		itemRank, known := rank[item.Discipline]
		if !known {
			continue
		}
		for _, dependencyID := range item.DependsOn {
			dependencyIndex, exists := byID[dependencyID]
			if !exists {
				continue
			}
			dependency := ledger.Items[dependencyIndex]
			if dependencyRank, ok := rank[dependency.Discipline]; ok && dependencyRank > itemRank {
				violations = append(violations, fmt.Sprintf("%s work item %q cannot depend on downstream %s work item %q", item.Discipline, item.ID, dependency.Discipline, dependency.ID))
			}
		}
	}
	return violations
}

func implementationBootstrapViolations(ledger artifacts.ImplementationLedgerArtifact, byID map[string]int) []string {
	verification := implementationVerificationItem(ledger)
	if verification == nil || verification.Command == nil {
		return nil
	}
	manifestPath, relevantExtensions := implementationManifestForCommand(verification.Command.Program)
	if manifestPath == "" {
		return nil
	}
	bootstrapID := ""
	violations := make([]string, 0)
	for _, item := range ledger.Items {
		if item.Kind != artifacts.ImplementationWorkKindFile || filepath.ToSlash(filepath.Clean(item.Path)) != manifestPath {
			continue
		}
		if item.Discipline != artifacts.ImplementationDisciplineBootstrap {
			violations = append(violations, fmt.Sprintf("workspace manifest %q must use bootstrap discipline", manifestPath))
		}
		if bootstrapID != "" {
			violations = append(violations, fmt.Sprintf("verification command requires exactly one %s owner", manifestPath))
		}
		bootstrapID = item.ID
	}
	if bootstrapID == "" {
		return append(violations, fmt.Sprintf("verification command %q requires a bootstrap file work item for %s", verification.Command.Program, manifestPath))
	}
	for _, item := range ledger.Items {
		if item.Kind != artifacts.ImplementationWorkKindFile || item.ID == bootstrapID || !hasImplementationExtension(item.Path, relevantExtensions) {
			continue
		}
		closure := implementationDependencyClosure(item, ledger, byID)
		if _, present := closure[bootstrapID]; !present {
			violations = append(violations, fmt.Sprintf("%s work item %q must depend directly or transitively on bootstrap item %q", filepath.Ext(item.Path), item.ID, bootstrapID))
		}
	}
	return violations
}

func implementationManifestForCommand(program string) (string, []string) {
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "go":
		return "go.mod", []string{".go"}
	case "cargo":
		return "Cargo.toml", []string{".rs"}
	case "npm", "node":
		return "package.json", []string{".js", ".cjs", ".mjs", ".ts", ".tsx"}
	default:
		return "", nil
	}
}

func hasImplementationExtension(path string, allowed []string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	for _, candidate := range allowed {
		if extension == strings.ToLower(candidate) {
			return true
		}
	}
	return false
}

func implementationCriterionOwnerViolations(ledger artifacts.ImplementationLedgerArtifact, criteria []string) []string {
	owners := map[string]artifacts.ImplementationWorkItem{}
	for _, item := range ledger.Items {
		for _, criterion := range cleanOrderedStrings(item.AcceptanceCriteria) {
			owners[criterion] = item
		}
	}
	violations := make([]string, 0)
	interfaceOwner := ""
	for _, criterion := range criteria {
		owner, exists := owners[criterion]
		if !exists {
			continue
		}
		switch {
		case isImplementationVerificationCriterion(criterion):
			if owner.Discipline != artifacts.ImplementationDisciplineVerification {
				violations = append(violations, fmt.Sprintf("verification criterion %q must be owned by verification work", criterion))
			}
		case isImplementationDocumentationCriterion(criterion):
			if owner.Discipline != artifacts.ImplementationDisciplineDocumentation {
				violations = append(violations, fmt.Sprintf("documentation criterion %q must be owned by documentation work", criterion))
			}
		case isImplementationTestCriterion(criterion):
			if owner.Discipline != artifacts.ImplementationDisciplineTest {
				violations = append(violations, fmt.Sprintf("test criterion %q must be owned by test work", criterion))
			}
		case isImplementationStorageCriterion(criterion):
			if owner.Discipline != artifacts.ImplementationDisciplineStorage {
				violations = append(violations, fmt.Sprintf("storage criterion %q must be owned by storage work", criterion))
			}
		case isImplementationInterfaceCriterion(criterion):
			if owner.Discipline != artifacts.ImplementationDisciplineInterface {
				violations = append(violations, fmt.Sprintf("interface criterion %q must be owned by interface work", criterion))
			}
			if interfaceOwner == "" {
				interfaceOwner = owner.ID
			} else if interfaceOwner != owner.ID {
				violations = append(violations, "all observable command, option, and failure criteria must have one interface owner")
			}
		}
	}
	return violations
}

func implementationEntrypointViolations(ledger artifacts.ImplementationLedgerArtifact, byID map[string]int) []string {
	entrypoints := make([]artifacts.ImplementationWorkItem, 0, 1)
	interfaces := make([]string, 0, 1)
	for _, item := range ledger.Items {
		switch item.Discipline {
		case artifacts.ImplementationDisciplineEntrypoint:
			entrypoints = append(entrypoints, item)
		case artifacts.ImplementationDisciplineInterface:
			interfaces = append(interfaces, item.ID)
		}
	}
	violations := make([]string, 0)
	if len(entrypoints) > 1 {
		violations = append(violations, "implementation ledger permits exactly one program entrypoint")
	}
	if requiresImplementationEntrypoint(ledger) && len(entrypoints) != 1 {
		violations = append(violations, "application objective requires exactly one entrypoint work item")
	}
	if len(entrypoints) == 1 && len(interfaces) > 0 {
		closure := implementationDependencyClosure(entrypoints[0], ledger, byID)
		found := false
		for _, interfaceID := range interfaces {
			if _, found = closure[interfaceID]; found {
				break
			}
		}
		if !found {
			violations = append(violations, fmt.Sprintf("entrypoint work item %q must depend on its interface owner", entrypoints[0].ID))
		}
	}
	return violations
}

func implementationVerificationItem(ledger artifacts.ImplementationLedgerArtifact) *artifacts.ImplementationWorkItem {
	for index := range ledger.Items {
		if ledger.Items[index].Kind == artifacts.ImplementationWorkKindVerification {
			return &ledger.Items[index]
		}
	}
	return nil
}

func requiresImplementationEntrypoint(ledger artifacts.ImplementationLedgerArtifact) bool {
	text := strings.ToLower(ledger.Objective + " " + strings.Join(ledger.AcceptanceCriteria, " "))
	for _, marker := range []string{"command-line", " cli ", "application", " app ", "website", "timer", "task tracker"} {
		if strings.Contains(" "+text+" ", marker) {
			return true
		}
	}
	return false
}

func isImplementationVerificationCriterion(value string) bool {
	text := strings.ToLower(value)
	return strings.Contains(text, "pass") && (strings.Contains(text, "test") || strings.Contains(text, "build"))
}

func isImplementationDocumentationCriterion(value string) bool {
	text := strings.ToLower(value)
	return strings.Contains(text, "readme") || strings.Contains(text, "documentation") || strings.Contains(text, "usage examples")
}

func isImplementationTestCriterion(value string) bool {
	text := strings.ToLower(value)
	return strings.Contains(text, "test") && !isImplementationVerificationCriterion(value)
}

func isImplementationStorageCriterion(value string) bool {
	text := strings.ToLower(value)
	if strings.Contains(text, "separat") {
		return false
	}
	for _, marker := range []string{"persist", "stored as json", "save as json", "json storage", "storage format"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isImplementationInterfaceCriterion(value string) bool {
	text := strings.ToLower(value)
	if isImplementationTestCriterion(value) || isImplementationVerificationCriterion(value) {
		return false
	}
	if strings.Contains(text, "separat") && (strings.Contains(text, "domain") || strings.Contains(text, "storage")) {
		return false
	}
	for _, marker := range []string{"command", "--", "option", "invalid", "missing", "nonexistent", "nonzero", "user input"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
