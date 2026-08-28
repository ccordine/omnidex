package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenerateBlockCoreRepairsInitialParserRejectionWithoutStageFailureMapper(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		input       assemblyline.FragmentGenerationInput
		rawInitial  string
		corrected   string
		project     directCodingLanguageFragmentProjector
		validate    directCodingLanguageFragmentValidator
		failurePart string
	}{
		{
			name: "go",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature: "func CanonicalRune(input rune) rune",
				Behavior:  "Return one canonical rune.",
			},
			rawInitial:  "func CanonicalRune(input rune) rune { return unicode.ToUpper(input) }",
			corrected:   `func CanonicalRune(input rune) rune { return input }`,
			project:     projectDirectCodingGoFragment,
			validate:    validateDirectCodingGoFragment,
			failurePart: `undeclared capabilities ["ToUpper" "unicode"]`,
		},
		{
			name: "javascript",
			input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
				Signature: "function normalizeToken(input, dependencies)",
				Behavior:  "Return one normalized token.",
			},
			rawInitial:  "function normalizeToken(input, dependencies) {\r\n  return hiddenNormalize(input);\r\n}",
			corrected:   `function normalizeToken(input, dependencies) { return input; }`,
			project:     assemblyline.ProjectJavaScriptFragment,
			validate:    validateDirectCodingJavaScriptFragment,
			failurePart: "undeclared direct symbol hiddenNormalize",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			projection, err := fixture.project(fixture.rawInitial)
			if err != nil {
				t.Fatal(err)
			}
			projected := projection.Source
			if projected != fixture.rawInitial[projection.StartByte:projection.EndByte] {
				t.Fatal("projected declaration is not its exact response span")
			}
			block := assemblyline.SourceBlock{
				ID: "feature.normalize", Signature: fixture.input.Signature,
				API: fixture.input.Signature, TaskID: "task_001",
				Role: assemblyline.SourceBlockTaskImplementation,
			}
			document := assemblyline.SourceDocument{
				ID: "feature", AdapterID: fixture.input.Language,
				Blocks: []assemblyline.SourceBlock{block},
			}
			stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
				Documents: []assemblyline.SourceDocument{document},
			}}
			executor := &directCodingLanguageProjectStageExecutor{
				config:                    directCodingLanguageStageConfig{Language: fixture.input.Language},
				acceptedRepairTransitions: make(map[string]int),
				repairGuidance:            make(map[string]map[string]struct{}),
				repairSources:             make(map[string]map[string]struct{}),
			}
			calls := make([]string, 0, 3)
			guidanceAttempts := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Emit: func(event typedWorkerEvent) {
					if event.Kind == typedWorkerSemantic && event.State == typedWorkerStarted {
						guidanceAttempts = event.MaxAttempts
					}
				},
				Execute: testPortableExecutor(func(
					scope string, model string, prompt string) (string, error) {
					calls = append(calls, scope+":"+model)
					switch scope {
					case "portable_semantic_worker":
						if model != "guidance" || !strings.Contains(prompt, "SOURCE_DIAGNOSTIC:") ||
							!strings.Contains(prompt, fixture.failurePart) {
							t.Fatalf("repair-guidance envelope lost exact initial rejection:\n%s", prompt)
						}
						if current := exactPromptJSONString(
							t, prompt, "EXACT_MUTABLE_DECLARATION_JSON:\n",
						); current != projected {
							t.Fatalf("repair guidance current=%q projected=%q raw=%q", current, projected, fixture.rawInitial)
						}
						return "Replace the undeclared external helper with a direct expression using only the declared parameter.", nil
					case "portable_fragment_worker":
						if len(calls) == 1 {
							if model != "initial" || !strings.Contains(prompt, fixture.input.Behavior) {
								t.Fatalf("initial fragment envelope=%s model=%s", prompt, model)
							}
							return fixture.rawInitial, nil
						}
						if model != "executor" || strings.Contains(prompt, "SOURCE_DIAGNOSTIC:") ||
							strings.Contains(prompt, fixture.input.Behavior) {
							t.Fatalf("repair-executor authority is not instruction plus mutable source:\n%s", prompt)
						}
						if current := exactPromptJSONString(
							t, prompt, "EXACT_MUTABLE_SOURCE_JSON:\n",
						); current != projected {
							t.Fatalf("repair executor current=%q projected=%q raw=%q", current, projected, fixture.rawInitial)
						}
						return fixture.corrected, nil
					default:
						t.Fatalf("unexpected repair scope %q", scope)
						return "", nil
					}
				}),
			}
			modelResolutions := 0
			got, err := executor.generateBlockWithRuntime(
				runtime, "initial", func() (string, string, error) {
					modelResolutions++
					return "guidance", "executor", nil
				}, stage,
				assemblyline.SourceBlockRef{Document: document, Block: block},
				fixture.input, fixture.validate,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = fixture.validate(fixture.input, fixture.corrected)
			if err != nil {
				t.Fatal(err)
			}
			want := fixture.corrected
			if got != want || modelResolutions != 1 || guidanceAttempts != maxTypedWorkerAttempts ||
				len(calls) != 3 ||
				calls[0] != "portable_fragment_worker:initial" ||
				calls[1] != "portable_semantic_worker:guidance" ||
				calls[2] != "portable_fragment_worker:executor" {
				t.Fatalf("repaired=%q want=%q calls=%v", got, want, calls)
			}
			if executor.config.Repair.stageFailureEnabled() {
				t.Fatal("fixture accidentally enabled staged-command failure mapping")
			}
		})
	}
}

func exactPromptJSONString(t *testing.T, prompt string, marker string) string {
	t.Helper()
	start := strings.Index(prompt, marker)
	if start < 0 {
		t.Fatalf("prompt omitted %q:\n%s", marker, prompt)
	}
	encoded := prompt[start+len(marker):]
	if end := strings.Index(encoded, "\n\n"); end >= 0 {
		encoded = encoded[:end]
	}
	var decoded string
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("decode prompt value %q: %v", encoded, err)
	}
	return decoded
}
