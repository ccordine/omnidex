package worker

import (
	"context"
	"encoding/json"
	"errors"
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

func TestTypeScriptFragmentWorkerRepairsOnlyParserOwnedLineRegion(t *testing.T) {
	t.Parallel()

	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "calculation.apply", Signature: "function apply(value: number): number",
		Contract: "Return the incremented value.", API: "function apply(value: number): number",
	}}
	invalid := strings.Join([]string{
		"function apply(value: number): number {",
		"  const keepOne = value + 1;",
		"  const keepTwo = keepOne + 1;",
		"  const keepThree = keepTwo + 1;",
		"  const keepFour = keepThree + 1;",
		"  const keepFive = keepFour + 1;",
		"  return keepFive;",
		"}<|endoftext|><|im_start|>",
	}, "\n")
	var prompts []string
	var correctionInput assemblyline.FragmentCorrectionInput
	var correctionSchema map[string]any
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2, CorrectionModel: "corrector",
		Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, schema, err := assemblyline.RenderPortableJob(portable)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompts = append(prompts, prompt)
			candidate := invalid
			if len(prompts) == 2 {
				correctionSchema = schema
				if strings.Contains(string(portable.Payload), "keepOne") ||
					strings.Contains(string(portable.Payload), "current_declaration") {
					t.Fatalf("localized correction payload retained the full declaration: %s", portable.Payload)
				}
				if err := json.Unmarshal(portable.Payload, &correctionInput); err != nil {
					t.Fatal(err)
				}
				candidate = `{"replacement_lines":["  const keepFive = keepFour + 1;\n  return keepFive;\n}"]}`
			}
			return assemblyline.PortableResult{JobID: portable.ID, Candidate: candidate}, nil
		},
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 || !strings.Contains(source, "const keepOne") ||
		strings.Contains(source, "<|endoftext|>") {
		t.Fatalf("localized repair did not preserve and reparse the full declaration: attempts=%d\n%s", len(prompts), source)
	}
	if correctionInput.CurrentDeclaration != "" || correctionInput.RepairRegion == nil ||
		len(correctionInput.Capabilities) != 0 || len(correctionInput.PermittedSymbols) != 0 {
		t.Fatalf("parser correction retained whole-declaration authority: %#v", correctionInput)
	}
	if correctionInput.RepairRegion.StartLine != 6 || correctionInput.RepairRegion.EndLine != 8 {
		t.Fatalf("repair region=%#v want lines 6..8", correctionInput.RepairRegion)
	}
	properties, ok := correctionSchema["properties"].(map[string]any)
	if !ok || len(properties) != 1 || properties["replacement_lines"] == nil ||
		correctionSchema["additionalProperties"] != false {
		t.Fatalf("localized correction did not receive its exact closed response schema: %#v", correctionSchema)
	}
	for _, forbidden := range []string{"const keepOne", "const keepTwo", "<|endoftext|>", "<|im_start|>"} {
		if strings.Contains(prompts[1], forbidden) {
			t.Fatalf("localized correction prompt exposed %q:\n%s", forbidden, prompts[1])
		}
	}
	if !strings.Contains(prompts[1], "CURRENT_REPAIR_REGION_JSON:") {
		t.Fatalf("localized correction prompt omitted its typed region:\n%s", prompts[1])
	}
}

func TestTypeScriptFragmentWorkerRepairsMalformedTSXThroughOnlyItsLocalRegion(t *testing.T) {
	t.Parallel()

	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "view.render", Signature: "function render(): ReactElement",
		Contract: "Render one labeled section.", API: "function render(): ReactElement",
	}, tsx: true}
	invalid := strings.Join([]string{
		"function render(): ReactElement {",
		`  const keepOne = "one";`,
		`  const keepTwo = "two";`,
		`  const keepThree = "three";`,
		`  const keepFour = "four";`,
		`  const keepFive = "five";`,
		`  const label = keepOne + keepTwo + keepThree + keepFour + keepFive;`,
		"  return (",
		"    <section>",
		"      <span>{label}</span>",
		"    </section",
		"  );",
		"}",
	}, "\n")
	var prompts []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2, CorrectionModel: "corrector",
		Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, _, err := assemblyline.RenderPortableJob(portable)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompts = append(prompts, prompt)
			candidate := invalid
			if len(prompts) == 2 {
				var correction assemblyline.FragmentCorrectionInput
				if err := json.Unmarshal(portable.Payload, &correction); err != nil {
					t.Fatal(err)
				}
				if correction.CurrentDeclaration != "" || correction.RepairRegion == nil {
					t.Fatalf("malformed TSX correction was not region-only: %#v", correction)
				}
				if !strings.Contains(correction.RepairRegion.Source, "</section") {
					t.Fatalf("parser-owned region omitted the malformed TSX delimiter: %#v", correction.RepairRegion)
				}
				replacement := strings.ReplaceAll(correction.RepairRegion.Source, "</section", "</section>")
				encoded, err := json.Marshal(map[string]any{"replacement_lines": strings.Split(replacement, "\n")})
				if err != nil {
					t.Fatal(err)
				}
				candidate = string(encoded)
			}
			return assemblyline.PortableResult{JobID: portable.ID, Candidate: candidate}, nil
		},
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 || !strings.Contains(source, `const keepOne = "one";`) ||
		!strings.Contains(source, "</section>") {
		t.Fatalf("localized TSX splice did not preserve and reparse the full declaration: attempts=%d\n%s", len(prompts), source)
	}
	if strings.Contains(prompts[1], "keepOne") || strings.Contains(prompts[1], "CURRENT_DECLARATION_JSON:") {
		t.Fatalf("localized TSX prompt leaked declaration-wide source:\n%s", prompts[1])
	}
}

func TestTypeScriptFragmentWorkerStopsWhenRejectionPersistenceFails(t *testing.T) {
	t.Parallel()

	job := directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
		ID: "calculation.apply", Signature: "function apply(value: number): number",
		Contract: "Return the input plus one.", API: "function apply(value: number): number",
	}}
	executions := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "corrector",
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			executions++
			return "function apply(value: number): number { return value + 1; }<|endoftext|>", nil
		}),
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			if validationErr == nil {
				t.Fatal("persistence failure fixture requires a rejected parser candidate")
			}
			return errors.New("persist rejected station receipt")
		},
	}
	_, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", job)
	if err == nil || !strings.Contains(err.Error(), "persist rejected station receipt") {
		t.Fatalf("persistence failure was not returned exactly: %v", err)
	}
	if executions != 1 {
		t.Fatalf("persistence failure dispatched %d model calls, want exactly 1", executions)
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
