package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingLanguageStageRepair struct {
	Target     assemblyline.SourceBlockRef
	Diagnostic string
}

type directCodingLanguageStageRepairMapper func(
	directCodingProgram,
	[]assemblyline.ComposedSourceDocument,
	testCommand,
	string,
) (directCodingLanguageStageRepair, bool, error)

type directCodingLanguageRepairConfig struct {
	MapStageFailure directCodingLanguageStageRepairMapper
}

type directCodingLanguageRepairModelResolver func() (string, string, error)

func (config directCodingLanguageRepairConfig) stageFailureEnabled() bool {
	return config.MapStageFailure != nil
}
