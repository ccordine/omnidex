package worker

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptBrowserStageProjectionsRetainStaticVerificationAuthority(t *testing.T) {
	fixtures := []struct {
		name        string
		product     string
		requirement string
	}{
		{
			name:        "maintenance tracker",
			product:     "A maintenance tracker",
			requirement: "Expose the current status of one scheduled maintenance task.",
		},
		{
			name:        "text summarizer",
			product:     "A text summarizer",
			requirement: "Accept supplied text and expose one resulting summary.",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			program := testTypeScriptBrowserProgram(t, fixture.product, fixture.requirement)
			context, err := assemblyline.ProjectApplicationTaskContext(
				program.Workload, program.Workload.Tasks[0].ID,
			)
			if err != nil {
				t.Fatalf("project task context: %v", err)
			}
			taskStage, err := projectDirectCodingApplicationTaskStage(program, context)
			if err != nil {
				t.Fatalf("project task verification stage: %v", err)
			}
			implementationID, err := directCodingTaskBlockIDByRole(
				taskStage.Source,
				context.Task.TaskID,
				assemblyline.SourceBlockTaskImplementation,
			)
			if err != nil {
				t.Fatalf("resolve implementation block: %v", err)
			}
			implementationStage, err := projectDirectCodingTypeScriptImplementationStage(
				&taskStage, context.Task.TaskID, implementationID,
			)
			if err != nil {
				t.Fatalf("project implementation verification stage: %v", err)
			}

			for stageName, stage := range map[string]directCodingProgram{
				"implementation": implementationStage,
				"task":           taskStage,
			} {
				if !reflect.DeepEqual(stage.StaticFiles, program.StaticFiles) {
					t.Fatalf("%s stage lost exact static-file authority", stageName)
				}
				assertTypeScriptStageStaticFileAuthority(t, stageName, stage.StaticFiles)
			}
		})
	}
}

func assertTypeScriptStageStaticFileAuthority(
	t *testing.T,
	stageName string,
	files []directCodingFileTask,
) {
	t.Helper()
	packageFiles, err := directCodingStagePackageFiles(files)
	if err != nil {
		t.Fatalf("%s stage package authority: %v", stageName, err)
	}
	byPath := make(map[string]directCodingFileTask, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, required := range []string{
		"package.json", "package-lock.json", "tsconfig.json", "vite.config.ts",
	} {
		expected, exists := byPath[required]
		if !exists || len(expected.Content) == 0 {
			t.Fatalf("%s stage authority omits %s", stageName, required)
		}
	}
	for _, file := range packageFiles {
		if !reflect.DeepEqual(file, byPath[file.Path]) {
			t.Fatalf("%s package authority differs from exact static file %s", stageName, file.Path)
		}
	}
}
