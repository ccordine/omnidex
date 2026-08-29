package worker

import (
	"errors"
	"fmt"
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
		"php",
		func(candidate string) (string, error) {
			t.Fatal("unchanged source reached the parser")
			return candidate, nil
		},
	)
	if !errors.Is(err, errDirectCodingLanguageCorrectionUnchanged) {
		t.Fatalf("unchanged correction error=%v", err)
	}
}

func TestLanguageRepairExecutorRejectsProjectedSourceOutsideOneDeclaration(t *testing.T) {
	t.Parallel()
	const current = "func Value() int { return hidden() }"
	const raw = "import \"example.invalid/value\"\n\nfunc Value() int { return 2 }"
	finalized := false
	runtime := typedWorkerRuntime{
		Context: t.Context(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
		Finalize: func(
			_ assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if result.Candidate != raw || result.Projection != nil || validationErr == nil ||
				!errors.Is(validationErr, errDirectCodingLanguageCorrectionInvalid) {
				t.Fatalf("result=%+v validation=%v", result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	_, err := runDirectCodingLanguageCorrection(
		runtime, "executor", "opaque", current,
		"Change the declaration.", "go",
		func(candidate string) (string, error) {
			t.Fatalf("out-of-bound source reached validator: %q", candidate)
			return "", nil
		},
	)
	if !errors.Is(err, errDirectCodingLanguageCorrectionInvalid) ||
		!strings.Contains(err.Error(), "exactly one declaration") || !finalized {
		t.Fatalf("invalid correction error=%v finalized=%t", err, finalized)
	}
}

func TestLanguageRepairRequiresRegisteredCorrectionProjectionIdentity(t *testing.T) {
	t.Parallel()
	_, err := runDirectCodingLanguageCorrection(
		typedWorkerRuntime{Context: t.Context(), Execute: func(
			assemblyline.PortableJob, string,
		) (assemblyline.PortableResult, error) {
			t.Fatal("unknown projection identity reached executor")
			return assemblyline.PortableResult{}, nil
		}},
		"executor", "opaque", "func Value() int { return 1 }", "Change it.", "unknown",
		func(candidate string) (string, error) { return candidate, nil },
	)
	if err == nil || !strings.Contains(err.Error(), "source projection") {
		t.Fatalf("unknown projection identity error=%v", err)
	}
}

func TestLanguageRepairFailsInvalidCorrectionWithoutManufacturingAnotherGuidanceJob(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		language   string
		dialect    string
		signature  string
		current    string
		invalid    string
		validate   directCodingLanguageFragmentValidator
		diagnostic string
	}{
		{
			name: "go import plus function", language: "go", dialect: "Go 1.24 function syntax",
			signature: "func Normalize(value int) int",
			current:   "func Normalize(value int) int { return hidden(value) }",
			invalid: "import \"example.invalid/hidden\"\n\n" +
				"func Normalize(value int) int { return value }",
			validate:   validateDirectCodingGoFragment,
			diagnostic: `SOURCE_DIAGNOSTIC: Go fragment references undeclared capability "hidden"`,
		},
		{
			name: "javascript sibling declaration", language: "javascript",
			dialect:   "ECMAScript 2022 function syntax",
			signature: "function normalize(value)",
			current:   "function normalize(value) { return hidden(value); }",
			invalid: "function normalize(value) { return value; }\n" +
				"function sibling(value) { return value; }",
			validate:   validateDirectCodingJavaScriptFragment,
			diagnostic: "SOURCE_DIAGNOSTIC: JavaScript fragment references undeclared direct symbol hidden",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			input := assemblyline.FragmentGenerationInput{
				Language: fixture.language, Dialect: fixture.dialect,
				Signature: fixture.signature, Behavior: "Normalize one value.",
			}
			block := assemblyline.SourceBlock{
				ID: "feature.normalize", Signature: fixture.signature, API: fixture.signature,
				TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
			}
			document := assemblyline.SourceDocument{
				ID: "feature", AdapterID: fixture.language,
				Blocks: []assemblyline.SourceBlock{block},
			}
			stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
				Documents: []assemblyline.SourceDocument{document},
			}}
			executor := &directCodingLanguageProjectStageExecutor{
				config:                    directCodingLanguageStageConfig{Language: fixture.language},
				acceptedRepairTransitions: make(map[string]int),
				repairDiagnostics:         make(map[string]map[string]struct{}),
			}
			guidanceCalls := 0
			correctionCalls := 0
			runtime := typedWorkerRuntime{
				Context: t.Context(), MaxAttempts: exactSemanticLeafCalls,
				Execute: testPortableExecutor(func(
					scope string, model string, prompt string) (string, error) {
					switch scope {
					case "portable_semantic_worker":
						guidanceCalls++
						if model != "guidance" {
							t.Fatalf("guidance model=%s", model)
						}
						if strings.Contains(prompt, "REJECTED_INSTRUCTION_JSON:") ||
							strings.Contains(prompt, "EXACT_INSTRUCTION_FAILURE:") {
							t.Fatalf("guidance prompt contains manufactured rejection history:\n%s", prompt)
						}
						return "Change the exact mutable declaration using only its local parameter.", nil
					case "portable_fragment_worker":
						correctionCalls++
						if model != "executor" {
							t.Fatalf("correction model=%s", model)
						}
						return fixture.invalid, nil
					default:
						t.Fatalf("unexpected scope %q", scope)
						return "", nil
					}
				}),
			}
			_, err := executor.repairLanguageBlockWithRuntime(
				runtime, "guidance", "executor", stage,
				assemblyline.SourceBlockRef{Document: document, Block: block},
				input, fixture.current, fixture.diagnostic, fixture.validate,
			)
			if !errors.Is(err, errDirectCodingLanguageCorrectionInvalid) ||
				guidanceCalls != 1 || correctionCalls != 1 ||
				executor.acceptedRepairTransitions[block.ID] != 0 {
				t.Fatalf(
					"error=%v guidance=%d correction=%d repairs=%d",
					err, guidanceCalls, correctionCalls,
					executor.acceptedRepairTransitions[block.ID],
				)
			}
		})
	}
}

func TestLanguageRepairRecordsZeroDeltaWithoutManufacturingAnotherGuidanceJob(t *testing.T) {
	t.Parallel()
	const current = "func Normalize(value int) int { return value }"
	block := assemblyline.SourceBlock{
		ID: "feature.normalize", Signature: "func Normalize(value int) int",
		API: "func Normalize(value int) int", TaskID: "task_001",
		Role: assemblyline.SourceBlockTaskImplementation,
	}
	document := assemblyline.SourceDocument{
		ID: "feature", AdapterID: "go", Blocks: []assemblyline.SourceBlock{block},
	}
	stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
		Documents: []assemblyline.SourceDocument{document},
	}}
	executor := &directCodingLanguageProjectStageExecutor{
		config:                    directCodingLanguageStageConfig{Language: "go"},
		acceptedRepairTransitions: make(map[string]int),
		repairDiagnostics:         make(map[string]map[string]struct{}),
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: t.Context(), MaxAttempts: exactSemanticLeafCalls,
		Execute: testPortableExecutor(func(scope string, _ string, _ string) (string, error) {
			calls++
			switch scope {
			case "portable_semantic_worker":
				return "Return the local parameter without calling another declaration.", nil
			case "portable_fragment_worker":
				return current, nil
			default:
				return "", fmt.Errorf("unexpected scope %q", scope)
			}
		}),
	}
	_, err := executor.repairLanguageBlockWithRuntime(
		runtime, "guidance", "executor", stage,
		assemblyline.SourceBlockRef{Document: document, Block: block},
		assemblyline.FragmentGenerationInput{
			Language: "go", Dialect: "Go 1.24 function syntax",
			Signature: block.Signature, Behavior: "Normalize one value.",
		},
		current, "SOURCE_DIAGNOSTIC: exact compiler failure", validateDirectCodingGoFragment,
	)
	if !errors.Is(err, errDirectCodingLanguageCorrectionUnchanged) || calls != 2 ||
		executor.acceptedRepairTransitions[block.ID] != 0 {
		t.Fatalf(
			"zero delta error=%v calls=%d accepted transitions=%d",
			err, calls, executor.acceptedRepairTransitions[block.ID],
		)
	}
}

func TestLanguageRepairRequiresDistinctCompilerDiagnosticAfterAcceptedTransition(t *testing.T) {
	t.Parallel()
	const (
		initial    = "func Normalize(value int) int { return missing(value) }"
		corrected  = "func Normalize(value int) int { return value }"
		diagnostic = `SOURCE_DIAGNOSTIC: Go fragment references undeclared capability "missing"`
	)
	block := assemblyline.SourceBlock{
		ID: "feature.normalize", Signature: "func Normalize(value int) int",
		API: "func Normalize(value int) int", TaskID: "task_001",
		Role: assemblyline.SourceBlockTaskImplementation,
	}
	document := assemblyline.SourceDocument{
		ID: "feature", AdapterID: "go", Blocks: []assemblyline.SourceBlock{block},
	}
	stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
		Documents: []assemblyline.SourceDocument{document},
	}}
	executor := &directCodingLanguageProjectStageExecutor{
		config:                    directCodingLanguageStageConfig{Language: "go"},
		acceptedRepairTransitions: make(map[string]int),
		repairDiagnostics:         make(map[string]map[string]struct{}),
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: t.Context(),
		Execute: testPortableExecutor(func(scope string, _ string, _ string) (string, error) {
			calls++
			switch scope {
			case "portable_semantic_worker":
				return "Return the declared parameter directly.", nil
			case "portable_fragment_worker":
				return corrected, nil
			default:
				return "", fmt.Errorf("unexpected scope %q", scope)
			}
		}),
	}
	generation := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24 function syntax",
		Signature: block.Signature, Behavior: "Normalize one value.",
	}
	ref := assemblyline.SourceBlockRef{Document: document, Block: block}
	got, err := executor.repairLanguageBlockWithRuntime(
		runtime, "guidance", "executor", stage, ref,
		generation, initial, diagnostic, validateDirectCodingGoFragment,
	)
	if err != nil || got != corrected || calls != 2 ||
		executor.acceptedRepairTransitions[block.ID] != 1 {
		t.Fatalf("accepted source=%q error=%v calls=%d", got, err, calls)
	}
	_, err = executor.repairLanguageBlockWithRuntime(
		runtime, "guidance", "executor", stage, ref,
		generation, corrected, diagnostic, validateDirectCodingGoFragment,
	)
	if err == nil || !strings.Contains(err.Error(), "no distinct verified failure") || calls != 2 {
		t.Fatalf("repeated diagnostic error=%v calls=%d", err, calls)
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
		"ProjectTrimmedSourceDeclarationResponse",
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
