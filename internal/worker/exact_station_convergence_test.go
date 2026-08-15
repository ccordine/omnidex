package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestExactTypeScriptConvergenceUsesGuidanceThenExecutorUntilCompilerPasses(t *testing.T) {
	point, input := convergenceTestPoint(t)
	sources := []string{
		convergenceFunction("one"), convergenceFunction("two"), convergenceFunction("fixed"),
	}
	diagnosticLines := []string{
		"error TS2304: first", "error TS2322: second", "error TS2345: third",
	}
	diagnostics := map[string]*ExactTypeScriptReplayDiagnostic{
		input.CurrentDeclaration: convergenceDiagnostics(diagnosticLines, input.CurrentDeclaration),
		sources[0]:               convergenceDiagnostics(diagnosticLines[1:], sources[0]),
		sources[1]:               convergenceDiagnostics(diagnosticLines[2:], sources[1]),
		sources[2]:               nil,
	}
	input.Diagnostic = diagnosticLines[0]
	point = convergencePointForInput(t, input)
	guidanceCalls, executionCalls := 0, 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return diagnostics[source], nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, iteration int) (ExactStationReplay, error) {
			guidanceCalls++
			var guidance assemblyline.TypeScriptRepairGuidanceInput
			if err := json.Unmarshal(job.Payload, &guidance); err != nil {
				return ExactStationReplay{}, err
			}
			wantSource := input.CurrentDeclaration
			if iteration > 1 {
				wantSource = sources[iteration-2]
			}
			if guidance.CurrentDeclaration != "" || guidance.RepairRegion == nil ||
				guidance.RepairRegion.Source != wantSource ||
				guidance.Diagnostic != diagnostics[wantSource].ModelFeedback {
				return ExactStationReplay{}, fmt.Errorf("iteration %d guidance changed compiler authority", iteration)
			}
			return convergenceGuidanceReplay(job, fmt.Sprintf("Apply repair %d.", iteration)), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, iteration int) (ExactStationReplay, error) {
			executionCalls++
			var correction assemblyline.FragmentCorrectionInput
			if err := decodeReplayCorrectionInput(job, &correction); err != nil {
				return ExactStationReplay{}, err
			}
			if correction.RepairRegion == nil || correction.RepairGuidance != fmt.Sprintf("Apply repair %d.", iteration) ||
				correction.Diagnostic != "" || correction.RequiredChange != "" ||
				len(correction.Capabilities) != 0 || len(correction.PermittedSymbols) != 0 {
				return ExactStationReplay{}, fmt.Errorf("iteration %d executor retained analyst authority", iteration)
			}
			return convergenceReplay(job, sources[iteration-1]), nil
		},
	}

	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != ExactTypeScriptConvergenceCompiled || guidanceCalls != 3 ||
		executionCalls != 3 || len(result.Iterations) != 3 || result.FinalSource != sources[2] {
		t.Fatalf("terminal=%s guidance=%d execution=%d iterations=%d final=%q",
			result.Terminal, guidanceCalls, executionCalls, len(result.Iterations), result.FinalSource)
	}
}

func TestExactTypeScriptConvergenceFailsDeterministicallyOnGuidedNoOp(t *testing.T) {
	point, input := convergenceTestPoint(t)
	guidanceCalls, executionCalls := 0, 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return convergenceDiagnostic(input.Diagnostic, source), nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			guidanceCalls++
			return convergenceGuidanceReplay(job, "Change the failing expression."), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			executionCalls++
			return convergenceReplay(job, input.CurrentDeclaration), nil
		},
	}

	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || !strings.Contains(err.Error(), "made no source change") ||
		result.Terminal != ExactTypeScriptConvergenceStalled || guidanceCalls != 1 || executionCalls != 1 {
		t.Fatalf("terminal=%s guidance=%d execution=%d error=%v",
			result.Terminal, guidanceCalls, executionCalls, err)
	}
}

func TestExactTypeScriptConvergenceFailsOnNonImprovingCompilerResult(t *testing.T) {
	point, input := convergenceTestPoint(t)
	changed := convergenceFunction("changed")
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return convergenceDiagnostic(input.Diagnostic, source), nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceGuidanceReplay(job, "Change the failing expression."), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceReplay(job, changed), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || !strings.Contains(err.Error(), "no verified compiler progress") ||
		result.Terminal != ExactTypeScriptConvergenceStalled || len(result.Iterations) != 1 ||
		result.Iterations[0].DiagnosticDelta == nil ||
		result.Iterations[0].DiagnosticDelta.Assessment != ExactTypeScriptConvergenceUnchanged {
		t.Fatalf("terminal=%s iterations=%+v error=%v", result.Terminal, result.Iterations, err)
	}
}

func TestExactTypeScriptConvergencePreservesGuidanceArtifactFailure(t *testing.T) {
	point, input := convergenceTestPoint(t)
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return convergenceDiagnostic(input.Diagnostic, source), nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			replay := convergenceReplay(job, "")
			return replay, &ExactStationReplayArtifactError{Cause: fmt.Errorf("instruction JSON is invalid")}
		},
		execute: func(context.Context, assemblyline.PortableJob, int) (ExactStationReplay, error) {
			return ExactStationReplay{}, fmt.Errorf("executor must not run")
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || len(result.Iterations) != 1 ||
		result.Iterations[0].GuidanceArtifactError != "instruction JSON is invalid" ||
		result.Iterations[0].ExecutionReplay.Job.ID != "" {
		t.Fatalf("iterations=%+v error=%v", result.Iterations, err)
	}
}

func TestExactTypeScriptConvergencePreservesExecutorArtifactFailure(t *testing.T) {
	point, input := convergenceTestPoint(t)
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return convergenceDiagnostic(input.Diagnostic, source), nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceGuidanceReplay(job, "Change the failing expression."), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			replay := convergenceReplay(job, "")
			return replay, &ExactStationReplayArtifactError{Cause: fmt.Errorf("source is not parseable")}
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || len(result.Iterations) != 1 ||
		result.Iterations[0].Instruction == "" ||
		result.Iterations[0].ExecutionArtifactError != "source is not parseable" {
		t.Fatalf("iterations=%+v error=%v", result.Iterations, err)
	}
}

func TestExactTypeScriptConvergenceRetainsDispatchedCandidateWhenVerificationFails(t *testing.T) {
	point, input := convergenceTestPoint(t)
	candidate := convergenceFunction("compiler-rejected")
	verificationErr := fmt.Errorf("compiler receipt could not be projected")
	verifyCalls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			verifyCalls++
			if verifyCalls == 1 && source == input.CurrentDeclaration {
				return convergenceDiagnostic(input.Diagnostic, source), nil
			}
			if verifyCalls == 2 && source == candidate {
				return nil, verificationErr
			}
			return nil, fmt.Errorf("unexpected verification call %d for %q", verifyCalls, source)
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceGuidanceReplay(job, "Change the failing expression."), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceReplay(job, candidate), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if !errors.Is(err, verificationErr) || len(result.Iterations) != 1 ||
		result.Iterations[0].ExecutionReplay.Artifact.Source != candidate ||
		result.FinalSource != input.CurrentDeclaration || result.LastCandidate != candidate {
		t.Fatalf("final=%q candidate=%q iterations=%+v error=%v",
			result.FinalSource, result.LastCandidate, result.Iterations, err)
	}
}

func TestExactTypeScriptReplayProgramUsesOnlyCorrectionAuthority(t *testing.T) {
	fixtures := []assemblyline.FragmentCorrectionInput{
		{
			Language: "typescript", Signature: "function FilterInventory(value: string): ReactElement",
			Capabilities:       []string{"interface InventoryProps { readonly value: string }"},
			PermittedSymbols:   []string{"ReactElement", "useMemo"},
			CurrentDeclaration: "function FilterInventory(value: string): ReactElement { return <div>{value}</div>; }",
			RequiredChange:     "Fix the observed failure.", Diagnostic: "error TS2304: inventory",
		},
		{
			Language: "typescript", Signature: "function ToggleSchedule(active: boolean): ReactElement",
			Capabilities:       []string{"type ScheduleState = { readonly active: boolean }"},
			PermittedSymbols:   []string{"ReactElement", "useCallback"},
			CurrentDeclaration: "function ToggleSchedule(active: boolean): ReactElement { return <button>{String(active)}</button>; }",
			RequiredChange:     "Fix the observed failure.", Diagnostic: "error TS2322: schedule",
		},
	}
	for _, fixture := range fixtures {
		program, err := exactTypeScriptReplayProgram(fixture, fixture.CurrentDeclaration)
		if err != nil {
			t.Fatal(err)
		}
		if len(program.TypeScript.Documents) != 1 || len(program.Generated) != 1 ||
			!strings.Contains(program.TypeScript.Documents[0].Header, fixture.Capabilities[0]) {
			t.Fatalf("program exposed an inexact compiler context: %+v", program)
		}
	}
}

func TestExactTypeScriptReplayDiagnosticCountDeduplicatesRepeatedCompilerLines(t *testing.T) {
	raw := strings.Join([]string{
		"src/replay.tsx(10,3): error TS2304: Cannot find name 'missingValue'.",
		"src/replay.tsx(10,3): error TS2304: Cannot find name 'missingValue'.",
		"src/replay.tsx(14,7): error TS2322: Type 'string' is not assignable to type 'number'.",
		"Found 2 errors.",
	}, "\n")
	want := []string{
		"[source]:10:3: error TS2304: Cannot find name 'missingValue'.",
		"[source]:14:7: error TS2322: Type 'string' is not assignable to type 'number'.",
	}
	got := exactTypeScriptReplayDiagnosticLines(raw)
	if exactTypeScriptReplayDiagnosticCount(raw) != 2 || len(got) != len(want) ||
		got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("diagnostic lines=%q want %q", got, want)
	}
}

func convergenceTestPoint(t *testing.T) (queue.StationCallReplayPoint, assemblyline.FragmentCorrectionInput) {
	t.Helper()
	input := assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function Repair(value: string): string",
		Capabilities: []string{"type RepairValue = string"}, PermittedSymbols: []string{"String"},
		CurrentDeclaration: convergenceFunction("initial"), RequiredChange: "Fix the observed failure.",
		Diagnostic: "error TS2304: initial",
	}
	return convergencePointForInput(t, input), input
}

func convergencePointForInput(t *testing.T, input assemblyline.FragmentCorrectionInput) queue.StationCallReplayPoint {
	t.Helper()
	job, err := assemblyline.NewFragmentCorrectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	gap := replayTestGap(t, job)
	return queue.StationCallReplayPoint{Call: replayTestCall(t, gap), Gap: gap}
}

func convergenceFunction(value string) string {
	return fmt.Sprintf("function Repair(value: string): string { return %q; }", value)
}

func convergenceDiagnostic(feedback string, source string) *ExactTypeScriptReplayDiagnostic {
	return convergenceDiagnostics(strings.Split(feedback, "\n"), source)
}

func convergenceDiagnostics(diagnostics []string, source string) *ExactTypeScriptReplayDiagnostic {
	region := assemblyline.TypeScriptFragmentRepairRegion{
		Kind:      assemblyline.TypeScriptRepairRegionCompilerOwner,
		StartLine: 1, EndLine: 1, Source: source, Bindings: convergenceTestBindings(),
	}
	feedback := diagnostics[0]
	return &ExactTypeScriptReplayDiagnostic{
		Stage: ExactTypeScriptVerificationTypecheck, ModelFeedback: feedback,
		ModelFeedbackSHA256: replaySHA256(feedback), CompilerDiagnostics: diagnostics,
		CompilerOutputSHA256: replaySHA256(strings.Join(diagnostics, "\n")),
		Count:                len(diagnostics), RepairRegion: &region,
	}
}

func convergenceTestBindings() []assemblyline.TypeScriptRepairBinding {
	return []assemblyline.TypeScriptRepairBinding{{Name: "value", Type: "string"}}
}

func convergenceReplay(job assemblyline.PortableJob, source string) ExactStationReplay {
	return ExactStationReplay{
		Job: job,
		Artifact: ExactStationReplayArtifact{
			Kind: "typescript_repair_region", Source: source,
			SourceSHA256: replaySHA256(source), ChangedFromBase: true,
		},
	}
}

func convergenceGuidanceReplay(job assemblyline.PortableJob, instruction string) ExactStationReplay {
	return ExactStationReplay{
		Job: job,
		Artifact: ExactStationReplayArtifact{
			Kind: "typescript_repair_guidance", Source: instruction,
			SourceSHA256: replaySHA256(instruction),
		},
	}
}

func decodeReplayCorrectionInput(job assemblyline.PortableJob, target *assemblyline.FragmentCorrectionInput) error {
	return json.Unmarshal(job.Payload, target)
}
