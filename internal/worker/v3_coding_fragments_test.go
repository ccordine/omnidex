package worker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenerateTypeScriptFragmentsGivesModelsOnlyDirectAPIs(t *testing.T) {
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
		{
			ID: "domain", Path: "src/private/domain.ts",
			Blocks: []assemblyline.TypeScriptBlock{{
				ID: "value.type", Static: "export interface Value { amount: number }", API: "interface Value { amount: number }",
			}},
		},
		{
			ID: "functions", Path: "src/private/functions.ts",
			Blocks: []assemblyline.TypeScriptBlock{
				{
					ID: "value.double", Signature: "function double(value: Value): Value",
					Contract: "Return a new Value whose amount is twice the input amount.",
					API:      "function double(value: Value): Value", DependsOn: []string{"value.type"},
					Capabilities: []string{"value.type"},
				},
				{
					ID: "value.negative", Signature: "function negative(value: Value): boolean",
					Contract: "Return whether the input amount is below zero.",
					API:      "function negative(value: Value): boolean", DependsOn: []string{"value.type"},
					Capabilities: []string{"value.type"},
				},
			},
		},
	}}
	var mutex sync.Mutex
	prompts := make([]string, 0, 2)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, MaxConcurrency: 2,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, schema map[string]any) (string, error) {
			if schema != nil {
				t.Fatalf("fragment worker requested a response schema: %#v", schema)
			}
			mutex.Lock()
			prompts = append(prompts, prompt)
			mutex.Unlock()
			switch {
			case strings.Contains(prompt, "function double(value: Value): Value"):
				return "function double(value: Value): Value { return { amount: value.amount * 2 }; }", nil
			case strings.Contains(prompt, "function negative(value: Value): boolean"):
				return "function negative(value: Value): boolean { return value.amount < 0; }", nil
			default:
				t.Fatalf("unexpected TypeScript fragment prompt: %s", prompt)
				return "", nil
			}
		}),
	}
	generated, err := generateDirectCodingTypeScriptFragments(runtime, "qwen-coder", blueprint)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 2 || !strings.Contains(generated["value.double"], "amount * 2") {
		t.Fatalf("generated=%#v", generated)
	}
	for _, prompt := range prompts {
		for _, forbidden := range []string{
			"src/private", "domain.ts", "functions.ts", "value.double", "value.negative",
			"workspace", "job", "agent", "dependency graph",
		} {
			if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
				t.Fatalf("TypeScript fragment prompt leaked %q:\n%s", forbidden, prompt)
			}
		}
		if !strings.Contains(prompt, "interface Value { amount: number }") {
			t.Fatalf("fragment prompt omitted its one required API:\n%s", prompt)
		}
	}
}

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

func TestTypeScriptFragmentWaveHonorsOneLocalCapacityLane(t *testing.T) {
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{{
		ID: "functions", Path: "src/functions.ts", Blocks: []assemblyline.TypeScriptBlock{
			{ID: "one", Signature: "function one(): number", Contract: "Return one.", API: "function one(): number"},
			{ID: "two", Signature: "function two(): number", Contract: "Return two.", API: "function two(): number"},
		},
	}}}
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mutex sync.Mutex
	calls := 0
	firstPrompt := ""
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, MaxConcurrency: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, _ map[string]any) (string, error) {
			mutex.Lock()
			calls++
			call := calls
			if call == 1 {
				firstPrompt = prompt
			}
			mutex.Unlock()
			if call == 1 {
				close(firstStarted)
				<-releaseFirst
			} else if call == 2 {
				close(secondStarted)
			}
			const marker = "The declaration must match this signature exactly:\n"
			_, remainder, found := strings.Cut(prompt, marker)
			if !found {
				return "", fmt.Errorf("missing signature marker")
			}
			signature, _, _ := strings.Cut(remainder, "\n")
			return signature + " { return 1; }", nil
		}),
	}
	result := make(chan error, 1)
	go func() {
		_, err := generateDirectCodingTypeScriptFragments(runtime, "coder", blueprint)
		result <- err
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first fragment did not start")
	}
	mutex.Lock()
	startedPrompt := firstPrompt
	mutex.Unlock()
	if !strings.Contains(startedPrompt, "function one(): number") {
		close(releaseFirst)
		t.Fatalf("single-lane wave started out of blueprint order:\n%s", startedPrompt)
	}
	select {
	case <-secondStarted:
		close(releaseFirst)
		t.Fatal("second fragment started while the sole local capacity lane was occupied")
	default:
	}
	close(releaseFirst)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("serialized fragment wave did not complete")
	}
}

func TestTypeScriptFragmentWaveFailsBeforeStartingLaterSequentialWork(t *testing.T) {
	t.Parallel()

	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{{
		ID: "functions", Path: "src/functions.ts", Blocks: []assemblyline.TypeScriptBlock{
			{ID: "one", Signature: "function one(): number", Contract: "Return one.", API: "function one(): number"},
			{ID: "two", Signature: "function two(): number", Contract: "Return two.", API: "function two(): number"},
		},
	}}}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, MaxConcurrency: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			calls++
			return "export function one(): number { return 1; }", nil
		}),
	}
	_, err := generateDirectCodingTypeScriptFragments(runtime, "coder", blueprint)
	if err == nil {
		t.Fatal("invalid first sequential fragment unexpectedly succeeded")
	}
	if calls != 1 {
		t.Fatalf("started %d fragment workers after the first sequential failure", calls)
	}
}
