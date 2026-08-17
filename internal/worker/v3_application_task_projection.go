package worker

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func projectDirectCodingApplicationTaskStage(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) (directCodingProgram, error) {
	var zero directCodingProgram
	if err := program.TypeScript.Validate(); err != nil {
		return zero, err
	}
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return zero, fmt.Errorf("application task context differs from program workload authority")
	}
	featureID, acceptanceID, err := applicationTaskBlockIDs(context.Task.TaskID)
	if err != nil {
		return zero, err
	}
	documentIDs := map[string]struct{}{
		"application_runtime": {},
		"feature_" + strings.TrimPrefix(featureID, "feature."):          {},
		"acceptance_" + strings.TrimPrefix(acceptanceID, "acceptance."): {},
	}
	documents := make([]assemblyline.TypeScriptDocument, 0, len(documentIDs))
	for _, document := range program.TypeScript.Documents {
		if _, include := documentIDs[document.ID]; include {
			documents = append(documents, document)
			delete(documentIDs, document.ID)
		}
	}
	if len(documentIDs) != 0 {
		return zero, fmt.Errorf("application task %s stage lacks required documents", context.Task.TaskID)
	}

	requiredStatic := map[string]struct{}{
		"package.json": {}, "tsconfig.json": {}, "vite.config.ts": {},
	}
	staticFiles := make([]directCodingFileTask, 0, len(requiredStatic))
	for _, file := range program.StaticFiles {
		if _, include := requiredStatic[file.Path]; include {
			staticFiles = append(staticFiles, file)
			delete(requiredStatic, file.Path)
		}
	}
	if len(requiredStatic) != 0 {
		return zero, fmt.Errorf("application task %s stage lacks required toolchain files", context.Task.TaskID)
	}

	generated := make(map[string]string, 2)
	grounding := make(map[string]assemblyline.ApplicationAcceptanceGroundingReceipt, 1)
	for _, blockID := range []string{featureID, acceptanceID} {
		if source := program.Generated[blockID]; strings.TrimSpace(source) != "" {
			generated[blockID] = source
		}
	}
	if receipt, exists := program.AcceptanceGrounding[acceptanceID]; exists {
		grounding[acceptanceID] = receipt
	}
	stage := directCodingProgram{
		Adapter: program.Adapter, PackageName: program.PackageName,
		Workload: program.Workload, TypeScript: assemblyline.TypeScriptBlueprint{Documents: documents},
		StaticFiles: staticFiles, Generated: generated, AcceptanceGrounding: grounding,
		AcceptanceGroundingSeen: program.AcceptanceGroundingSeen,
	}
	if err := stage.TypeScript.Validate(); err != nil {
		return zero, fmt.Errorf("validate application task stage: %w", err)
	}
	if !reflect.DeepEqual(stage.Workload, program.Workload) {
		return zero, fmt.Errorf("application task stage changed frozen workload authority")
	}
	return stage, nil
}
