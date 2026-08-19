package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	liveTypeScriptGuidanceModelEnv = "OMNIDEX_TEST_TYPESCRIPT_REPAIR_GUIDANCE_MODEL"
	liveTypeScriptExecutorModelEnv = "OMNIDEX_TEST_TYPESCRIPT_REPAIR_EXECUTOR_MODEL"
	liveTypeScriptRepairReportEnv  = "OMNIDEX_TEST_TYPESCRIPT_REPAIR_REPORT"
)

type liveTypeScriptGuidedRepairCase struct {
	name           string
	wantIterations int
	input          assemblyline.FragmentCorrectionInput
}

type liveTypeScriptGuidedRepairReportHeader struct {
	Type          string    `json:"type"`
	Schema        string    `json:"schema"`
	CreatedAt     time.Time `json:"created_at"`
	GuidanceModel string    `json:"guidance_model"`
	ExecutorModel string    `json:"executor_model"`
	ContextTokens int       `json:"context_tokens"`
}

type liveTypeScriptGuidedRepairReportRun struct {
	Type                  string                                        `json:"type"`
	Case                  string                                        `json:"case"`
	Status                string                                        `json:"status"`
	Error                 string                                        `json:"error,omitempty"`
	SourcePoint           queue.StationCallReplayPoint                  `json:"source_point"`
	SourcePointDispatched bool                                          `json:"source_point_dispatched"`
	Convergence           ExactTypeScriptConvergence                    `json:"convergence"`
	Evidence              []liveTypeScriptGuidedRepairIterationEvidence `json:"evidence"`
}

type liveTypeScriptGuidedRepairIterationEvidence struct {
	Number                           int                          `json:"number"`
	GuidanceResponseCaptureBase64    string                       `json:"guidance_response_capture_base64"`
	GuidanceProviderIdentityEvidence llm.ProviderIdentityEvidence `json:"guidance_provider_identity_evidence"`
	ExecutorResponseCaptureBase64    string                       `json:"executor_response_capture_base64"`
	ExecutorProviderIdentityEvidence llm.ProviderIdentityEvidence `json:"executor_provider_identity_evidence"`
}

// TestLiveTypeScriptGuidanceExecutorCompilerConvergence is an opt-in primitive
// qualification, not an autonomy benchmark. It sends unrelated real compiler
// failures through the checked-in guidance renderer, a separately routed source
// executor, AST application, and the real pinned TypeScript compiler. One case
// requires two successive compiler-feedback iterations. The report retains
// every prepared request, provider response, and compiler result carried by
// ExactTypeScriptConvergence.
func TestLiveTypeScriptGuidanceExecutorCompilerConvergence(t *testing.T) {
	guidanceModel := strings.TrimSpace(os.Getenv(liveTypeScriptGuidanceModelEnv))
	executorModel := strings.TrimSpace(os.Getenv(liveTypeScriptExecutorModelEnv))
	if guidanceModel == "" && executorModel == "" {
		t.Skip(liveTypeScriptGuidanceModelEnv + " and " + liveTypeScriptExecutorModelEnv + " are not set")
	}
	if guidanceModel == "" || executorModel == "" {
		t.Fatal("both live TypeScript guidance and executor models are required")
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	reportPath := requireLiveCodingQualificationEnv(t, liveTypeScriptRepairReportEnv)
	report, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create exclusive live TypeScript repair report: %v", err)
	}
	defer func() {
		if closeErr := report.Close(); closeErr != nil {
			t.Errorf("close live TypeScript repair report: %v", closeErr)
		}
	}()
	encoder := json.NewEncoder(report)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(liveTypeScriptGuidedRepairReportHeader{
		Type: "header", Schema: "omnidex.live-guided-typescript-repair-qualification.v2",
		CreatedAt: time.Now().UTC(), GuidanceModel: guidanceModel,
		ExecutorModel: executorModel, ContextTokens: contextTokens,
	}); err != nil {
		t.Fatal(err)
	}
	client := ollama.NewUnbounded(baseURL, "", "", contextTokens)

	for _, fixture := range liveTypeScriptGuidedRepairCases() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			point := convergencePointForInput(t, fixture.input)
			ctx := context.Background()
			convergence, convergenceErr := ConvergeExactTypeScriptStation(
				ctx, client, point, guidanceModel, executorModel,
			)
			run := liveTypeScriptGuidedRepairReportRun{
				Type: "run", Case: fixture.name, Status: "passed",
				SourcePoint: point, SourcePointDispatched: false,
				Convergence: convergence, Evidence: liveTypeScriptGuidedRepairEvidence(convergence),
			}
			if convergenceErr != nil {
				run.Status, run.Error = "failed", convergenceErr.Error()
			}
			if err := encoder.Encode(run); err != nil {
				t.Fatalf("write live TypeScript repair evidence: %v", err)
			}
			if err := report.Sync(); err != nil {
				t.Fatalf("sync live TypeScript repair evidence: %v", err)
			}
			for _, iteration := range convergence.Iterations {
				t.Logf(
					"guided_repair case=%s iteration=%d guidance_model=%s guidance_prompt_tokens=%d guidance_output_tokens=%d instruction=%q executor_model=%s executor_prompt_tokens=%d executor_output_tokens=%d diagnostic_after=%s",
					fixture.name, iteration.Number, iteration.GuidanceReplay.Model,
					iteration.GuidanceReplay.Generation.Usage.PromptEvalCount,
					iteration.GuidanceReplay.Generation.Usage.EvalCount,
					iteration.Instruction, iteration.ExecutionReplay.Model,
					iteration.ExecutionReplay.Generation.Usage.PromptEvalCount,
					iteration.ExecutionReplay.Generation.Usage.EvalCount,
					liveTypeScriptDiagnosticState(iteration),
				)
			}
			if convergenceErr != nil {
				t.Fatal(convergenceErr)
			}
			if convergence.Terminal != ExactTypeScriptConvergenceCompiled ||
				len(convergence.Iterations) != fixture.wantIterations {
				t.Fatalf("terminal=%s iterations=%d want=%d",
					convergence.Terminal, len(convergence.Iterations), fixture.wantIterations)
			}
			for _, iteration := range convergence.Iterations {
				if iteration.GuidanceReplay.Model != guidanceModel ||
					iteration.ExecutionReplay.Model != executorModel ||
					iteration.GuidanceReplay.Job.Kind != assemblyline.WorkTypeScriptRepairGuidance ||
					iteration.ExecutionReplay.Job.Kind != assemblyline.WorkFragmentCorrection ||
					strings.TrimSpace(iteration.Instruction) == "" {
					t.Fatalf("iteration did not preserve split model authority: %+v", iteration)
				}
				var execution assemblyline.FragmentCorrectionInput
				if err := json.Unmarshal(iteration.ExecutionReplay.Job.Payload, &execution); err != nil {
					t.Fatal(err)
				}
				if execution.RepairGuidance != iteration.Instruction || execution.Diagnostic != "" ||
					execution.RequiredChange != "" || len(execution.Capabilities) != 0 ||
					len(execution.PermittedSymbols) != 0 {
					t.Fatalf("executor received analyst/compiler authority: %+v", execution)
				}
			}
		})
	}
}

func liveTypeScriptGuidedRepairCases() []liveTypeScriptGuidedRepairCase {
	return []liveTypeScriptGuidedRepairCase{
		{
			name: "shared numeric state", wantIterations: 1,
			input: assemblyline.FragmentCorrectionInput{
				Language:  "typescript",
				Signature: "function ReadMetric(state: MetricState): number",
				Capabilities: []string{
					"type SharedMetric = null | boolean | number | string",
					"type MetricState = { readonly [key: string]: SharedMetric }",
				},
				PermittedSymbols: []string{"useState"},
				CurrentDeclaration: `function ReadMetric(state: MetricState): number {
  const [reading] = useState<number>(() => state.reading ?? 0);
  return reading;
}`,
				RequiredChange: "Eliminate the exact compiler failure without changing unrelated statements.",
				Diagnostic:     "[source]: error TS2345: Argument of type '() => string | number | boolean' is not assignable to parameter of type 'number | (() => number)'.",
			},
		},
		{
			name: "shared text state", wantIterations: 1,
			input: assemblyline.FragmentCorrectionInput{
				Language:  "typescript",
				Signature: "function ReadLabel(state: LabelState): string",
				Capabilities: []string{
					"type SharedLabel = null | boolean | number | string",
					"type LabelState = { readonly [key: string]: SharedLabel }",
				},
				PermittedSymbols: []string{"useState"},
				CurrentDeclaration: `function ReadLabel(state: LabelState): string {
  const [label] = useState<string>(() => state.label ?? '');
  return label;
}`,
				RequiredChange: "Eliminate the exact compiler failure without changing unrelated statements.",
				Diagnostic:     "[source]: error TS2345: Argument of type '() => string | number | boolean' is not assignable to parameter of type 'string | (() => string)'.",
			},
		},
		{
			name: "nested lexical scope", wantIterations: 1,
			input: assemblyline.FragmentCorrectionInput{
				Language:  "typescript",
				Signature: "function CommitInventory(index: number, actions: InventoryActions): void",
				Capabilities: []string{
					"interface InventoryActions { commit(index: number, value: number): void }",
				},
				CurrentDeclaration: `function CommitInventory(index: number, actions: InventoryActions): void {
  const values: readonly number[] = [index];
  const commitNext = (): void => {
    values.forEach(value => {
      const nextValue = value + 1;
      void nextValue;
    });
    actions.commit(index, nextValue);
  };
  commitNext();
}`,
				RequiredChange: "Eliminate the exact compiler failure without changing unrelated statements.",
				Diagnostic:     "[source]: error TS2304: Cannot find name 'nextValue'.",
			},
		},
		{
			name: "incompatible local value", wantIterations: 1,
			input: assemblyline.FragmentCorrectionInput{
				Language:  "typescript",
				Signature: "function ScheduleDay(entry: ScheduleEntry): number",
				Capabilities: []string{
					"interface ScheduleEntry { readonly day: number }",
				},
				CurrentDeclaration: "function ScheduleDay(entry: ScheduleEntry): number {\n" +
					"  const day: number = `${entry.day}`;\n" +
					"  return day;\n" +
					"}",
				RequiredChange: "Eliminate the exact compiler failure without changing unrelated statements.",
				Diagnostic:     "[source]: error TS2322: Type 'string' is not assignable to type 'number'.",
			},
		},
		{
			name: "successive compiler failures", wantIterations: 2,
			input: assemblyline.FragmentCorrectionInput{
				Language:  "typescript",
				Signature: "function BatchTotal(entry: BatchEntry): number",
				Capabilities: []string{
					"interface BatchEntry { readonly amount: number; readonly count: number }",
				},
				CurrentDeclaration: "function BatchTotal(entry: BatchEntry): number {\n" +
					"  const amount: number = `${entry.amount}`;\n" +
					"  const count: number = `${entry.count}`;\n" +
					"  return amount + count;\n" +
					"}",
				RequiredChange: "Eliminate the exact compiler failure without changing unrelated statements.",
				Diagnostic:     "[source]: error TS2322: Type 'string' is not assignable to type 'number'.",
			},
		},
	}
}

func liveTypeScriptGuidedRepairEvidence(
	convergence ExactTypeScriptConvergence,
) []liveTypeScriptGuidedRepairIterationEvidence {
	evidence := make([]liveTypeScriptGuidedRepairIterationEvidence, 0, len(convergence.Iterations))
	for _, iteration := range convergence.Iterations {
		evidence = append(evidence, liveTypeScriptGuidedRepairIterationEvidence{
			Number: iteration.Number,
			GuidanceResponseCaptureBase64: base64.StdEncoding.EncodeToString(
				iteration.GuidanceReplay.Generation.ProviderResponseCapture,
			),
			GuidanceProviderIdentityEvidence: iteration.GuidanceReplay.Generation.ProviderIdentityEvidence,
			ExecutorResponseCaptureBase64: base64.StdEncoding.EncodeToString(
				iteration.ExecutionReplay.Generation.ProviderResponseCapture,
			),
			ExecutorProviderIdentityEvidence: iteration.ExecutionReplay.Generation.ProviderIdentityEvidence,
		})
	}
	return evidence
}

func liveTypeScriptDiagnosticState(iteration ExactTypeScriptConvergenceIteration) string {
	if iteration.ExecutionReplay.Job.ID == "" {
		return "not_run"
	}
	if iteration.AfterDiagnostic == nil {
		return "compiled"
	}
	return fmt.Sprintf("%s:%d", iteration.AfterDiagnostic.Stage, iteration.AfterDiagnostic.Count)
}
