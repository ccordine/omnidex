package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExactTypeScriptReplayCompilerReturnsCandidateSyntaxAsCorrectableDiagnostic(t *testing.T) {
	compiler := &exactTypeScriptReplayCompiler{
		workspace: &directCodingTypeScriptStageWorkspace{root: "/unused-for-parser-rejection"},
		contract: assemblyline.TypeScriptFunctionContract{
			Signature: "function Repair(value: string): string",
			TSX:       true,
		},
	}
	diagnostic, err := compiler.Verify(
		context.Background(),
		"function Repair(value: string): string { return value; } }",
	)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic == nil || diagnostic.Count != 1 ||
		!strings.Contains(diagnostic.ModelFeedback, "TypeScript syntax rejected") ||
		len(diagnostic.CompilerDiagnostics) != 1 ||
		diagnostic.CompilerDiagnostics[0] != diagnostic.ModelFeedback ||
		diagnostic.RepairRegion == nil ||
		diagnostic.RepairRegion.Kind != assemblyline.TypeScriptRepairRegionSyntaxWindow {
		t.Fatalf("syntax rejection was not retained as one correctable diagnostic: %+v", diagnostic)
	}
}

func TestExactTypeScriptConvergenceRecordsErrorsBeyondBoundedModelFeedback(t *testing.T) {
	point, input := convergenceTestPoint(t)
	actual := []string{
		input.Diagnostic,
		"[source]: error TS2322: additional compiler failure",
	}
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, _ string) (*ExactTypeScriptReplayDiagnostic, error) {
			region := assemblyline.TypeScriptFragmentRepairRegion{
				Kind:      assemblyline.TypeScriptRepairRegionCompilerOwner,
				StartLine: 1, EndLine: 1, Source: input.CurrentDeclaration,
			}
			return &ExactTypeScriptReplayDiagnostic{
				ModelFeedback: input.Diagnostic, CompilerDiagnostics: actual, Count: len(actual),
				RepairRegion: &region,
			}, nil
		},
		replay: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			calls++
			return convergenceReplay(job, input.CurrentDeclaration), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(context.Background(), point, "model:test", runtime)
	if err == nil || !strings.Contains(err.Error(), "no-op") || calls != 1 ||
		result.Baseline.Count != 2 || len(result.Baseline.CompilerDiagnostics) != 2 {
		t.Fatalf("calls=%d baseline=%+v error=%v", calls, result.Baseline, err)
	}
}

func TestExactTypeScriptConvergenceStartsFromCurrentCompilerFeedback(t *testing.T) {
	point, input := convergenceTestPoint(t)
	currentFeedback := "DECLARATION_LOCATION: line 2 column 10\n" +
		"TYPESCRIPT_DIAGNOSTIC: error TS2322: Type 'string' is not assignable to type 'number'."
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, _ string) (*ExactTypeScriptReplayDiagnostic, error) {
			region := assemblyline.TypeScriptFragmentRepairRegion{
				Kind:      assemblyline.TypeScriptRepairRegionCompilerOwner,
				StartLine: 1, EndLine: 1, Source: input.CurrentDeclaration,
			}
			return &ExactTypeScriptReplayDiagnostic{
				ModelFeedback: currentFeedback, ModelFeedbackSHA256: replaySHA256(currentFeedback),
				CompilerDiagnostics: []string{input.Diagnostic}, Count: 1,
				RepairRegion: &region,
			}, nil
		},
		replay: func(_ context.Context, job assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			calls++
			var correction assemblyline.FragmentCorrectionInput
			if err := decodeReplayCorrectionInput(job, &correction); err != nil {
				return ExactStationReplay{}, err
			}
			if correction.CurrentDeclaration != "" || correction.RepairRegion == nil ||
				correction.RepairRegion.Source != input.CurrentDeclaration || correction.Diagnostic != currentFeedback {
				return ExactStationReplay{}, fmt.Errorf("first correction did not use current compiler authority: %+v", correction)
			}
			return convergenceReplay(job, input.CurrentDeclaration), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(context.Background(), point, "model:test", runtime)
	if err == nil || !strings.Contains(err.Error(), "no-op") || calls != 1 ||
		result.Baseline.ModelFeedback != currentFeedback {
		t.Fatalf("calls=%d baseline=%+v error=%v", calls, result.Baseline, err)
	}
}

func TestExactTypeScriptConvergenceRejectsCompilerPrefixDifferentFromFrozenAuthority(t *testing.T) {
	point, input := convergenceTestPoint(t)
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, _ string) (*ExactTypeScriptReplayDiagnostic, error) {
			return &ExactTypeScriptReplayDiagnostic{
				ModelFeedback:       input.Diagnostic,
				CompilerDiagnostics: []string{"[source]: error TS2322: different compiler failure"},
			}, nil
		},
		replay: func(_ context.Context, _ assemblyline.PortableJob, _ int) (ExactStationReplay, error) {
			calls++
			return ExactStationReplay{}, nil
		},
	}
	_, err := convergeExactTypeScriptStationWithRuntime(context.Background(), point, "model:test", runtime)
	if err == nil || !strings.Contains(err.Error(), "compiler diagnostic prefix differs") || calls != 0 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
