package worker

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoAndUnguidedTypeScriptCorrectionEnvelopesAreAbsent(t *testing.T) {
	t.Parallel()
	for _, retired := range []string{
		"v3_repository_change_correction.go",
		"v3_repository_change_failure.go",
		"v3_repository_change_ownership.go",
		"../assemblyline/go_fragment_correction.go",
	} {
		if _, err := os.Stat(retired); !os.IsNotExist(err) {
			t.Fatalf("retired correction source %q still exists or cannot be checked: %v", retired, err)
		}
	}
	for _, source := range []string{
		"v3_go_fragment_generation.go",
		"v3_go_fragment_modification.go",
		"v3_repository_change_prepare.go",
		"v3_repository_change_commands.go",
		"v3_repository_change_generation.go",
		"v3_repository_desired_generation.go",
	} {
		raw, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"FragmentCorrection", "CorrectionModel", "CodingFragmentCorrection",
			"RequiredChange:", "Diagnostic:",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("Go source %s retained correction authority %q", source, forbidden)
			}
		}
	}
	typeScriptWorker, err := os.ReadFile("v3_coding_typescript_fragment_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"for attempt :=", "runtime.CorrectionModel", "RequiredChange:", "Diagnostic:",
	} {
		if strings.Contains(string(typeScriptWorker), forbidden) {
			t.Fatalf("TypeScript source worker retained unguided correction path %q", forbidden)
		}
	}
	renderer, err := os.ReadFile("../assemblyline/portable_job_source_render.go")
	if err != nil {
		t.Fatal(err)
	}
	rendererSource := string(renderer)
	start := strings.Index(rendererSource, "func renderPortableFragmentCorrection(")
	if start < 0 {
		t.Fatal("fragment correction renderer boundary is absent")
	}
	rendererSource = rendererSource[start:]
	for _, forbidden := range []string{
		"BuildGoFragmentCorrectionPrompt", "RequiredChange: input.RequiredChange",
		"Diagnostic: input.Diagnostic", "Available: strings.Join(input.Capabilities",
		"Globals: input.PermittedSymbols",
	} {
		if strings.Contains(rendererSource, forbidden) {
			t.Fatalf("fragment correction renderer retained combined authority %q", forbidden)
		}
	}
}

func TestFragmentCorrectionConstructorAdmitsOnlyGuidanceAndMutableSource(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]assemblyline.FragmentCorrectionInput{
		"unguided typescript": {
			Language: "typescript", Signature: "function value(): number",
			CurrentDeclaration: "function value(): number { return 1; }",
			RequiredChange:     "Return two.", Diagnostic: "expected two",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := assemblyline.NewFragmentCorrectionJob(input); err == nil {
				t.Fatalf("accepted retired %s correction envelope", name)
			}
		})
	}
	if _, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		CurrentDeclaration: "function value() { return 1; }",
		RepairGuidance:     "Replace the returned literal with two.",
	}); err == nil {
		t.Fatal("language-blind correction bypassed the source-projection identity")
	}
	job, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(
		assemblyline.FragmentCorrectionInput{
			CurrentDeclaration: "function value() { return 1; }",
			RepairGuidance:     "Replace the returned literal with two.",
		},
		"javascript",
	)
	if err != nil {
		t.Fatal(err)
	}
	var input assemblyline.FragmentCorrectionInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		t.Fatal(err)
	}
	if input.Diagnostic != "" || input.RequiredChange != "" ||
		len(input.Capabilities) != 0 || len(input.PermittedSymbols) != 0 {
		t.Fatalf("repair executor retained analyst authority: %+v", input)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"OBSERVED_FAILURE", "REQUIRED_CHANGE", "ONLY_AVAILABLE_DECLARATIONS",
		"ALREADY_IN_SCOPE_IDENTIFIERS",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("repair executor prompt retained analyst authority %q:\n%s", forbidden, prompt)
		}
	}
}
