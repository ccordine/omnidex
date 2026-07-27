package worker

import (
	"fmt"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const (
	maxImplementationWorkItems    = 16
	maxImplementationItemAttempts = 4
	maxImplementationRepairCycles = 4
	maxImplementationVerifyRuns   = 10
)

var implementationWorkIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

func validateImplementationLedger(ledger artifacts.ImplementationLedgerArtifact) error {
	violations := make([]string, 0)
	if strings.TrimSpace(ledger.ObjectiveID) == "" {
		violations = append(violations, "objective_id is required")
	}
	if strings.TrimSpace(ledger.Objective) == "" {
		violations = append(violations, "objective is required")
	}
	criteria := cleanOrderedStrings(ledger.AcceptanceCriteria)
	constraints := cleanOrderedStrings(ledger.Constraints)
	if len(criteria) == 0 {
		violations = append(violations, "acceptance_criteria are required")
	}
	if len(constraints) != len(ledger.Constraints) {
		violations = append(violations, "constraints must be non-empty and unique")
	}
	if ledger.Revision < 1 {
		violations = append(violations, "revision must be positive")
	}
	if len(ledger.Items) < 2 || len(ledger.Items) > maxImplementationWorkItems {
		violations = append(violations, fmt.Sprintf("ledger must contain 2-%d work items", maxImplementationWorkItems))
	}

	byID := make(map[string]int, len(ledger.Items))
	writers := map[string]string{}
	verificationIDs := make([]string, 0, 1)
	for index, item := range ledger.Items {
		prefix := fmt.Sprintf("items[%d]", index)
		id := strings.TrimSpace(item.ID)
		if !implementationWorkIDPattern.MatchString(id) {
			violations = append(violations, prefix+" has an invalid id")
		} else if _, duplicate := byID[id]; duplicate {
			violations = append(violations, fmt.Sprintf("work item id %q is duplicated", id))
		} else {
			byID[id] = index
		}
		if strings.TrimSpace(item.Responsibility) == "" {
			violations = append(violations, prefix+" responsibility is required")
		}
		if !validImplementationWorkStatus(item.Status) {
			violations = append(violations, prefix+" has invalid status "+fmt.Sprintf("%q", item.Status))
		}
		if item.Attempts < 0 {
			violations = append(violations, prefix+" attempts cannot be negative")
		}
		if item.RepairCycles < 0 || item.RepairCycles > maxImplementationRepairCycles {
			violations = append(violations, prefix+" repair_cycles is outside the allowed range")
		}
		switch item.Kind {
		case artifacts.ImplementationWorkKindFile:
			path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(item.Path)))
			if path == "." || filepath.IsAbs(item.Path) || strings.HasPrefix(path, "../") {
				violations = append(violations, prefix+" file path must remain inside the workspace")
			} else if err := validateV3WritePath(path); err != nil {
				violations = append(violations, prefix+" "+err.Error())
			}
			if keyword := implementationGoKeywordDirectory(path); keyword != "" {
				violations = append(violations, fmt.Sprintf("%s Go source directory %q is a language keyword; choose a package-safe directory name", prefix, keyword))
			}
			if owner, duplicate := writers[path]; duplicate {
				violations = append(violations, fmt.Sprintf("file %q must have one writer; assigned to %q and %q", path, owner, id))
			} else {
				writers[path] = id
			}
			if !validImplementationFileDiscipline(item.Discipline) {
				violations = append(violations, prefix+" has invalid file discipline")
			}
			if item.Command != nil {
				violations = append(violations, prefix+" file work cannot execute a command")
			}
		case artifacts.ImplementationWorkKindVerification:
			verificationIDs = append(verificationIDs, id)
			if strings.TrimSpace(item.Path) != "" {
				violations = append(violations, prefix+" verification work cannot own a file")
			}
			if item.Discipline != artifacts.ImplementationDisciplineVerification {
				violations = append(violations, prefix+" verification discipline is required")
			}
			if item.Command == nil {
				violations = append(violations, prefix+" verification command is required")
			} else if err := validateImplementationVerificationCommand(*item.Command); err != nil {
				violations = append(violations, prefix+" "+err.Error())
			}
		default:
			violations = append(violations, prefix+" has invalid kind "+fmt.Sprintf("%q", item.Kind))
		}
	}
	if len(verificationIDs) != 1 {
		violations = append(violations, "ledger requires exactly one authoritative verification item")
	}

	for _, item := range ledger.Items {
		seenDependencies := map[string]struct{}{}
		for _, dependency := range item.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if dependency == item.ID {
				violations = append(violations, fmt.Sprintf("work item %q cannot depend on itself", item.ID))
			}
			if _, exists := byID[dependency]; !exists {
				violations = append(violations, fmt.Sprintf("work item %q depends on unknown item %q", item.ID, dependency))
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				violations = append(violations, fmt.Sprintf("work item %q repeats dependency %q", item.ID, dependency))
			}
			seenDependencies[dependency] = struct{}{}
		}
	}
	if implementationLedgerHasCycle(ledger, byID) {
		violations = append(violations, "implementation dependency graph contains a cycle")
	}
	if len(verificationIDs) == 1 {
		verification := ledger.Items[byID[verificationIDs[0]]]
		closure := implementationDependencyClosure(verification, ledger, byID)
		for _, item := range ledger.Items {
			if item.Kind == artifacts.ImplementationWorkKindFile {
				if _, covered := closure[item.ID]; !covered {
					violations = append(violations, fmt.Sprintf("verification item must depend on file work item %q", item.ID))
				}
			}
		}
	}
	assignedCriteria := map[string]int{}
	authoritativeCriteria := map[string]struct{}{}
	for _, criterion := range criteria {
		authoritativeCriteria[criterion] = struct{}{}
		if isImplementationGlobalConstraint(criterion) {
			violations = append(violations, fmt.Sprintf("global implementation constraint %q cannot be an acceptance criterion", criterion))
		}
	}
	for _, item := range ledger.Items {
		for _, criterion := range cleanOrderedStrings(item.AcceptanceCriteria) {
			assignedCriteria[criterion]++
			if _, authoritative := authoritativeCriteria[criterion]; !authoritative {
				violations = append(violations, fmt.Sprintf("work item %q acceptance criterion %q is not authoritative", item.ID, criterion))
			}
		}
	}
	for _, criterion := range criteria {
		if assignedCriteria[criterion] == 0 {
			violations = append(violations, fmt.Sprintf("authoritative acceptance criterion %q is not assigned to a work item", criterion))
		} else if assignedCriteria[criterion] > 1 {
			violations = append(violations, fmt.Sprintf("authoritative acceptance criterion %q must have exactly one owner", criterion))
		}
	}
	violations = append(violations, implementationArtifactCoverageViolations(ledger, criteria)...)
	violations = append(violations, implementationSeparationViolations(ledger, append(append([]string(nil), criteria...), constraints...))...)
	violations = append(violations, implementationTopologyViolations(ledger, criteria)...)
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	violations = compactExactImplementationViolations(violations)
	return fmt.Errorf("invalid implementation ledger: %s", strings.Join(violations, "; "))
}

func implementationGoKeywordDirectory(path string) string {
	if strings.ToLower(filepath.Ext(path)) != ".go" {
		return ""
	}
	directory := filepath.ToSlash(filepath.Dir(path))
	if directory == "." {
		return ""
	}
	for _, element := range strings.Split(directory, "/") {
		if token.Lookup(element).IsKeyword() {
			return element
		}
	}
	return ""
}

func validateImplementationVerificationCommand(command artifacts.ImplementationCommand) error {
	if err := validateV3Command(strings.TrimSpace(command.Program), append([]string(nil), command.Args...)); err != nil {
		return err
	}
	if isV3WorkspaceInitializer(command.Program, command.Args) {
		return fmt.Errorf("verification command cannot be a workspace initializer")
	}
	if command.TimeoutSeconds < 0 || command.TimeoutSeconds > int(maxV3CommandLimit.Seconds()) {
		return fmt.Errorf("verification command timeout must be between 0 and %d seconds", int(maxV3CommandLimit.Seconds()))
	}
	return nil
}

func readyImplementationWorkItem(ledger artifacts.ImplementationLedgerArtifact) (int, error) {
	if err := validateImplementationLedger(ledger); err != nil {
		return -1, err
	}
	byID := implementationItemIndexes(ledger)
	for index, item := range ledger.Items {
		if item.Status != artifacts.ImplementationWorkStatusPending {
			continue
		}
		ready := true
		for _, dependency := range item.DependsOn {
			if ledger.Items[byID[dependency]].Status != artifacts.ImplementationWorkStatusCompleted {
				ready = false
				break
			}
		}
		if ready {
			return index, nil
		}
	}
	for _, item := range ledger.Items {
		if item.Status == artifacts.ImplementationWorkStatusFailed {
			return -1, fmt.Errorf("implementation work item %q failed: %s", item.ID, safeLine(item.LastError, "no failure detail"))
		}
		if item.Status != artifacts.ImplementationWorkStatusCompleted {
			return -1, fmt.Errorf("implementation ledger has no ready work item while %q remains %s", item.ID, item.Status)
		}
	}
	return -1, nil
}

func reopenImplementationOwner(ledger *artifacts.ImplementationLedgerArtifact, ownerID, feedback string) error {
	if ledger == nil {
		return fmt.Errorf("implementation ledger is required")
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return fmt.Errorf("implementation owner feedback is required")
	}
	for index := range ledger.Items {
		item := &ledger.Items[index]
		if item.ID != strings.TrimSpace(ownerID) {
			continue
		}
		if item.Kind != artifacts.ImplementationWorkKindFile {
			return fmt.Errorf("implementation failure owner %q is not a file work item", ownerID)
		}
		if item.RepairCycles >= maxImplementationRepairCycles {
			return fmt.Errorf("implementation work item %q exhausted its %d verification-repair cycles: %s", ownerID, maxImplementationRepairCycles, feedback)
		}
		item.Status = artifacts.ImplementationWorkStatusPending
		item.Attempts = 0
		item.RepairCycles++
		item.LastError = trimForBudget(feedback, 6000)
		item.ResultSummary = ""
		ledger.Revision++
		return nil
	}
	return fmt.Errorf("implementation failure owner %q does not exist", ownerID)
}

func validImplementationWorkStatus(status string) bool {
	return containsString([]string{
		artifacts.ImplementationWorkStatusPending,
		artifacts.ImplementationWorkStatusRunning,
		artifacts.ImplementationWorkStatusCompleted,
		artifacts.ImplementationWorkStatusFailed,
	}, status)
}

func validImplementationFileDiscipline(discipline string) bool {
	return containsString([]string{
		artifacts.ImplementationDisciplineBootstrap,
		artifacts.ImplementationDisciplineDomain,
		artifacts.ImplementationDisciplineStorage,
		artifacts.ImplementationDisciplineInterface,
		artifacts.ImplementationDisciplineEntrypoint,
		artifacts.ImplementationDisciplineTest,
		artifacts.ImplementationDisciplineDocumentation,
	}, discipline)
}

func compactExactImplementationViolations(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
