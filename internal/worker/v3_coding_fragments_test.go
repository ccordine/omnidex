package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptFragmentWorkerAcceptsOneInitialDeclaration(t *testing.T) {
	t.Parallel()
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "forbidden-corrector",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind != assemblyline.WorkFragmentGeneration || model != "coder" {
				t.Fatalf("initial call kind=%q model=%q", job.Kind, model)
			}
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: "function apply(value: number): number { return value + 1; }",
			}, nil
		},
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "coder", directCodingTypeScriptFragmentJob{dialect: "TypeScript 5.9.3", block: assemblyline.SourceBlock{
			ID: "calculation.apply", Signature: "function apply(value: number): number",
			Contract: "Return the input plus one.", API: "function apply(value: number): number",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(source, "value + 1") {
		t.Fatalf("calls=%d source=%q", calls, source)
	}
}

func TestTypeScriptFragmentWorkerFailsAfterOneInvalidInitialCandidate(t *testing.T) {
	t.Parallel()
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "forbidden-corrector",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if model != "coder" {
				t.Fatalf("unexpected model %q", model)
			}
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: "export function apply(value: number): number { return value + 1; }",
			}, nil
		},
	}
	_, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "coder", directCodingTypeScriptFragmentJob{dialect: "TypeScript 5.9.3", block: assemblyline.SourceBlock{
			ID: "calculation.apply", Signature: "function apply(value: number): number",
			Contract: "Return the input plus one.", API: "function apply(value: number): number",
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "one raw function declaration") || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestTypeScriptGuidedExecutorReceivesOnlyInstructionAndMutableDeclaration(t *testing.T) {
	t.Parallel()
	const current = "function apply(value: number): number { return value; }"
	const guidance = "Add one to the returned value."
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind != assemblyline.WorkFragmentCorrection || model != "executor" {
				t.Fatalf("executor call kind=%q model=%q", job.Kind, model)
			}
			var input assemblyline.FragmentCorrectionInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			if input.RepairGuidance != guidance || input.CurrentDeclaration != current ||
				input.Diagnostic != "" || input.RequiredChange != "" ||
				len(input.Capabilities) != 0 || len(input.PermittedSymbols) != 0 {
				t.Fatalf("executor authority=%+v", input)
			}
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: "function apply(value: number): number { return value + 1; }",
			}, nil
		},
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "executor", directCodingTypeScriptFragmentJob{
			block: assemblyline.SourceBlock{
				ID: "calculation.apply", Signature: "function apply(value: number): number",
				API: "function apply(value: number): number",
			},
			available: "interface HiddenAnalysis {}", current: current, repairGuidance: guidance,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(source, "value + 1") {
		t.Fatalf("calls=%d source=%q", calls, source)
	}
}

func TestTypeScriptGuidedRegionExecutorSplicesOneMutableRegion(t *testing.T) {
	t.Parallel()
	const current = "function apply(value: number): number {\n  return value;\n}"
	region := &assemblyline.TypeScriptFragmentRepairRegion{
		Kind:      assemblyline.TypeScriptRepairRegionSyntaxWindow,
		StartLine: 2, EndLine: 2, Source: "  return value;",
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "  return value + 1;"}, nil
		},
	}
	source, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "executor", directCodingTypeScriptFragmentJob{
			block: assemblyline.SourceBlock{
				ID: "calculation.apply", Signature: "function apply(value: number): number",
				API: "function apply(value: number): number",
			},
			current: current, repairRegion: region,
			repairGuidance: "Add one to the returned value.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "return value + 1;") {
		t.Fatalf("spliced source=%q", source)
	}
}

func TestTypeScriptGuidedExecutorRejectsNoChangeAfterOneCall(t *testing.T) {
	t.Parallel()
	const current = "function apply(value: number): number { return value; }"
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{JobID: job.ID, Candidate: current}, nil
		},
	}
	_, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "executor", directCodingTypeScriptFragmentJob{
			block: assemblyline.SourceBlock{
				ID: "calculation.apply", Signature: "function apply(value: number): number",
				API: "function apply(value: number): number",
			},
			current: current, repairGuidance: "Add one to the returned value.",
		},
	)
	if !errors.Is(err, errDirectCodingTypeScriptUnchangedCorrection) || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestTypeScriptFragmentWorkerStopsWhenRejectionPersistenceFails(t *testing.T) {
	t.Parallel()
	executions := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			executions++
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: "function apply(value: number): number { return value + ; }",
			}, nil
		},
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			if validationErr == nil {
				t.Fatal("persistence failure fixture requires a rejected candidate")
			}
			return errors.New("persist rejected station receipt")
		},
	}
	_, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "coder", directCodingTypeScriptFragmentJob{dialect: "TypeScript 5.9.3", block: assemblyline.SourceBlock{
			ID: "calculation.apply", Signature: "function apply(value: number): number",
			Contract: "Return the input plus one.", API: "function apply(value: number): number",
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "persist rejected station receipt") || executions != 1 {
		t.Fatalf("executions=%d error=%v", executions, err)
	}
}
