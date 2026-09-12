package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaScriptOwnsOneBehavioralVerificationPerTask(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter)
	pair, err := directCodingTaskSinglePair(program.Coverage, program.Workload.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if pair.ImplementationPath != "feature001.mjs" || pair.VerificationPath != "feature001.test.mjs" {
		t.Fatalf("unexpected implementation/test pair: %+v", pair)
	}
	if err := validateDirectCodingSinglePairSourceOwnership(program.Workload, program.Source); err != nil {
		t.Fatal(err)
	}
	ref, input := javaScriptAcceptanceFixture(t, &program)
	if ref.Block.Signature != "function verifyFeature001()" || len(input.Capabilities) != 1 || !strings.Contains(input.Capabilities[0], "function run(input, dependencies)") {
		t.Fatalf("verification received implementation declarations: %+v", input)
	}
	job, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Write ready to standard output.", "run", "arguments", "standardInput", "output", "error", "exitCode", "state", "assert.deepStrictEqual"} {
		if !strings.Contains(prompt, required) {
			t.Errorf("verification lacks necessary %q context: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{pair.ImplementationPath, pair.VerificationPath, program.Generated["feature.001"], "function feature001", "response schema", "approval", "workspace"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("verification received unrelated %q", forbidden)
		}
	}
	program.Generated["feature.001"] = "implementation bytes stay private"
	_, changed := javaScriptAcceptanceFixture(t, &program)
	changedJob, err := assemblyline.NewFragmentGenerationJob(changed)
	if err != nil {
		t.Fatal(err)
	}
	changedPrompt, err := assemblyline.RenderPortableJob(changedJob)
	if err != nil || changedPrompt != prompt {
		t.Fatalf("implementation body changed verification context: %v", err)
	}
}

func TestJavaScriptVerificationPairAvoidsEitherOccupiedLeaf(t *testing.T) {
	for _, occupied := range []string{"feature001.mjs", "feature001.test.mjs"} {
		target, err := projectJavaScriptCommandLineFocusedTargetTree(1, directCodingTargetTreeOccupation{FilePaths: []string{occupied}})
		if err != nil || !sameExactStrings(target.Paths, []string{"feature002.mjs", "feature002.test.mjs"}) {
			t.Fatalf("occupied %s did not reserve a complete fresh pair: %v %v", occupied, target.Paths, err)
		}
	}
}

func TestJavaScriptImplementationReceivesInterfacesAsDeclarations(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter)
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskImplementation {
				continue
			}
			input, err := directCodingLanguageFragmentInput(&program, assemblyline.SourceBlockRef{Document: document, Block: block}, "javascript")
			if err != nil {
				t.Fatal(err)
			}
			wantBehavior := strings.Join([]string{
				"Delivery surface: " + string(program.Workload.Surface),
				"Product context: " + program.Workload.ProductQuote,
				"Exact user requirement: " + program.Workload.Tasks[0].RequirementQuote,
			}, "\n")
			if input.Behavior != wantBehavior {
				t.Fatalf("implementation behavior acquired interface or framework instructions: %s", input.Behavior)
			}
			if strings.ContainsAny(input.Signature, "\r\n") || !strings.Contains(input.Signature, "@type {{arguments: string[], standardInput: string}}") {
				t.Fatalf("missing bounded input declaration: %s", input.Signature)
			}
			if len(input.Capabilities) != 1 || !strings.Contains(input.Capabilities[0], "@returns {"+javaScriptTaskResultType+"}") || !strings.Contains(input.Capabilities[0], "export function normalizeTaskResult(value)") {
				t.Fatalf("missing direct result interface: %v", input.Capabilities)
			}
			return
		}
	}
	t.Fatal("JavaScript task has no implementation declaration")
}

func javaScriptAcceptanceFixture(t *testing.T, program *directCodingProgram) (assemblyline.SourceBlockRef, assemblyline.FragmentGenerationInput) {
	t.Helper()
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskVerification {
				continue
			}
			ref := assemblyline.SourceBlockRef{Document: document, Block: block}
			input, err := directCodingLanguageFragmentInput(program, ref, "javascript")
			if err != nil {
				t.Fatal(err)
			}
			return ref, input
		}
	}
	t.Fatal("JavaScript task has no behavioral verification declaration")
	return assemblyline.SourceBlockRef{}, assemblyline.FragmentGenerationInput{}
}
