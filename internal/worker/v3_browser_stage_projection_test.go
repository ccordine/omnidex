package worker

import (
	"os"
	"path/filepath"
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
			program := testTypeScriptBrowserProgram(t, fixture.name, fixture.product, fixture.requirement)
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
				assertTypeScriptStageStaticFilesMaterialize(t, stageName, stage.StaticFiles)
			}
		})
	}
}

func assertTypeScriptStageStaticFilesMaterialize(
	t *testing.T,
	stageName string,
	files []directCodingFileTask,
) {
	t.Helper()
	root := t.TempDir()
	packageFiles, err := directCodingStagePackageFiles(files)
	if err != nil {
		t.Fatalf("%s stage package authority: %v", stageName, err)
	}
	workspace := directCodingTypeScriptStageWorkspace{
		root: root, packageAuthority: make(map[string]directCodingFileTask, len(packageFiles)),
	}
	for _, file := range packageFiles {
		workspace.packageAuthority[file.Path] = file
		if err := writeDirectCodingStageFile(root, file); err != nil {
			t.Fatalf("write %s stage package authority: %v", stageName, err)
		}
	}
	if err := workspace.resetSource(); err != nil {
		t.Fatalf("reset %s stage source: %v", stageName, err)
	}
	if err := workspace.writeAssembly(directCodingAssembly{Files: files}); err != nil {
		t.Fatalf("materialize %s stage static authority: %v", stageName, err)
	}
	byPath := make(map[string]directCodingFileTask, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, required := range []string{
		"package.json", "package-lock.json", "tsconfig.json", "vite.config.ts",
	} {
		expected, exists := byPath[required]
		if !exists {
			t.Fatalf("%s stage authority omits %s", stageName, required)
		}
		actual, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(required)))
		if err != nil {
			t.Fatalf("read materialized %s stage file %s: %v", stageName, required, err)
		}
		if !reflect.DeepEqual(actual, expected.Content) {
			t.Fatalf("materialized %s stage file %s differs from authority", stageName, required)
		}
	}
}
