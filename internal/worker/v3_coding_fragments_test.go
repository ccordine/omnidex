package worker

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptFragmentWorkerCorrectsOnlyTheRejectedFunction(t *testing.T) {
	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "calculation.apply", Signature: "function apply(value: number): number",
		Contract: "Return the input plus one.", API: "function apply(value: number): number",
	}}
	var prompts []string
	var models []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "corrector",
		Execute: testPortableExecutor(func(_ string, model string, prompt string, _ map[string]any) (string, error) {
			prompts = append(prompts, prompt)
			models = append(models, model)
			if len(prompts) == 1 {
				return "```typescript\nexport function apply(value: number): number { return value + 1; }\n```", nil
			}
			return "function apply(value: number): number { return value + 1; }", nil
		}),
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(source, "function apply") || len(prompts) != 2 {
		t.Fatalf("source=%q attempts=%d", source, len(prompts))
	}
	if !strings.Contains(prompts[1], "raw function declaration") {
		t.Fatalf("exact parser failure did not reach correction prompt:\n%s", prompts[1])
	}
	if !strings.Contains(prompts[1], "export function apply") {
		t.Fatalf("correction did not receive the exact rejected declaration:\n%s", prompts[1])
	}
	if !strings.Contains(prompts[1], "```typescript") {
		t.Fatalf("correction did not preserve the exact rejected fenced candidate:\n%s", prompts[1])
	}
	if strings.Contains(prompts[1], "Return the input plus one") || strings.Contains(prompts[1], "LOCAL_BEHAVIOR") {
		t.Fatalf("correction replayed the superseded initial behavior:\n%s", prompts[1])
	}
	if !slices.Equal(models, []string{"coder", "corrector"}) {
		t.Fatalf("models=%v want dedicated correction routing", models)
	}
}

func TestTypeScriptFragmentWorkerRejectsMarkdownCodeEnvelope(t *testing.T) {
	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "calculation.apply", Signature: "function apply(value: number): number",
		Contract: "Return the input plus one.", API: "function apply(value: number): number",
	}}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return "```typescript\nfunction apply(value: number): number { return value + 1; }\n```", nil
		}),
	}
	if _, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job); err == nil {
		t.Fatal("Markdown-fenced TypeScript was accepted as one exact declaration")
	}
}

func TestTypeScriptFragmentWorkerRetriesAnUnchangedCorrectionWithOriginalFailure(t *testing.T) {
	const current = "function apply(value: number): number { return value; }"
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.TypeScriptBlock{
			ID: "calculation.apply", Signature: "function apply(value: number): number",
			Contract: "Return the input plus one.", API: "function apply(value: number): number",
		},
		current: current,
		failure: "AssertionError: expected 1 to be 2",
	}
	var prompts []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "corrector",
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, _ map[string]any) (string, error) {
			prompts = append(prompts, prompt)
			if len(prompts) == 1 {
				return current, nil
			}
			return "function apply(value: number): number { return value + 1; }", nil
		}),
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 || !strings.Contains(source, "value + 1") {
		t.Fatalf("prompts=%d source=%q", len(prompts), source)
	}
	for _, required := range []string{"expected 1 to be 2", "unchanged"} {
		if !strings.Contains(strings.ToLower(prompts[1]), strings.ToLower(required)) {
			t.Fatalf("second correction prompt omitted %q:\n%s", required, prompts[1])
		}
	}
	if strings.Contains(prompts[1], "Return the input plus one") || strings.Contains(prompts[1], "LOCAL_BEHAVIOR") {
		t.Fatalf("correction prompt replayed the original local behavior contract:\n%s", prompts[1])
	}
}

func TestTypeScriptFragmentWorkerGivesCommentViolationOneDirectTypedInstruction(t *testing.T) {
	t.Parallel()

	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "view.render", Signature: "function render(): ReactElement",
		Contract: "Render one usable control.", API: "function render(): ReactElement",
	}, tsx: true}
	var prompts []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "corrector",
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, _ map[string]any) (string, error) {
			prompts = append(prompts, prompt)
			if len(prompts) == 1 {
				return "function render(): ReactElement { return <button>{/* later */}Ready</button>; }", nil
			}
			return "function render(): ReactElement { return <button>Ready</button>; }", nil
		}),
	}
	if _, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job); err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 {
		t.Fatalf("attempts=%d", len(prompts))
	}
	for _, required := range []string{
		"Delete every comment node", "Change nothing unrelated", "comments are forbidden",
	} {
		if !strings.Contains(prompts[1], required) {
			t.Fatalf("direct correction omitted %q:\n%s", required, prompts[1])
		}
	}
	for _, forbidden := range []string{"Render one usable control", "LOCAL_BEHAVIOR"} {
		if strings.Contains(prompts[1], forbidden) {
			t.Fatalf("direct correction replayed %q:\n%s", forbidden, prompts[1])
		}
	}
}

func TestTypeScriptFragmentWorkerStopsRepeatedIdenticalCorrections(t *testing.T) {
	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "calculation.apply", Signature: "function apply(value: number): number",
		Contract: "Return the input plus one.", API: "function apply(value: number): number",
	}}
	attempts := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "corrector",
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			attempts++
			return "export function apply(value: number): number { return value + 1; }", nil
		}),
	}
	_, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err == nil || !strings.Contains(err.Error(), "no progress") {
		t.Fatalf("expected repeated-candidate failure, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("repeated candidate consumed %d attempts, want 2", attempts)
	}
}

func TestTypeScriptFragmentWorkerReportsOnlyEnvelopeMeasurements(t *testing.T) {
	const (
		available = "interface Value { amount: number }"
		current   = "function apply(value: Value): Value { return value; }"
		failure   = "REQUIRED_CHANGE: Return a copied value."
	)
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.TypeScriptBlock{
			ID: "calculation.apply", Signature: "function apply(value: Value): Value",
			Contract: "Return a copied value.", API: "function apply(value: Value): Value",
		},
		available: available,
		current:   current,
		failure:   failure,
	}
	var started typedWorkerEvent
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return "function apply(value: Value): Value { return { ...value }; }", nil
		}),
		Emit: func(event typedWorkerEvent) {
			if event.State == typedWorkerStarted {
				started = event
			}
		},
	}
	if _, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job); err != nil {
		t.Fatal(err)
	}
	if started.PromptBytes == 0 || started.CapabilityBytes != len(available) ||
		started.CurrentBytes != len(current) || started.CorrectionBytes != len(failure) {
		t.Fatalf("started event did not report the exact envelope measurements: %#v", started)
	}
	rendered := renderDirectCodingWorkerEvent(started)
	for _, secret := range []string{available, current, failure} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("worker status exposed envelope contents %q: %s", secret, rendered)
		}
	}
}
