package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenerateBlockCoreKeepsInitialProjectionFailuresTerminal(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		input       assemblyline.FragmentGenerationInput
		raw         string
		failurePart string
		project     directCodingLanguageFragmentProjector
		validate    directCodingLanguageFragmentValidator
	}{
		{
			name: "go malformed declaration",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature: "func SumReadings(values []int) int",
				Behavior:  "Return the sum of all readings.",
			},
			raw: `func SumReadings(values []int) int { return`, failurePart: "parse Go fragment",
			project: projectDirectCodingGoFragment, validate: validateDirectCodingGoFragment,
		},
		{
			name: "javascript extra declaration",
			input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
				Signature: "function selectReading(values)",
				Behavior:  "Return one selected reading.",
			},
			raw: `function selectReading(values) { return values[0]; }
function auditReading(value) { return value; }`,
			failurePart: "exactly one top-level declaration",
			project:     assemblyline.ProjectJavaScriptFragment,
			validate:    validateDirectCodingJavaScriptFragment,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			block := assemblyline.SourceBlock{
				ID: "feature.reading", Signature: fixture.input.Signature,
				API: fixture.input.Signature, TaskID: "task_001",
				Role: assemblyline.SourceBlockTaskImplementation,
			}
			document := assemblyline.SourceDocument{
				ID: "reading", AdapterID: fixture.input.Language,
				Blocks: []assemblyline.SourceBlock{block},
			}
			stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
				Documents: []assemblyline.SourceDocument{document},
			}}
			executor := &directCodingLanguageProjectStageExecutor{
				config: directCodingLanguageStageConfig{
					Language: fixture.input.Language, ProjectFragment: fixture.project,
				},
				repairAttempts: make(map[string]int),
				repairGuidance: make(map[string]map[string]struct{}),
				repairSources:  make(map[string]map[string]struct{}),
			}
			generationCalls := 0
			validationCalls := 0
			finalizationCalls := 0
			var finalizedFailure error
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
					generationCalls++
					return fixture.raw, nil
				}),
				Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
					finalizationCalls++
					finalizedFailure = validationErr
					return nil
				},
			}
			modelResolutions := 0
			_, err := executor.generateBlockWithRuntime(
				runtime, "initial", func() (string, string, error) {
					modelResolutions++
					return "guidance", "executor", nil
				}, stage, assemblyline.SourceBlockRef{Document: document, Block: block},
				fixture.input,
				func(input assemblyline.FragmentGenerationInput, candidate string) (string, error) {
					validationCalls++
					return fixture.validate(input, candidate)
				},
			)
			if err == nil || !strings.Contains(err.Error(), fixture.failurePart) {
				t.Fatalf("projection failure=%v", err)
			}
			var repairable *directCodingLanguageFragmentRejection
			if errors.As(err, &repairable) || errors.As(finalizedFailure, &repairable) {
				t.Fatalf("projection failure became repairable: returned=%v finalized=%v", err, finalizedFailure)
			}
			if generationCalls != 1 || validationCalls != 0 || finalizationCalls != 1 ||
				modelResolutions != 0 || finalizedFailure == nil {
				t.Fatalf(
					"generation=%d validation=%d finalization=%d model_resolution=%d finalized=%v",
					generationCalls, validationCalls, finalizationCalls, modelResolutions, finalizedFailure,
				)
			}
		})
	}
}

func TestGenerateBlockCoreDoesNotExposeVerificationSourceToRepair(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
		Signature: "function verifyToken(input, dependencies)",
		Behavior:  "Verify one token.",
	}
	block := assemblyline.SourceBlock{
		ID: "verify.token", Signature: input.Signature, API: input.Signature,
		TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
	}
	document := assemblyline.SourceDocument{
		ID: "verify", AdapterID: "javascript", Blocks: []assemblyline.SourceBlock{block},
	}
	stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
		Documents: []assemblyline.SourceDocument{document},
	}}
	executor := &directCodingLanguageProjectStageExecutor{
		config: directCodingLanguageStageConfig{
			ProjectFragment: assemblyline.ProjectJavaScriptFragment,
		},
		repairAttempts: make(map[string]int),
		repairGuidance: make(map[string]map[string]struct{}),
		repairSources:  make(map[string]map[string]struct{}),
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			calls++
			return `function verifyToken(input, dependencies) { return hiddenVerify(input); }`, nil
		}),
	}
	modelResolutions := 0
	_, err := executor.generateBlockWithRuntime(
		runtime, "initial", func() (string, string, error) {
			modelResolutions++
			return "guidance", "executor", nil
		}, stage, assemblyline.SourceBlockRef{Document: document, Block: block},
		input, validateDirectCodingJavaScriptFragment,
	)
	if err == nil || !strings.Contains(err.Error(), "undeclared direct symbol hiddenVerify") {
		t.Fatalf("verification rejection=%v", err)
	}
	if calls != 1 || modelResolutions != 0 {
		t.Fatalf("verification source escaped into repair: calls=%d model_resolutions=%d", calls, modelResolutions)
	}
}

func TestGenerateBlockRoutesInitialRejectionWithoutStageFailureMapperGate(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_language_stage.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func (executor *directCodingLanguageProjectStageExecutor) GenerateBlock(")
	end := strings.Index(source, "func (executor *directCodingLanguageProjectStageExecutor) VerifyTask(")
	if start < 0 || end <= start {
		t.Fatal("GenerateBlock source boundary is absent")
	}
	generation := source[start:end]
	if !strings.Contains(generation, "return executor.generateBlockWithRuntime(") {
		t.Fatal("GenerateBlock omitted initial repair wiring")
	}
	for _, forbidden := range []string{"stageFailureEnabled", "MapStageFailure", "config.Repair"} {
		if strings.Contains(generation, forbidden) {
			t.Fatalf("GenerateBlock gates initial parser repair on staged-command mapping %q", forbidden)
		}
	}
	coreStart := strings.Index(source, "func (executor *directCodingLanguageProjectStageExecutor) generateBlockWithRuntime(")
	if coreStart < 0 || coreStart >= end {
		t.Fatal("GenerateBlock production core is absent")
	}
	core := source[coreStart:end]
	for _, required := range []string{
		"errors.As(err, &rejection)", "directCodingLanguageParserRepairDiagnostic(",
		"return executor.repairLanguageBlockWithRuntime(",
	} {
		if !strings.Contains(core, required) {
			t.Fatalf("GenerateBlock production core omitted initial repair wiring %q", required)
		}
	}
	for _, forbidden := range []string{"stageFailureEnabled", "MapStageFailure", "config.Repair"} {
		if strings.Contains(core, forbidden) {
			t.Fatalf("GenerateBlock production core gates parser repair on staged-command mapping %q", forbidden)
		}
	}
}
