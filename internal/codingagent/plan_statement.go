package codingagent

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// PlanTaskStatement is the exact user-visible authorization projection for one
// proposed task. The full typed workload remains the execution authority.
func PlanTaskStatement(task Task) (string, error) {
	var statement strings.Builder
	fmt.Fprintf(&statement, "%s\nAcceptance: %s", strings.TrimSpace(task.Goal), strings.Join(task.Acceptance, "; "))
	if len(task.ReadFiles) > 0 {
		fmt.Fprintf(&statement, "\nRead: %s", strings.Join(task.ReadFiles, ", "))
	}
	if len(task.WriteFiles) > 0 {
		fmt.Fprintf(&statement, "\nCreate/edit: %s", strings.Join(task.WriteFiles, ", "))
	}
	if len(task.DeleteFiles) > 0 {
		fmt.Fprintf(&statement, "\nDelete: %s", strings.Join(task.DeleteFiles, ", "))
	}
	if len(task.Dependencies) > 0 {
		fmt.Fprintf(&statement, "\nAfter: %s", strings.Join(task.Dependencies, ", "))
	}
	checks := make([]string, len(task.Checks))
	for index, check := range task.Checks {
		checks[index] = check.ID + "=" + strings.Join(check.Argv, " ")
	}
	fmt.Fprintf(&statement, "\nChecks: %s", strings.Join(checks, "; "))
	result := statement.String()
	if len(result) > model.MaxCodingPlanStatementBytes {
		return "", fmt.Errorf("task %s authorization summary exceeds %d bytes", task.ID, model.MaxCodingPlanStatementBytes)
	}
	return result, nil
}
