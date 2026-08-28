package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const testPortableGrossResourceBytes = 128 * 1024

func TestTypeScriptWholeDeclarationCorrectionCrossesFormerLocalByteCeilings(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name       string
		minimumLen int
	}{
		{name: "above former five KiB declaration ceiling", minimumLen: 6 * 1024},
		{name: "above former portable and thirty-two KiB ceilings", minimumLen: 40 * 1024},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			const signature = "function revise(value: number): number"
			current := validLargeTypeScriptDeclaration(signature, fixture.minimumLen, "value")
			corrected := validLargeTypeScriptDeclaration(signature, fixture.minimumLen, "value + 1")
			if len(current) < fixture.minimumLen || len(corrected) < fixture.minimumLen {
				t.Fatalf("fixture current=%dB corrected=%dB want at least %dB", len(current), len(corrected), fixture.minimumLen)
			}
			if len(current) >= testPortableGrossResourceBytes || len(corrected) >= testPortableGrossResourceBytes {
				t.Fatalf("fixture escaped gross resource boundary: current=%dB corrected=%dB", len(current), len(corrected))
			}

			job := directCodingTypeScriptFragmentJob{
				block: assemblyline.SourceBlock{
					ID: "calculation.revise", Signature: signature,
					Contract: "Return the corrected numeric result.", API: signature,
				},
				current: current, repairGuidance: "Add one to the returned value.",
			}
			var promptBytes int
			var payloadBytes int
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					payloadBytes = len(portable.Payload)
					prompt, err := assemblyline.RenderPortableJob(portable)
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					promptBytes = len(prompt)
					contract, err := llmResponseContractForPortableJob(portable)
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					if err := validateExactStationStaticCall(
						prompt, contract,
						llm.ProviderIdentitySelection{Model: "test-model", NativeContextLimit: 8192},
					); err != nil {
						return assemblyline.PortableResult{}, err
					}
					return assemblyline.PortableResult{JobID: portable.ID, Candidate: corrected}, nil
				},
			}
			source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "executor", job)
			if err != nil {
				t.Fatalf("valid %dB whole-declaration correction was rejected: %v", len(corrected), err)
			}
			if strings.TrimSpace(source) != strings.TrimSpace(corrected) {
				t.Fatal("worker changed the accepted correction")
			}
			if payloadBytes <= 0 || payloadBytes >= testPortableGrossResourceBytes ||
				promptBytes <= 0 || promptBytes >= testPortableGrossResourceBytes {
				t.Fatalf("composed correction payload=%dB prompt=%dB escaped the shared gross boundary", payloadBytes, promptBytes)
			}
		})
	}
}

func TestTypeScriptWorkerAcceptsOnlyExactRawOutputBeyondFormerPortableCandidateCeiling(t *testing.T) {
	t.Parallel()

	const signature = "function revise(value: number): number"
	source := validLargeTypeScriptDeclaration(
		signature, testPortableGrossResourceBytes+1, "value + 1",
	)
	raw := source
	if len(raw) <= testPortableGrossResourceBytes {
		t.Fatalf("raw fixture=%dB did not cross former candidate ceiling", len(raw))
	}
	job := directCodingTypeScriptFragmentJob{dialect: "TypeScript 5.9.3", block: assemblyline.SourceBlock{
		ID: "calculation.revise", Signature: signature,
		Contract: "Return the corrected numeric result.", API: signature,
	}}
	executions := 0
	var events []typedWorkerEvent
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string) (string, error) {
			executions++
			return raw, nil
		}),
		Emit: func(event typedWorkerEvent) { events = append(events, event) },
	}
	got, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err != nil {
		t.Fatalf("accept exact raw output beyond former candidate ceiling: %v", err)
	}
	if got != source {
		t.Fatalf("projected source=%q want=%q", got, source)
	}
	if executions != 1 {
		t.Fatalf("raw model result dispatched %d calls, want 1", executions)
	}
	foundSizeWarning := false
	for _, event := range events {
		if event.State == typedWorkerCompleted &&
			strings.Contains(event.Warning, "declaration_size_review") &&
			!strings.Contains(event.Warning, "output_projection") {
			foundSizeWarning = true
		}
	}
	if !foundSizeWarning {
		t.Fatalf("completion events omitted exact declaration size metadata: %+v", events)
	}

	oversized := validLargeTypeScriptDeclaration(
		signature, testPortableGrossResourceBytes+1, "value + 1",
	)

	job.current = oversized
	job.repairGuidance = "Add one to the returned value."
	executions = 0
	_, err = runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err == nil || !strings.Contains(err.Error(), "portable job payload exceeds gross resource ceiling of 131072 bytes") {
		t.Fatalf("oversized current declaration error=%v", err)
	}
	if executions != 0 {
		t.Fatalf("oversized current declaration dispatched %d model calls, want 0", executions)
	}
}

func TestTypeScriptWorkerStopsRepeatedUnprojectableResponseState(t *testing.T) {
	t.Parallel()

	job := directCodingTypeScriptFragmentJob{dialect: "TypeScript 5.9.3", block: assemblyline.SourceBlock{
		ID: "calculation.revise", Signature: "function revise(value: number): number",
		Contract: "Return the revised value.", API: "function revise(value: number): number",
	}}
	for name, raw := range map[string]string{
		"no executable node": "I would implement this after considering the types.",
		"Markdown fence":     "```typescript\nfunction revise(value: number): number { return value; }\n```",
		"JSON fence":         "```json\n{\"source\":\"function revise() {}\"}\n```",
		"outer comment":      "// generated source\nfunction revise(value: number): number { return value; }",
		"ambiguous node": "function revise(value: number): number { return value; }\n" +
			"function revise(value: number): number { return value + 1; }",
	} {
		t.Run(name, func(t *testing.T) {
			executions := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), CorrectionModel: "corrector",
				Execute: testPortableExecutor(func(_ string, _ string, _ string) (string, error) {
					executions++
					return raw, nil
				}),
			}
			_, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
			if err == nil {
				t.Fatalf("projection error=%v", err)
			}
			if executions != 1 {
				t.Fatalf("invalid projection dispatched %d calls, want exactly 1", executions)
			}
		})
	}
}

func TestProviderTokenReceiptRemainsTheContextAuthority(t *testing.T) {
	t.Parallel()

	if err := llm.ValidateExactPreparedNativeUsage(8192, 6144, 2048, llm.ProviderGenerationUsage{
		PromptEvalCount: 1730, EvalCount: 1418,
	}); err != nil {
		t.Fatalf("measured in-budget provider usage was rejected: %v", err)
	}
	if err := llm.ValidateExactPreparedNativeUsage(8192, 6144, 2048, llm.ProviderGenerationUsage{
		PromptEvalCount: 6145, EvalCount: 1,
	}); err == nil {
		t.Fatal("measured provider usage beyond the native input budget was accepted")
	}
}

func validLargeTypeScriptDeclaration(signature string, minimumBytes int, initial string) string {
	var source strings.Builder
	source.WriteString(signature)
	source.WriteString(" {\n  let result = ")
	source.WriteString(initial)
	source.WriteString(";\n")
	for source.Len()+len("  return result;\n}") < minimumBytes {
		source.WriteString("  result = result + 0;\n")
	}
	source.WriteString("  return result;\n}")
	return source.String()
}
