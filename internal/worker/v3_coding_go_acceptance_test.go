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

func TestGoDocumentsOwnTypedComparisonAndFormatting(t *testing.T) {
	program, _, ref, input := goAcceptanceFixture(t)
	if ref.Document.Path != "feature001_test.go" || ref.Block.Signature != "func ExpectedFeature001(arguments []string) string" {
		t.Fatalf("expected-value block=%+v", ref)
	}
	driver := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskSupport)
	for _, source := range []string{"value001 := Feature001(input001)", "expected := ExpectedFeature001(input001)", "if value001 != expected"} {
		if !strings.Contains(driver.Block.Static, source) {
			t.Fatalf("driver omitted %q: %s", source, driver.Block.Static)
		}
	}
	if driver.Block.Generated() {
		t.Fatal("test driver must be code-owned")
	}
	if input.Signature != ref.Block.Signature || len(input.Capabilities) != 0 {
		t.Fatalf("expected-value context=%+v", input)
	}
	if name, err := directCodingGoTaskAcceptanceName(program, "task_001"); err != nil || name != "TestFeature001" {
		t.Fatalf("test name=%q error=%v", name, err)
	}
}

func TestGoValueQuestionsExposeOnlyTheirLocalResponsibility(t *testing.T) {
	program, _, _, _ := goAcceptanceFixture(t)
	for _, role := range []assemblyline.SourceBlockRole{assemblyline.SourceBlockTaskImplementation, assemblyline.SourceBlockTaskExample, assemblyline.SourceBlockTaskVerification} {
		ref := goAcceptanceFixtureBlock(t, program, role)
		input, err := directCodingGoFragmentInput(&program, ref)
		if err != nil {
			t.Fatal(err)
		}
		job, err := assemblyline.NewFragmentGenerationJob(input)
		if err != nil {
			t.Fatal(err)
		}
		prompt, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{ref.Document.Path, "TaskResult", "CapabilityResults", "Product context:", "Delivery surface:", "testing.T", "t.Fatalf", "State", "result.Output"} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("%s leaked %q: %s", role, forbidden, prompt)
			}
		}
		if !strings.Contains(prompt, input.Signature) || !strings.Contains(prompt, "Exact user requirement: Write ready to standard output") {
			t.Fatalf("missing local value authority: %s", prompt)
		}
	}
}

func TestGoCodeOwnedDriverComparesTypedValues(t *testing.T) {
	for _, fixture := range []struct {
		name, behavior, implementation, example, expected string
		kind                                              assemblyline.ApplicationResultValueKind
	}{
		{"numeric product", "Print the product of the two integer arguments.",
			`left, _ := strconv.Atoi(arguments[0]); right, _ := strconv.Atoi(arguments[1]); return left * right`,
			`return []string{"6", "7"}`,
			`left, _ := strconv.Atoi(arguments[0]); right, _ := strconv.Atoi(arguments[1]); return left * right`, assemblyline.ApplicationResultInteger},
		{"text enclosure", "Print the first argument surrounded by square brackets.",
			`return "[" + arguments[0] + "]"`,
			`return []string{"hello"}`,
			`return "[" + arguments[0] + "]"`, assemblyline.ApplicationResultText},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program, _, _, _ := goAcceptanceFixtureForBehavior(t, fixture.behavior, fixture.implementation, fixture.kind)
			for _, value := range []struct {
				role assemblyline.SourceBlockRole
				body string
			}{
				{assemblyline.SourceBlockTaskExample, fixture.example}, {assemblyline.SourceBlockTaskVerification, fixture.expected},
			} {
				ref := goAcceptanceFixtureBlock(t, program, value.role)
				input, err := directCodingGoFragmentInput(&program, ref)
				if err != nil {
					t.Fatal(err)
				}
				source, err := validateDirectCodingGoFragment(input, value.body)
				if err != nil {
					t.Fatal(err)
				}
				program.Generated[ref.Block.ID] = source
			}
			runLiveGoAcceptanceFixture(t, program, false)
			program.Generated["feature.001"] = goWrongValueDeclaration(t, program)
			runLiveGoAcceptanceFixture(t, program, true)
		})
	}
}

func goWrongValueDeclaration(t *testing.T, program directCodingProgram) string {
	t.Helper()
	block := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskImplementation)
	wrong := `"incorrect"`
	if program.Project.ResultValueKinds["requirement_001"] == assemblyline.ApplicationResultInteger {
		wrong = "-999999"
	}
	return block.Block.Signature + " { return " + wrong + " }"
}

func goAcceptanceFixtureBlock(t *testing.T, program directCodingProgram, role assemblyline.SourceBlockRole) assemblyline.SourceBlockRef {
	t.Helper()
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role == role {
				return assemblyline.SourceBlockRef{Document: document, Block: block}
			}
		}
	}
	t.Fatalf("fixture has no %s block", role)
	return assemblyline.SourceBlockRef{}
}

func goAcceptanceFixture(t *testing.T) (
	directCodingProgram,
	assemblyline.ApplicationTaskContext,
	assemblyline.SourceBlockRef,
	assemblyline.FragmentGenerationInput,
) {
	return goAcceptanceFixtureForBehavior(t, "Write ready to standard output", `return "ready"`, assemblyline.ApplicationResultText)
}

func goAcceptanceFixtureForBehavior(t *testing.T, behavior, body string, kind assemblyline.ApplicationResultValueKind) (
	directCodingProgram,
	assemblyline.ApplicationTaskContext,
	assemblyline.SourceBlockRef,
	assemblyline.FragmentGenerationInput,
) {
	return goAcceptanceFixtureWithInput(t, behavior, body, kind, assemblyline.ApplicationInputArguments)
}

func goAcceptanceFixtureWithInput(t *testing.T, behavior, body string, kind assemblyline.ApplicationResultValueKind, channel assemblyline.ApplicationInputSource) (
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
			ID: "requirement_001", SourceQuote: behavior,
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
		specification, contexts, directCodingCapabilityGraph{}, coverage, directCodingResultValueKindPlan{"requirement_001": kind}, directCodingInputSourcePlan{"requirement_001": channel},
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
		Project:  directCodingProjectSelection{Stack: stack, Dialect: "Go", ResultValueKinds: directCodingResultValueKindPlan{"requirement_001": kind}, InputSources: directCodingInputSourcePlan{"requirement_001": channel}},
		Workload: workload, TargetTree: target, Coverage: coverage, Source: source,
		Generated: map[string]string{},
	}
	implementation := goAcceptanceFixtureBlock(t, program, assemblyline.SourceBlockTaskImplementation)
	program.Generated["feature.001"] = implementation.Block.Signature + " {\n" + body + "\n}"
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
	input, err := directCodingGoFragmentInput(&program, ref)
	if err != nil {
		t.Fatal(err)
	}
	return program, context, ref, input
}
