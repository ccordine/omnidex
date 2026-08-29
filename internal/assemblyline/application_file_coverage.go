package assemblyline

import (
	"fmt"
	"sort"
)

// ApplicationFileCoveragePlan is code-owned provenance from focused path-only
// tree calls. It is not model output and grants no content, graph, ordering, or
// filesystem authority.
type ApplicationFileCoveragePlan struct {
	WorkloadSHA256 string
	Files          []ApplicationFileCoverage
}

type ApplicationFileCoverage struct {
	Path    string
	Kind    TargetArtifactKind
	TaskIDs []string
}

func (plan ApplicationFileCoveragePlan) ValidateFor(
	target TargetTree,
	workload FrozenApplicationWorkload,
) error {
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != workload.SHA256 {
		return fmt.Errorf("file coverage plan differs from frozen workload authority")
	}
	if len(target.Paths) == 0 || len(plan.Files) != len(target.Paths) {
		return fmt.Errorf("file coverage plan must cover every target-tree path exactly once")
	}
	taskOrder := make(map[string]int, len(workload.Tasks))
	coveredTasks := make(map[string]struct{}, len(workload.Tasks))
	for index, task := range workload.Tasks {
		taskOrder[task.ID] = index
	}
	for index, file := range plan.Files {
		if index >= len(target.Paths) || file.Path != target.Paths[index] {
			return fmt.Errorf("file coverage plan paths must equal the canonical target tree")
		}
		if err := validateTargetTreePath(file.Path); err != nil {
			return fmt.Errorf("file coverage path %d: %w", index, err)
		}
		switch file.Kind {
		case TargetArtifactImplementation, TargetArtifactVerification:
		default:
			return fmt.Errorf("file coverage path %s has unsupported artifact kind %q", file.Path, file.Kind)
		}
		if len(file.TaskIDs) == 0 {
			return fmt.Errorf("file coverage path %s has no task provenance", file.Path)
		}
		lastOrder := -1
		seen := make(map[string]struct{}, len(file.TaskIDs))
		for _, taskID := range file.TaskIDs {
			order, exists := taskOrder[taskID]
			if !exists {
				return fmt.Errorf("file coverage path %s names unknown task %s", file.Path, taskID)
			}
			if _, duplicate := seen[taskID]; duplicate || order <= lastOrder {
				return fmt.Errorf("file coverage path %s has duplicated or unordered task provenance", file.Path)
			}
			seen[taskID] = struct{}{}
			coveredTasks[taskID] = struct{}{}
			lastOrder = order
		}
	}
	if len(coveredTasks) != len(workload.Tasks) {
		return fmt.Errorf("file coverage plan does not cover every frozen task")
	}
	return nil
}

func (plan ApplicationFileCoveragePlan) FilesForTask(
	taskID string,
) ([]ApplicationFileCoverage, error) {
	files := make([]ApplicationFileCoverage, 0)
	for _, file := range plan.Files {
		for _, owner := range file.TaskIDs {
			if owner == taskID {
				copy := file
				copy.TaskIDs = append([]string(nil), file.TaskIDs...)
				files = append(files, copy)
				break
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("file coverage plan has no files for task %s", taskID)
	}
	return files, nil
}

func (plan ApplicationFileCoveragePlan) TasksForPath(path string) ([]string, error) {
	for _, file := range plan.Files {
		if file.Path == path {
			return append([]string(nil), file.TaskIDs...), nil
		}
	}
	return nil, fmt.Errorf("file coverage plan has no target path %s", path)
}

func NewApplicationFileCoveragePlan(
	workload FrozenApplicationWorkload,
	target TargetTree,
	provenance map[string][]string,
	kinds map[string]TargetArtifactKind,
) (ApplicationFileCoveragePlan, error) {
	paths := append([]string(nil), target.Paths...)
	sort.Strings(paths)
	if len(paths) != len(target.Paths) {
		return ApplicationFileCoveragePlan{}, fmt.Errorf("target tree path cardinality changed during coverage construction")
	}
	target.Paths = paths
	files := make([]ApplicationFileCoverage, len(paths))
	for index, path := range paths {
		files[index] = ApplicationFileCoverage{
			Path: path, Kind: kinds[path], TaskIDs: append([]string(nil), provenance[path]...),
		}
	}
	plan := ApplicationFileCoveragePlan{WorkloadSHA256: workload.SHA256, Files: files}
	if err := plan.ValidateFor(target, workload); err != nil {
		return ApplicationFileCoveragePlan{}, err
	}
	return plan, nil
}
