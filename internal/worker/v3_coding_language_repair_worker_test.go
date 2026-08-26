package worker

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLanguageRepairExecutorRejectsUnchangedSource(t *testing.T) {
	t.Parallel()
	const current = "function value(array $items): array { return $items; }"
	runtime := typedWorkerRuntime{
		Context: t.Context(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: current}, nil
		},
	}
	_, err := runDirectCodingLanguageCorrection(
		runtime, "executor", "opaque", current,
		"Return a changed declaration.",
		func(candidate string) (string, error) {
			t.Fatal("unchanged source reached the parser")
			return candidate, nil
		},
	)
	if !errors.Is(err, errDirectCodingLanguageCorrectionUnchanged) {
		t.Fatalf("unchanged correction error=%v", err)
	}
}

func TestLanguageRepairSourceKeepsGuidanceAndExecutorEnvelopesDisjoint(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_language_repair_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func runDirectCodingLanguageCorrection(")
	end := strings.Index(source, "func failDirectCodingLanguageCorrection(")
	if start < 0 || end <= start {
		t.Fatal("language correction source boundary is absent")
	}
	executorSource := source[start:end]
	for _, forbidden := range []string{
		"Diagnostic:", "RequiredChange:", "Language:", "Signature:",
		"Capabilities:", "PermittedSymbols:", "Behavior:", "command.Args", "Document.Path",
	} {
		if strings.Contains(executorSource, forbidden) {
			t.Fatalf("repair executor retained analyst authority %q", forbidden)
		}
	}

	raw, err = os.ReadFile("v3_coding_language_repair.go")
	if err != nil {
		t.Fatal(err)
	}
	guidanceSource := string(raw)
	for _, forbidden := range []string{
		"generation.Behavior", "command.Args", "Document.Path", "TaskID", "Contract",
	} {
		if strings.Contains(guidanceSource, forbidden) {
			t.Fatalf("repair guidance construction retained forbidden authority %q", forbidden)
		}
	}
}

func TestLanguageRepairRejectsRepeatedGuidanceAndSourceCycles(t *testing.T) {
	t.Parallel()
	executor := &directCodingLanguageProjectStageExecutor{
		repairGuidance: make(map[string]map[string]struct{}),
		repairSources:  make(map[string]map[string]struct{}),
	}
	if err := executor.acceptLanguageRepairGuidance("opaque", "Replace the returned expression."); err != nil {
		t.Fatal(err)
	}
	if err := executor.acceptLanguageRepairGuidance("opaque", "Replace the returned expression."); !errors.Is(err, errDirectCodingLanguageGuidanceRepeated) {
		t.Fatalf("repeated guidance error=%v", err)
	}
	if err := executor.acceptLanguageRepairSource("opaque", "function value() { return 2; }"); err != nil {
		t.Fatal(err)
	}
	if err := executor.acceptLanguageRepairSource("opaque", "function value() { return 2; }"); err == nil {
		t.Fatal("repeated corrected source was accepted")
	}
}

func TestLanguageRepairSplicesOnlyTheExactGeneratedBlock(t *testing.T) {
	t.Parallel()
	program := directCodingProgram{Generated: map[string]string{
		"feature.101": "function feature101() { return 1; }",
		"feature.702": "function feature702() { return 7; }",
	}}
	if err := applyDirectCodingLanguageRepair(
		&program, "feature.101", "function feature101() { return 1; }",
		"function feature101() { return 2; }",
	); err != nil {
		t.Fatal(err)
	}
	if program.Generated["feature.101"] != "function feature101() { return 2; }" ||
		program.Generated["feature.702"] != "function feature702() { return 7; }" ||
		len(program.Generated) != 2 {
		t.Fatalf("generated source changed outside exact block: %#v", program.Generated)
	}
	if err := applyDirectCodingLanguageRepair(
		&program, "feature.101", "function feature101() { return 1; }",
		"function feature101() { return 3; }",
	); err == nil {
		t.Fatal("stale repair authority was accepted")
	}
}
