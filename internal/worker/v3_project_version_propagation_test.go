package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestVersionProfileIDPropagatesFromSelectionThroughTaskAssembly(t *testing.T) {
	specification, workload := goCommandLineStackFixture(t)
	profile := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	static, err := genericGoCommandLineStaticFiles(profile, "profile-propagation")
	if err != nil {
		t.Fatal(err)
	}
	selection, err := selectDirectCodingProject(
		typedWorkerRuntime{}, nil, specification,
		map[string]string{"go.mod": directCodingTestFileContent(t, static, "go.mod")}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.VersionProfileID != profile.ID {
		t.Fatalf("selected profile=%s want=%s", selection.VersionProfileID, profile.ID)
	}

	target, coverage, err := resolveDirectCodingTargetTree(
		typedWorkerRuntime{}, "", "", specification, workload,
		selection.Stack, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if target.VersionProfileID != "" {
		t.Fatalf("code-owned focused tree selected profile %q", target.VersionProfileID)
	}
	target.VersionProfileID = selection.VersionProfileID
	if _, err := directCodingVersionProfileForTargetTree(target); err != nil {
		t.Fatal(err)
	}

	program, err := compileDirectCodingProgram(
		"profile-propagation", specification, nil, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	if program.VersionProfileID != profile.ID {
		t.Fatalf("program profile=%s want=%s", program.VersionProfileID, profile.ID)
	}
	program.Generated["feature.001"] = `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	if len(input.Arguments) == 0 { return TaskResult{Error: "argument required", ExitCode: 2} }
	return TaskResult{Output: input.Arguments[0]}
}`
	program.Generated["acceptance.001"] = `func TestFeature001(t *testing.T) {
	result := Feature001(TaskInput{Arguments: []string{"ready"}}, CapabilityResults{})
	if result.Output != "ready" { t.Fatalf("output = %q", result.Output) }
}`
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if assembly.VersionProfileID != profile.ID {
		t.Fatalf("assembly profile=%s want=%s", assembly.VersionProfileID, profile.ID)
	}

	contexts, err := directCodingApplicationTaskContexts(applicationWorkloadInput(specification), workload)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, contexts["requirement_001"])
	if err != nil {
		t.Fatal(err)
	}
	if stage.VersionProfileID != profile.ID {
		t.Fatalf("task stage profile=%s want=%s", stage.VersionProfileID, profile.ID)
	}
	stageAssembly, err := directCodingAssemblyFromProgram(stage)
	if err != nil {
		t.Fatal(err)
	}
	if stageAssembly.VersionProfileID != profile.ID {
		t.Fatalf("task assembly profile=%s want=%s", stageAssembly.VersionProfileID, profile.ID)
	}
	ref := directCodingTestGeneratedBlockRef(t, stage.Source, "feature.001")
	input, err := directCodingLanguageFragmentInput(&stage, ref, "go")
	if err != nil {
		t.Fatal(err)
	}
	if input.Dialect != profile.SourceDialect {
		t.Fatalf("task fragment dialect=%q want=%q", input.Dialect, profile.SourceDialect)
	}
}

func TestProgramVersionProfileRejectsTargetTreeAuthorityMismatch(t *testing.T) {
	tests := []struct {
		name    string
		target  assemblyline.TargetTree
		failure string
	}{
		{
			name: "stack mismatch",
			target: assemblyline.TargetTree{
				StackID:          "unknown_stack",
				VersionProfileID: goCommandLineVersionProfileV1,
				Paths:            []string{"main.go"},
			},
			failure: `program target tree stack "unknown_stack" differs from program authority "` + genericGoCommandLineAdapter + `"`,
		},
		{
			name: "profile mismatch",
			target: assemblyline.TargetTree{
				StackID:          genericGoCommandLineAdapter,
				VersionProfileID: "unknown_profile",
				Paths:            []string{"main.go"},
			},
			failure: `program target tree version profile "unknown_profile" differs from program authority "` + goCommandLineVersionProfileV1 + `"`,
		},
		{
			name: "missing profile ID",
			target: assemblyline.TargetTree{
				StackID: genericGoCommandLineAdapter,
				Paths:   []string{"main.go"},
			},
			failure: `program target tree version profile "" differs from program authority "` + goCommandLineVersionProfileV1 + `"`,
		},
		{
			name: "missing stack ID",
			target: assemblyline.TargetTree{
				VersionProfileID: goCommandLineVersionProfileV1,
				Paths:            []string{"main.go"},
			},
			failure: `program target tree stack "" differs from program authority "` + genericGoCommandLineAdapter + `"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := directCodingVersionProfileForProgram(directCodingProgram{
				StackID:          genericGoCommandLineAdapter,
				VersionProfileID: goCommandLineVersionProfileV1,
				TargetTree:       test.target,
			})
			if err == nil || err.Error() != test.failure {
				t.Fatalf("error=%v want=%q", err, test.failure)
			}
		})
	}
}

func TestProgramVersionProfileAllowsAbsentTargetTreeAuthority(t *testing.T) {
	profile, err := directCodingVersionProfileForProgram(directCodingProgram{
		StackID:          genericGoCommandLineAdapter,
		VersionProfileID: goCommandLineVersionProfileV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != goCommandLineVersionProfileV1 {
		t.Fatalf("profile=%q want=%q", profile.ID, goCommandLineVersionProfileV1)
	}
}

func directCodingTestGeneratedBlockRef(
	t *testing.T,
	blueprint assemblyline.SourceBlueprint,
	blockID string,
) assemblyline.SourceBlockRef {
	t.Helper()
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.ID == blockID {
				return assemblyline.SourceBlockRef{Document: document, Block: block}
			}
		}
	}
	t.Fatalf("source blueprint omits block %s in %s", blockID, strings.Join(taskStageDocumentIDs(directCodingProgram{Source: blueprint}), ","))
	return assemblyline.SourceBlockRef{}
}
