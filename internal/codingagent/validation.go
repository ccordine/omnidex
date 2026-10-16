package codingagent

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func (task Task) Validate() error {
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.Goal) == "" || len(task.Acceptance) == 0 {
		return fmt.Errorf("task packet requires identity, goal, and acceptance criteria")
	}
	if err := validateTextList(task.Acceptance, "task acceptance criteria"); err != nil {
		return err
	}
	if err := validateTextList(task.KnownAffectedSystems, "task affected systems"); err != nil {
		return err
	}
	guidance := map[string]bool{}
	for _, item := range task.Guidance {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Text) == "" || guidance[item.ID] {
			return fmt.Errorf("task guidance requires distinct identities, kinds, and concrete text")
		}
		guidance[item.ID] = true
	}
	write := map[string]bool{}
	read := map[string]bool{}
	deleted := map[string]bool{}
	for _, file := range task.WriteFiles {
		if err := model.ValidateTaskFileReference(file); err != nil {
			return err
		}
		if write[file] {
			return fmt.Errorf("task packet repeats write file %q", file)
		}
		write[file] = true
	}
	for _, file := range task.DeleteFiles {
		if err := model.ValidateTaskFileReference(file); err != nil {
			return err
		}
		if deleted[file] {
			return fmt.Errorf("task packet repeats delete file %q", file)
		}
		deleted[file] = true
	}
	for _, file := range task.ReadFiles {
		if err := model.ValidateTaskFileReference(file); err != nil {
			return err
		}
		if read[file] {
			return fmt.Errorf("task packet repeats read file %q", file)
		}
		read[file] = true
	}
	checks := map[string]bool{}
	for _, check := range task.Checks {
		if strings.TrimSpace(check.ID) == "" || len(check.Argv) == 0 || strings.TrimSpace(check.Argv[0]) == "" || checks[check.ID] {
			return fmt.Errorf("task check requires a distinct identity and exact command")
		}
		checks[check.ID] = true
	}
	return nil
}

func ValidateReview(taskIDs map[string]Task, result ReviewResult, requirements ...Requirement) error {
	if result.Findings == nil {
		return fmt.Errorf("review requires a findings array")
	}
	if strings.TrimSpace(result.Verification) == "" {
		return fmt.Errorf("review requires concrete verification text")
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
		if finding.File != "" {
			if err := model.ValidateTaskFileReference(finding.File); err != nil {
				return fmt.Errorf("review finding file: %w", err)
			}
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

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
