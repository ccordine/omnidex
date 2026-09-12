package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaOwnsBehavioralVerificationPerTask(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaCommandLineAdapter)
	pair, err := directCodingTaskSinglePair(program.Coverage, program.Workload.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if pair.ImplementationPath != "Feature001.java" || pair.VerificationPath != "Feature001Test.java" {
		t.Fatalf("unexpected Java implementation/test pair: %+v", pair)
	}
	if err := validateDirectCodingSinglePairSourceOwnership(program.Workload, program.Source); err != nil {
		t.Fatal(err)
	}
	ref, input := javaAcceptanceFixture(t, &program)
	if ref.Block.Signature != "static void verifyFeature001()" || len(input.Capabilities) != 1 {
		t.Fatalf("verification needs only its observation declaration: %+v", input)
	}
	job, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Write ready to standard output.", "static native Map<String, Object> run(", "arguments: List<String>", "standardInput: String", "output: String", "exitCode: Integer", "assert"} {
		if !strings.Contains(prompt, required) {
			t.Errorf("Java verification lacks necessary %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{pair.ImplementationPath, pair.VerificationPath, "feature001(", program.Generated["feature.001"], "Runtime.result", "response schema", "approval", "workspace"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("Java verification received unrelated %q", forbidden)
		}
	}
	program.Generated["feature.001"] = "implementation remains code-owned"
	_, changed := javaAcceptanceFixture(t, &program)
	changedJob, err := assemblyline.NewFragmentGenerationJob(changed)
	if err != nil {
		t.Fatal(err)
	}
	changedPrompt, err := assemblyline.RenderPortableJob(changedJob)
	if err != nil || changedPrompt != prompt {
		t.Fatalf("implementation changed the test-body prompt: %v", err)
	}
}

func TestJavaVerificationPairAvoidsEitherOccupiedLeaf(t *testing.T) {
	for _, occupied := range []string{"Feature001.java", "Feature001Test.java"} {
		target, err := projectJavaCommandLineFocusedTargetTree(1, directCodingTargetTreeOccupation{FilePaths: []string{occupied}})
		if err != nil || !sameExactStrings(target.Paths, []string{"Feature002.java", "Feature002Test.java"}) {
			t.Fatalf("occupied %s did not reserve a fresh complete pair: %v %v", occupied, target.Paths, err)
		}
	}
}

func javaAcceptanceFixture(t *testing.T, program *directCodingProgram) (assemblyline.SourceBlockRef, assemblyline.FragmentGenerationInput) {
	t.Helper()
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskVerification {
				continue
			}
			ref := assemblyline.SourceBlockRef{Document: document, Block: block}
			input, err := directCodingLanguageFragmentInput(program, ref, "java")
			if err != nil {
				t.Fatal(err)
			}
			return ref, input
		}
	}
	t.Fatal("Java task has no behavioral verification declaration")
	return assemblyline.SourceBlockRef{}, assemblyline.FragmentGenerationInput{}
}
