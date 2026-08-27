package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptGenerateBlockCoreRepairsInitialParserRejection(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name string
		tsx  bool
	}{
		{name: "typescript"},
		{name: "tsx", tsx: true},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			const invalid = `function NormalizeToken(input: number): string { return input as unknown as string; }`
			const corrected = `function NormalizeToken(input: string): string { return input; }`
			const instruction = "Replace the parameter type with string and return that declared input parameter."
			job := directCodingTypeScriptFragmentJob{
				block: assemblyline.SourceBlock{
					ID: "feature.normalize", Signature: "function NormalizeToken(input: string): string",
					API:      "function NormalizeToken(input: string): string",
					Contract: "Return the normalized token.", TaskID: "task_001",
					Role: assemblyline.SourceBlockTaskImplementation,
				},
				dialect: "TypeScript 5.9.3 function syntax", tsx: fixture.tsx,
			}
			calls := make([]string, 0, 3)
			guidanceAttempts := 0
			initialRejectionFinalized := false
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Finalize: func(
					portable assemblyline.PortableJob,
					_ assemblyline.PortableResult,
					validationErr error,
				) error {
					if portable.Kind != assemblyline.WorkFragmentGeneration || validationErr == nil {
						return nil
					}
					var rejection *directCodingTypeScriptInitialFragmentRejection
					if !errors.As(validationErr, &rejection) || rejection.Candidate != invalid ||
						rejection.Failure == nil ||
						!strings.Contains(rejection.Failure.Error(), "does not match required signature") {
						t.Fatalf("finalized initial rejection=%#v error=%v", rejection, validationErr)
					}
					initialRejectionFinalized = true
					return nil
				},
				Emit: func(event typedWorkerEvent) {
					if event.Kind == typedWorkerSemantic && event.State == typedWorkerStarted {
						guidanceAttempts = event.MaxAttempts
					}
				},
				Execute: testPortableExecutor(func(
					scope string, model string, prompt string, _ map[string]any,
				) (string, error) {
					calls = append(calls, scope+":"+model)
					switch len(calls) {
					case 1:
						if scope != "portable_fragment_worker" || model != "initial" ||
							!strings.Contains(prompt, job.block.Contract) {
							t.Fatalf("initial TypeScript envelope model=%s scope=%s:\n%s", model, scope, prompt)
						}
						return invalid, nil
					case 2:
						if scope != "portable_semantic_worker" || model != "guidance" ||
							!strings.Contains(prompt, invalid) ||
							!strings.Contains(prompt, "SOURCE_DIAGNOSTIC:") ||
							!strings.Contains(prompt, "does not match required signature") {
							t.Fatalf("TypeScript guidance lost exact rejection authority:\n%s", prompt)
						}
						return `{"instruction":"` + instruction + `"}`, nil
					case 3:
						for _, forbidden := range []string{
							"SOURCE_DIAGNOSTIC:", "EXACT_SIGNATURE:",
							"REQUIRED_DECLARATION_SIGNATURE:", job.block.Contract,
						} {
							if strings.Contains(prompt, forbidden) {
								t.Fatalf("TypeScript executor leaked %q:\n%s", forbidden, prompt)
							}
						}
						if scope != "portable_fragment_worker" || model != "correction" ||
							!strings.Contains(prompt, invalid) || !strings.Contains(prompt, instruction) {
							t.Fatalf("TypeScript executor lost instruction or mutable source:\n%s", prompt)
						}
						return corrected, nil
					default:
						t.Fatalf("unexpected TypeScript repair call %d", len(calls))
						return "", nil
					}
				}),
			}
			modelResolutions := 0
			got, err := generateDirectCodingTypeScriptBlockWithRuntime(
				runtime, "initial", func() (string, string, error) {
					modelResolutions++
					return "guidance", "correction", nil
				}, directCodingTypeScriptRepairEvents{}, job,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got != corrected || modelResolutions != 1 ||
				guidanceAttempts != maxTypedWorkerAttempts || !initialRejectionFinalized ||
				len(calls) != 3 {
				t.Fatalf(
					"corrected=%q model_resolutions=%d guidance_attempts=%d calls=%v",
					got, modelResolutions, guidanceAttempts, calls,
				)
			}
		})
	}
}

func TestTypeScriptInitialProjectionFailureRemainsTerminal(t *testing.T) {
	t.Parallel()
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.SourceBlock{
			ID: "feature.normalize", Signature: "function NormalizeToken(input: string): string",
			API: "function NormalizeToken(input: string): string", Contract: "Return the token.",
			TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
		},
		dialect: "TypeScript 5.9.3 function syntax",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			calls++
			return `export function NormalizeToken(input: string): string { return input; }`, nil
		}),
	}
	modelResolutions := 0
	_, err := generateDirectCodingTypeScriptBlockWithRuntime(
		runtime, "initial", func() (string, string, error) {
			modelResolutions++
			return "guidance", "correction", nil
		}, directCodingTypeScriptRepairEvents{}, job,
	)
	if err == nil || !strings.Contains(err.Error(), "wrapped in extra export authority") {
		t.Fatalf("projection rejection=%v", err)
	}
	if calls != 1 || modelResolutions != 0 {
		t.Fatalf("projection authority escaped into repair: calls=%d model_resolutions=%d", calls, modelResolutions)
	}
}

func TestTypeScriptInitialRepairDoesNotExposeVerificationSource(t *testing.T) {
	t.Parallel()
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.SourceBlock{
			ID: "verify.normalize", Signature: "function VerifyNormalize(): void",
			API: "function VerifyNormalize(): void", Contract: "Verify normalization.",
			TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
		},
		dialect: "TypeScript 5.9.3 function syntax",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			calls++
			return `function VerifyNormalize(value: string): void { return; }`, nil
		}),
	}
	modelResolutions := 0
	_, err := generateDirectCodingTypeScriptBlockWithRuntime(
		runtime, "initial", func() (string, string, error) {
			modelResolutions++
			return "guidance", "correction", nil
		}, directCodingTypeScriptRepairEvents{}, job,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match required signature") {
		t.Fatalf("verification rejection=%v", err)
	}
	if calls != 1 || modelResolutions != 0 {
		t.Fatalf("verification source escaped into repair: calls=%d model_resolutions=%d", calls, modelResolutions)
	}
}

func TestTypeScriptInitialRepairKeepsInvalidGuidedOutputTerminal(t *testing.T) {
	t.Parallel()
	const invalid = `function NormalizeToken(input: number): string { return input as unknown as string; }`
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.SourceBlock{
			ID: "feature.normalize", Signature: "function NormalizeToken(input: string): string",
			API: "function NormalizeToken(input: string): string", Contract: "Return the token.",
			TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
		},
		dialect: "TypeScript 5.9.3 function syntax",
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(scope string, _ string, _ string, _ map[string]any) (string, error) {
			calls++
			switch scope {
			case "portable_semantic_worker":
				return `{"instruction":"Replace the parameter type with string."}`, nil
			case "portable_fragment_worker":
				if calls == 1 {
					return invalid, nil
				}
				return `export function NormalizeToken(input: string): string { return input; }`, nil
			default:
				t.Fatalf("unexpected scope %q", scope)
				return "", nil
			}
		}),
	}
	_, err := generateDirectCodingTypeScriptBlockWithRuntime(
		runtime, "initial", func() (string, string, error) {
			return "guidance", "correction", nil
		}, directCodingTypeScriptRepairEvents{}, job,
	)
	if err == nil || !strings.Contains(err.Error(), "wrapped in extra export authority") {
		t.Fatalf("guided rejection=%v", err)
	}
	if calls != 3 {
		t.Fatalf("invalid guided output was retried: calls=%d", calls)
	}
}

func TestTypeScriptGenerateBlockUsesInitialRepairProductionCore(t *testing.T) {
	t.Parallel()
	executorSource, err := os.ReadFile("v3_coding_typescript_executor.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(executorSource), "generateDirectCodingApplicationTaskBlock(") {
		t.Fatal("TypeScript stage GenerateBlock does not use application-task generation")
	}
	runtimeSource, err := os.ReadFile("v3_application_task_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeSource), "return generateDirectCodingTypeScriptBlockWithRuntime(") {
		t.Fatal("TypeScript application-task generation bypasses initial repair production core")
	}
}
