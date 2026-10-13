package codingagent

import (
	"fmt"
	"strings"
)

func (workload Workload) Validate() error {
	if len(workload.Questions) > 0 {
		return fmt.Errorf("a frozen workload cannot contain unanswered planning questions")
	}
	if len(workload.Tasks) == 0 {
		return fmt.Errorf("agent plan contains no tasks")
	}
	known := map[string]bool{}
	writers := map[string]string{}
	for _, task := range workload.Tasks {
		if task.ID == "" || known[task.ID] || strings.TrimSpace(task.Goal) == "" || len(task.Acceptance) == 0 || len(task.WriteFiles) == 0 {
			return fmt.Errorf("agent task requires one unique identity, goal, acceptance and write scope")
		}
		if err := validateTextList(task.Acceptance, "acceptance"); err != nil {
			return err
		}
		for _, dependency := range task.Dependencies {
			if !known[dependency] {
				return fmt.Errorf("task %s has unknown or unordered dependency %s", task.ID, dependency)
			}
		}
		for _, file := range append(append([]string{}, task.ReadFiles...), task.WriteFiles...) {
			if err := ValidatePath(file); err != nil {
				return err
			}
		}
		for _, file := range task.WriteFiles {
			if writer := writers[file]; writer != "" && !contains(task.Dependencies, writer) {
				return fmt.Errorf("task %s shares writable file %s without depending on %s", task.ID, file, writer)
			}
			writers[file] = task.ID
		}
		if err := validateChecks(task.Checks); err != nil {
			return err
		}
		known[task.ID] = true
	}
	return validateChecks(workload.Checks)
}

func ValidateReview(taskIDs map[string]Task, result ReviewResult, requirements ...Requirement) error {
	if result.Findings == nil || result.Questions == nil {
		return fmt.Errorf("review requires findings and questions arrays")
	}
	if err := validateTextList(result.Questions, "questions"); err != nil {
		return err
	}
	if result.Verification != "" && strings.TrimSpace(result.Verification) == "" {
		return fmt.Errorf("review verification must contain concrete text when present")
	}
	for _, finding := range result.Findings {
		task, known := taskIDs[finding.TaskID]
		if !known || strings.TrimSpace(finding.Requirement) == "" || strings.TrimSpace(finding.Observation) == "" {
			return fmt.Errorf("review finding lacks its exact task, requirement or observed defect")
		}
		bound := contains(task.Acceptance, finding.Requirement)
		for _, requirement := range requirements {
			if contains(task.RequirementIDs, requirement.ID) && requirement.Outcome == finding.Requirement {
				bound = true
			}
		}
		if !bound {
			return fmt.Errorf("review cannot introduce a new acceptance requirement")
		}
		if finding.File != "" && !contains(task.WriteFiles, finding.File) && !contains(task.ReadFiles, finding.File) {
			return fmt.Errorf("review finding refers to an ungranted file")
		}
	}
	return nil
}

func validateTextList(values []string, name string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return fmt.Errorf("%s must contain distinct concrete text", name)
		}
		seen[value] = true
	}
	return nil
}

func validateCheckResults(expected []Check, results []CheckResult) error {
	if len(expected) != len(results) {
		return fmt.Errorf("verification omitted frozen checks")
	}
	for index, check := range expected {
		if results[index].ID != check.ID {
			return fmt.Errorf("verification changed its frozen check identity")
		}
	}
	return nil
}

func validateChecks(checks []Check) error {
	if len(checks) == 0 {
		return fmt.Errorf("agent plan requires real verification commands")
	}
	known := map[string]bool{}
	for _, check := range checks {
		if check.ID == "" || known[check.ID] || len(check.Argv) == 0 || strings.TrimSpace(check.Argv[0]) == "" {
			return fmt.Errorf("verification check requires one unique ID and command")
		}
		for _, arg := range check.Argv {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("verification command contains NUL")
			}
		}
		known[check.ID] = true
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
