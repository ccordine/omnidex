package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoRuntimeCapabilityNoneEmitsNoWrapperAuthority(t *testing.T) {
	t.Parallel()
	program := goRuntimeCapabilityFixtureProgram(t, nil)
	runtime := directCodingTestSourceDocument(t, program.Source, "application_runtime")
	if runtime.Preamble != "package main" || len(runtime.Blocks) != 1 ||
		runtime.Blocks[0].ID != "runtime.api" {
		t.Fatalf("NONE runtime document=%+v", runtime)
	}
	feature := directCodingTestGeneratedBlockRef(t, program.Source, "feature.001")
	if !reflect.DeepEqual(feature.Block.Capabilities, []string{"runtime.api"}) {
		t.Fatalf("NONE implementation capabilities=%q", feature.Block.Capabilities)
	}
	input, err := directCodingLanguageFragmentInput(&program, feature, "go")
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.Join(input.Capabilities, "\n")
	if strings.Contains(visible, "RuntimeReadFile") ||
		strings.Contains(visible, "RuntimeEnvironmentValue") {
		t.Fatalf("NONE exposed runtime wrapper APIs: %q", visible)
	}
}

func TestGoRuntimeCapabilityScopeRejectsUnselectedAndLateAuthority(t *testing.T) {
	t.Parallel()
	base := goRuntimeCapabilityBaseProgram(t)
	candidates, err := directCodingGoRuntimeCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	unknown := directCodingRuntimeCapabilityGraph{
		"requirement_001": {"runtime.stdlib.unregistered"},
	}
	if err := validateDirectCodingRuntimeCapabilityGraph(
		[]assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Echo the input."},
		},
		candidates, unknown,
	); err == nil || !strings.Contains(err.Error(), "unregistered") {
		t.Fatalf("unregistered runtime capability error=%v", err)
	}
	emptyStack := directCodingProjectStack{ID: "without-runtime-capabilities"}
	unchanged, err := bindDirectCodingRuntimeCapabilities(
		emptyStack, base, emptyDirectCodingRuntimeCapabilityGraph(
			[]assemblyline.Requirement{
				{ID: "requirement_001", SourceQuote: "Echo the input."},
			},
		),
	)
	if err != nil || unchanged.Workload.SHA256 != base.Workload.SHA256 {
		t.Fatalf("empty unregistered stack binding=%+v error=%v", unchanged, err)
	}
	if _, err := bindDirectCodingRuntimeCapabilities(
		emptyStack,
		base,
		directCodingRuntimeCapabilityGraph{
			"requirement_001": {"runtime.stdlib.read_file"},
		},
	); err == nil || !strings.Contains(
		err.Error(), "does not register runtime capabilities",
	) {
		t.Fatalf("unregistered stack runtime authority error=%v", err)
	}
	base.Generated["feature.001"] = `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	return TaskResult{Output: input.StandardInput}
}`
	if _, err := bindDirectCodingGoRuntimeCapabilities(
		base,
		directCodingRuntimeCapabilityGraph{
			"requirement_001": {"runtime.stdlib.read_file"},
		},
	); err == nil || !strings.Contains(err.Error(), "before source generation") {
		t.Fatalf("late runtime authority error=%v", err)
	}

	environmentOnly := goRuntimeCapabilityFixtureProgram(
		t, []string{"runtime.stdlib.environment_value"},
	)
	feature := directCodingTestGeneratedBlockRef(t, environmentOnly.Source, "feature.001")
	input, err := directCodingLanguageFragmentInput(&environmentOnly, feature, "go")
	if err != nil {
		t.Fatal(err)
	}
	readFileCandidate := `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	contents, _ := RuntimeReadFile(input.Arguments[0])
	return TaskResult{Output: string(contents)}
}`
	if _, err := validateDirectCodingGoFragment(
		input, readFileCandidate,
	); err == nil || !strings.Contains(err.Error(), "RuntimeReadFile") {
		t.Fatalf("unselected wrapper error=%v", err)
	}
}

func TestGoRuntimeCapabilityAPIsDoNotAuthorizeWrapperParameterNames(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		id         string
		identifier string
	}{
		{id: "runtime.stdlib.environment_value", identifier: "key"},
		{id: "runtime.stdlib.read_file", identifier: "name"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.id, func(t *testing.T) {
			t.Parallel()
			program := goRuntimeCapabilityFixtureProgram(t, []string{fixture.id})
			feature := directCodingTestGeneratedBlockRef(t, program.Source, "feature.001")
			input, err := directCodingLanguageFragmentInput(&program, feature, "go")
			if err != nil {
				t.Fatal(err)
			}
			candidate := `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	return TaskResult{Output: ` + fixture.identifier + `}
}`
			if _, err := validateDirectCodingGoFragment(
				input, candidate,
			); err == nil || !strings.Contains(err.Error(), fixture.identifier) {
				t.Fatalf(
					"undeclared wrapper parameter %s error=%v",
					fixture.identifier, err,
				)
			}
		})
	}
}

func directCodingTestSourceDocument(
	t *testing.T,
	blueprint assemblyline.SourceBlueprint,
	documentID string,
) assemblyline.SourceDocument {
	t.Helper()
	for _, document := range blueprint.Documents {
		if document.ID == documentID {
			return document
		}
	}
	t.Fatalf("source blueprint omits document %s", documentID)
	return assemblyline.SourceDocument{}
}

func directCodingTestSourceBlock(
	t *testing.T,
	document assemblyline.SourceDocument,
	blockID string,
) assemblyline.SourceBlock {
	t.Helper()
	for _, block := range document.Blocks {
		if block.ID == blockID {
			return block
		}
	}
	t.Fatalf("source document %s omits block %s", document.ID, blockID)
	return assemblyline.SourceBlock{}
}
