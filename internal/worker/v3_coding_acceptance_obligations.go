package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingAcceptanceObligationCount(
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
		return 1, nil
	}
	return 0, fmt.Errorf("acceptance block %s references unknown task %s", ref.Block.ID, ref.Block.TaskID)
}
