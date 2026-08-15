package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExactTypeScriptReplayCompilerReturnsCandidateSyntaxAsCorrectableDiagnostic(t *testing.T) {
	compiler := &exactTypeScriptReplayCompiler{
		workspace: &directCodingTypeScriptStageWorkspace{root: "/unused-for-parser-rejection"},
		contract: assemblyline.TypeScriptFunctionContract{
			Signature: "function Repair(value: string): string", TSX: true,
		},
	}
	diagnostic, err := compiler.Verify(
		context.Background(), "function Repair(value: string): string { return value; } }",
	)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic == nil || diagnostic.Count != 1 ||
		!strings.Contains(diagnostic.ModelFeedback, "TypeScript syntax rejected") ||
		diagnostic.RepairRegion == nil ||
		diagnostic.RepairRegion.Kind != assemblyline.TypeScriptRepairRegionSyntaxWindow {
		t.Fatalf("syntax rejection was not retained as one correctable diagnostic: %+v", diagnostic)
	}
}

func TestExactTypeScriptConvergenceRecordsErrorsBeyondBoundedModelFeedback(t *testing.T) {
	point, input := convergenceTestPoint(t)
	actual := []string{input.Diagnostic, "[source]: error TS2322: additional compiler failure"}
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return convergenceDiagnostics(actual, source), nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceGuidanceReplay(job, "Change the failing expression."), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			calls++
			return convergenceReplay(job, input.CurrentDeclaration), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || !strings.Contains(err.Error(), "made no source change") || calls != 1 ||
		result.Baseline.Count != 2 || len(result.Baseline.CompilerDiagnostics) != 2 {
		t.Fatalf("calls=%d baseline=%+v error=%v", calls, result.Baseline, err)
	}
}

func TestExactTypeScriptConvergenceStartsGuidanceFromCurrentCompilerFeedback(t *testing.T) {
	point, input := convergenceTestPoint(t)
	currentFeedback := "DECLARATION_LOCATION: line 2 column 10\n" +
		"TYPESCRIPT_DIAGNOSTIC: error TS2322: Type 'string' is not assignable to type 'number'."
	guidanceCalls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			diagnostic := convergenceDiagnostic(input.Diagnostic, source)
			diagnostic.ModelFeedback = currentFeedback
			diagnostic.ModelFeedbackSHA256 = replaySHA256(currentFeedback)
			return diagnostic, nil
		},
		guide: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			guidanceCalls++
			var guidance assemblyline.TypeScriptRepairGuidanceInput
			if err := jsonUnmarshalPortable(job.Payload, &guidance); err != nil {
				return ExactStationReplay{}, err
			}
			if guidance.Diagnostic != currentFeedback || guidance.RepairRegion == nil ||
				guidance.RepairRegion.Source != input.CurrentDeclaration {
				return ExactStationReplay{}, fmt.Errorf("guidance did not use current compiler authority: %+v", guidance)
			}
			return convergenceGuidanceReplay(job, "Change the failing expression."), nil
		},
		execute: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			return convergenceReplay(job, input.CurrentDeclaration), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || guidanceCalls != 1 || result.Baseline.ModelFeedback != currentFeedback {
		t.Fatalf("guidance=%d baseline=%+v error=%v", guidanceCalls, result.Baseline, err)
	}
}

func TestExactTypeScriptConvergenceRejectsCompilerPrefixDifferentFromFrozenAuthority(t *testing.T) {
	point, input := convergenceTestPoint(t)
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			diagnostic := convergenceDiagnostic(
				"[source]: error TS2322: different compiler failure", source,
			)
			return diagnostic, nil
		},
		guide: func(_ context.Context, _ assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			calls++
			return ExactStationReplay{}, nil
		},
		execute: func(_ context.Context, _ assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			calls++
			return ExactStationReplay{}, nil
		},
	}
	_, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "analyst:test", "executor:test", runtime,
	)
	if err == nil || !strings.Contains(err.Error(), "compiler diagnostic prefix differs") || calls != 0 ||
		input.Diagnostic == "" {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func jsonUnmarshalPortable(raw []byte, target any) error {
	return json.Unmarshal(raw, target)
}
