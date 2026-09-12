package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func validateDirectCodingLanguageAcceptanceProgram(program directCodingProgram) error {
	count := 0
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskVerification {
				continue
			}
			adapter, err := directCodingArtifactAdapterByID(document.AdapterID)
			if err != nil {
				return err
			}
			if adapter.ValidateAcceptance == nil {
				return fmt.Errorf("adapter %s has no generated verification-body validator", adapter.ID)
			}
			count++
			if err := adapter.ValidateAcceptance(assemblyline.SourceBlockRef{Document: document, Block: block}, program.Generated[block.ID]); err != nil {
				return fmt.Errorf("validate %s task %s acceptance: %w", adapter.ID, block.TaskID, err)
			}
		}
	}
	if count == 0 {
		return fmt.Errorf("program has no task-owned behavioral acceptance")
	}
	return nil
}
