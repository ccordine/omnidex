package codingagent

import (
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"strings"
)

func (task Task) Validate() error {
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.Goal) == "" {
		return fmt.Errorf("task requires identity and instructions")
	}
	for _, files := range [][]string{task.ReadFiles, task.WriteFiles, task.DeleteFiles} {
		seen := map[string]bool{}
		for _, file := range files {
			if err := model.ValidateTaskFileReference(file); err != nil {
				return err
			}
			if seen[file] {
				return fmt.Errorf("task repeats file reference %q", file)
			}
			seen[file] = true
		}
	}
	return nil
}
func ValidateReview(tasks map[string]Task, result ReviewResult) error {
	if result.Findings == nil || strings.TrimSpace(result.Verification) == "" {
		return fmt.Errorf("review requires findings and verification text")
	}
	for _, finding := range result.Findings {
		if _, known := tasks[finding.TaskID]; !known || strings.TrimSpace(finding.Observation) == "" {
			return fmt.Errorf("review finding requires its exact task and observed defect")
		}
		if finding.File != "" {
			if err := model.ValidateTaskFileReference(finding.File); err != nil {
				return err
			}
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
