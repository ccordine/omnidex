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

func (config directCodingLanguageRepairConfig) enabled() bool {
	return config.MapStageFailure != nil
}
