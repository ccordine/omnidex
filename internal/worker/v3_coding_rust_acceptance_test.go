package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRustOwnsBehavioralVerificationPerTask(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericRustCommandLineAdapter)
	pair, err := directCodingTaskSinglePair(program.Coverage, program.Workload.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if pair.ImplementationPath != "src/feature001.rs" || pair.VerificationPath != "src/feature001_test.rs" {
		t.Fatalf("unexpected Rust implementation/test pair: %+v", pair)
	}
	if err := validateDirectCodingSinglePairSourceOwnership(program.Workload, program.Source); err != nil {
		t.Fatal(err)
	}
	ref, input := rustAcceptanceFixture(t, &program)
	if ref.Block.Signature != "fn verify_feature_001()" || len(input.Capabilities) != 4 {
		t.Fatalf("verification lacks its direct type and observation declarations: %+v", input)
	}
	job, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Write ready to standard output.", "fn run(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult", "pub struct TaskInput", "pub struct TaskResult", "pub type CapabilityResults", "assert_eq"} {
		if !strings.Contains(prompt, required) {
			t.Errorf("Rust verification lacks necessary %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{pair.ImplementationPath, pair.VerificationPath, "pub fn feature_001", program.Generated["feature.001"], "response schema", "approval", "workspace"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("Rust verification received unrelated %q", forbidden)
		}
	}
	program.Generated["feature.001"] = "implementation remains code-owned"
	_, changed := rustAcceptanceFixture(t, &program)
	changedJob, err := assemblyline.NewFragmentGenerationJob(changed)
	if err != nil {
		t.Fatal(err)
	}
	changedPrompt, err := assemblyline.RenderPortableJob(changedJob)
	if err != nil || changedPrompt != prompt {
		t.Fatalf("implementation changed the test-body prompt: %v", err)
	}
}

func TestRustVerificationPairAvoidsEitherOccupiedLeaf(t *testing.T) {
	for _, occupied := range []string{"src/feature001.rs", "src/feature001_test.rs"} {
		target, err := projectRustCommandLineFocusedTargetTree(1, directCodingTargetTreeOccupation{FilePaths: []string{occupied}})
		if err != nil || !sameExactStrings(target.Paths, []string{"src/feature002.rs", "src/feature002_test.rs"}) {
			t.Fatalf("occupied %s did not reserve a fresh complete pair: %v %v", occupied, target.Paths, err)
		}
	}
}

func rustAcceptanceFixture(t *testing.T, program *directCodingProgram) (assemblyline.SourceBlockRef, assemblyline.FragmentGenerationInput) {
	t.Helper()
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskVerification {
				continue
			}
			ref := assemblyline.SourceBlockRef{Document: document, Block: block}
			input, err := directCodingLanguageFragmentInput(program, ref, "rust")
			if err != nil {
				t.Fatal(err)
			}
			return ref, input
		}
	}
	t.Fatal("Rust task has no behavioral verification declaration")
	return assemblyline.SourceBlockRef{}, assemblyline.FragmentGenerationInput{}
}
