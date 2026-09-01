package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoFocusedTargetTreeAllocatesOneImplementationAcceptancePair(t *testing.T) {
	target, err := projectGoCommandLineFocusedTargetTree(
		1, directCodingTargetTreeOccupation{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"feature001.go", "feature001_test.go"}
	if !sameExactStrings(target.Paths, want) {
		t.Fatalf("target paths=%v; want %v", target.Paths, want)
	}

	target, err = projectGoCommandLineFocusedTargetTree(1, directCodingTargetTreeOccupation{
		FilePaths: []string{"feature001.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"feature002.go", "feature002_test.go"}
	if !sameExactStrings(target.Paths, want) {
		t.Fatalf("occupied target paths=%v; want %v", target.Paths, want)
	}
}

func TestGoDocumentsOwnAcceptanceStructureAndPairPlacement(t *testing.T) {
	program, _, ref, _ := goAcceptanceFixture(t)
	if ref.Document.Path != "feature001_test.go" {
		t.Fatalf("acceptance path=%q", ref.Document.Path)
	}
	if ref.Document.Preamble != "package main\n\nimport \"testing\"" {
		t.Fatalf("acceptance preamble=%q", ref.Document.Preamble)
	}
	if ref.Block.Signature != "func TestFeature001(t *testing.T)" ||
		ref.Block.Role != assemblyline.SourceBlockTaskVerification ||
		ref.Block.TaskID != "task_001" {
		t.Fatalf("acceptance block=%+v", ref.Block)
	}
	if !sameExactStrings(ref.Block.DependsOn, []string{"runtime.api", "feature.001"}) ||
		!sameExactStrings(ref.Block.Capabilities, []string{"runtime.api", "feature.001"}) {
		t.Fatalf("acceptance authority=%+v", ref.Block)
	}
	if !sameExactStrings(ref.Block.Globals, []string{"Fatal", "Fatalf", "Error", "Errorf"}) {
		t.Fatalf("acceptance testing methods=%v", ref.Block.Globals)
	}
	runtime, exists := directCodingSourceBlueprintBlock(program.Source, "runtime.api")
	if !exists || !strings.Contains(runtime.API, "Command-line arguments excluding") ||
		!strings.Contains(runtime.API, "Complete standard-input text") ||
		!strings.Contains(runtime.API, "Process status; zero means success") {
		t.Fatalf("code-owned runtime field semantics=%q", runtime.API)
	}
	if name, err := directCodingGoTaskAcceptanceName(program, "task_001"); err != nil {
		t.Fatal(err)
	} else if name != "TestFeature001" {
		t.Fatalf("acceptance name=%q", name)
	}
}

func TestGoAcceptancePromptIsOneOrdinaryPathBlindBodyQuestion(t *testing.T) {
	_, _, ref, input := goAcceptanceFixture(t)
	job, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{
		"What Go statements implement this behavior?",
		"func TestFeature001(t *testing.T)",
		"Demonstrate this behavior by exercising Feature001",
		"Exact user requirement: Write ready to standard output",
		"Command-line arguments excluding the executable name",
		"User-visible standard-output text",
		"Process status; zero means success",
	} {
		if !strings.Contains(prompt, wanted) {
			t.Fatalf("acceptance prompt omitted %q: %s", wanted, prompt)
		}
	}
	for _, forbidden := range []string{
		ref.Document.Path, "response schema", "response packet", "return json",
		"preserve", "reproduce", "AST", "BEGIN_", "END_",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("acceptance prompt leaked %q: %s", forbidden, prompt)
		}
	}
}

func TestGoAcceptanceValidatesCodeOwnedDeclarationAroundOrdinaryBody(t *testing.T) {
	program, _, ref, input := goAcceptanceFixture(t)
	body := `result := Feature001(TaskInput{}, CapabilityResults{})
if result.Output != "ready" {
	t.Fatalf("output = %q", result.Output)
}`
	declaration, err := validateDirectCodingGoFragment(input, body)
	if err != nil {
		t.Fatalf("validate ordinary body: %v", err)
	}
	if !strings.HasPrefix(declaration, "func TestFeature001(t *testing.T) {") {
		t.Fatalf("code-owned declaration=%q", declaration)
	}
	if err := validateDirectCodingGoAcceptance(&program, ref, declaration); err != nil {
		t.Fatalf("validate acceptance: %v", err)
	}
	program.Generated[ref.Block.ID] = declaration
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatalf("assemble code-owned Go documents: %v", err)
	}
	assembled := ""
	for _, file := range assembly.Files {
		if file.Path == "feature001_test.go" {
			assembled = string(file.Content)
		}
	}
	if !strings.HasPrefix(assembled, "package main\n\nimport \"testing\"\n\nfunc TestFeature001(t *testing.T) {") ||
		!strings.Contains(assembled, `result := Feature001(TaskInput{}, CapabilityResults{})`) {
		t.Fatalf("assembled code-owned acceptance document=%q", assembled)
	}
}

func TestGoAcceptanceRejectsUnprovenOrStructurallyInvalidBodies(t *testing.T) {
	program, _, ref, _ := goAcceptanceFixture(t)
	for name, source := range map[string]string{
		"wrong declaration": `func TestSomethingElse(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if result.Output != "ready" { t.Fatal("wrong") }
		}`,
		"no implementation call": `func TestFeature001(t *testing.T) { t.Fatal("missing") }`,
		"detached failure": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			_ = result
			t.Fatal("detached")
		}`,
		"boolean shortcut": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if result.Output != "ready" || true { t.Fatal("tautology") }
		}`,
		"self comparison": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if result.Output == result.Output { t.Fatal("tautology") }
		}`,
		"nested proof": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if result.Output != "ready" { func() { t.Fatal("nested") }() }
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingGoAcceptance(&program, ref, source); err == nil {
				t.Fatalf("accepted invalid source: %s", source)
			}
		})
	}
}

func goAcceptanceFixture(t *testing.T) (
	directCodingProgram,
	assemblyline.ApplicationTaskContext,
	assemblyline.SourceBlockRef,
	assemblyline.FragmentGenerationInput,
) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceCommandLine,
		ProductQuote: "small Go command",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Write ready to standard output",
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		t.Fatal(err)
	}
	context := contexts["requirement_001"]
	target, err := projectGoCommandLineFocusedTargetTree(1, directCodingTargetTreeOccupation{})
	if err != nil {
		t.Fatal(err)
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(
		stack, workload, target,
		map[string][]string{"task_001": append([]string(nil), target.Paths...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := genericGoCommandLineDocuments(
		specification, contexts, directCodingCapabilityGraph{}, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	source, err := bindDirectCodingSourceBlueprintAdapters(
		stack, assemblyline.SourceBlueprint{Documents: documents},
	)
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{
		Project:  directCodingProjectSelection{Stack: stack, Dialect: "Go"},
		Workload: workload, TargetTree: target, Coverage: coverage, Source: source,
		Generated: map[string]string{
			"feature.001": `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	return TaskResult{Output: "ready"}
}`,
		},
	}
	var ref assemblyline.SourceBlockRef
	for _, document := range source.Documents {
		for _, block := range document.Blocks {
			if block.Role == assemblyline.SourceBlockTaskVerification {
				ref = assemblyline.SourceBlockRef{Document: document, Block: block}
			}
		}
	}
	if ref.Block.ID == "" {
		t.Fatal("Go acceptance fixture has no verification block")
	}
	input, err := directCodingLanguageFragmentInput(&program, ref, "go")
	if err != nil {
		t.Fatal(err)
	}
	return program, context, ref, input
}
