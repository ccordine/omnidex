package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourceHasNoApplicationWorkloadPlannerBoundary(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"ApplicationWorkloadDraft",
		"ApplicationJobSpecification",
		"WorkApplicationJobObjective",
		"WorkApplicationBehaviorCoverage",
		"WorkApplicationBehavior",
		"WorkApplicationCriterionCoverage",
		"WorkApplicationCriterion",
		"application_job_objective",
		"application_job_specification",
		"application_behavior_coverage",
		"application_behavior",
		"application_criterion_coverage",
		"application_criterion",
		"resolveDirectCodingApplicationWorkload",
		"CodingWorkload",
		"omnidex.application-workload.v1",
	}
	for _, relative := range []string{"cmd", "internal"} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, token := range forbidden {
				if strings.Contains(source, token) {
					t.Errorf("production source %s retains workload planner boundary %q", path, token)
				}
			}
		})
	}
}
