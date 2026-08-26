package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingAcceptanceCriterionCount(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (int, error) {
	if stage == nil || ref.Block.TaskID == "" {
		return 0, fmt.Errorf("acceptance block %s has no frozen task authority", ref.Block.ID)
	}
	for _, task := range stage.Workload.Tasks {
		if task.ID != ref.Block.TaskID {
			continue
		}
		if len(task.AcceptanceCriteria) == 0 {
			return 0, fmt.Errorf("acceptance task %s has no frozen criteria", task.ID)
		}
		return len(task.AcceptanceCriteria), nil
	}
	return 0, fmt.Errorf("acceptance block %s references unknown task %s", ref.Block.ID, ref.Block.TaskID)
}
