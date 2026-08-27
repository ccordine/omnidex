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
	const raw = "```go\nimport \"example.invalid/value\"\n\nfunc Value() int { return 2 }\n```"
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

func TestLanguageRepairRetriesInvalidCorrectionWithinBoundedAuthority(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		language   string
		dialect    string
		signature  string
		current    string
		invalid    string
		corrected  string
		validate   directCodingLanguageFragmentValidator
		diagnostic string
	}{
		{
			name: "go import plus function", language: "go", dialect: "Go 1.24 function syntax",
			signature: "func Normalize(value int) int",
			current:   "func Normalize(value int) int { return hidden(value) }",
			invalid: "```go\nimport \"example.invalid/hidden\"\n\n" +
				"func Normalize(value int) int { return value }\n```",
			corrected:  "func Normalize(value int) int { return value }",
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
			corrected:  "function normalize(value) { return value; }",
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
				repairGuidance:            make(map[string]map[string]struct{}),
				repairSources:             make(map[string]map[string]struct{}),
			}
			guidanceCalls := 0
			correctionCalls := 0
			runtime := typedWorkerRuntime{
				Context: t.Context(), MaxAttempts: maxTypedWorkerAttempts,
				Execute: testPortableExecutor(func(
					scope string, model string, prompt string, _ map[string]any,
				) (string, error) {
					switch scope {
					case "portable_semantic_worker":
						guidanceCalls++
						if model != "guidance" {
							t.Fatalf("guidance model=%s", model)
						}
						if guidanceCalls == 2 &&
							(!strings.Contains(prompt, "outside the code-owned declaration") ||
								!strings.Contains(prompt, "REJECTED_INSTRUCTION_JSON:")) {
							t.Fatalf("second guidance omitted invalid-source evidence:\n%s", prompt)
						}
						return `{"instruction":"Change the exact mutable declaration using only its local parameter, attempt ` +
							string(rune('0'+guidanceCalls)) + `."}`, nil
					case "portable_fragment_worker":
						correctionCalls++
						if model != "executor" {
							t.Fatalf("correction model=%s", model)
						}
						if correctionCalls == 1 {
							return fixture.invalid, nil
						}
						return fixture.corrected, nil
					default:
						t.Fatalf("unexpected scope %q", scope)
						return "", nil
					}
				}),
			}
			got, err := executor.repairLanguageBlockWithRuntime(
				runtime, "guidance", "executor", stage,
				assemblyline.SourceBlockRef{Document: document, Block: block},
				input, fixture.current, fixture.diagnostic, fixture.validate,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.corrected || guidanceCalls != 2 || correctionCalls != 2 ||
				executor.acceptedRepairTransitions[block.ID] != 1 {
				t.Fatalf(
					"source=%q guidance=%d correction=%d repairs=%d",
					got, guidanceCalls, correctionCalls,
					executor.acceptedRepairTransitions[block.ID],
				)
			}
		})
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
