package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptFragmentWorkerContinuesWhileRejectedCandidateStateChanges(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name      string
		block     assemblyline.TypeScriptBlock
		tsx       bool
		responses []string
		want      string
	}{
		{
			name: "numeric transformation",
			block: assemblyline.TypeScriptBlock{
				ID: "calculation.adjust", Signature: "function adjust(value: number): number",
				Contract: "Return the adjusted value.", API: "function adjust(value: number): number",
				Policy: assemblyline.TypeScriptFunctionPolicy{ForbiddenIdentifiers: []string{"console"}},
			},
			responses: []string{
				"function adjust(value: string): number { return Number(value) + 1; }",
				"function adjust(value: number): number { /* later */ return value + 1; }",
				"function adjust(value: number): number { const run = () => {}; run(); return value + 1; }",
				"function adjust(value: number): number { console.log(value); return value + 1; }",
				"function adjust(value: number): number { return value + 1; }",
			},
			want: "return value + 1",
		},
		{
			name: "visual component",
			block: assemblyline.TypeScriptBlock{
				ID: "view.panel", Signature: "function Panel(): ReactElement",
				Contract: "Render one labeled panel.", API: "function Panel(): ReactElement",
				Policy: assemblyline.TypeScriptFunctionPolicy{ForbiddenIdentifiers: []string{"console"}},
			},
			tsx: true,
			responses: []string{
				"function Panel(): JSX.Element { return <section />; }",
				"function Panel(): ReactElement { return <section>{/* later */}</section>; }",
				"function Panel(): ReactElement { const run = () => {}; run(); return <section />; }",
				"function Panel(): ReactElement { console.log('panel'); return <section />; }",
				"function Panel(): ReactElement { return <section aria-label=\"ready\" />; }",
			},
			want: "aria-label=\"ready\"",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, CorrectionModel: "corrector",
				Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
					if calls >= len(fixture.responses) {
						t.Fatalf("worker dispatched beyond the progressive fixture")
					}
					response := fixture.responses[calls]
					calls++
					return response, nil
				}),
			}
			source, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", directCodingTypeScriptFragmentJob{
				block: fixture.block, tsx: fixture.tsx,
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != len(fixture.responses) || !strings.Contains(source, fixture.want) {
				t.Fatalf("calls=%d source=%q", calls, source)
			}
		})
	}
}

func TestTypeScriptFragmentWorkerAllowsChangedCandidateWithSameDiagnosticUntilRealTerminalState(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name        string
		block       assemblyline.TypeScriptBlock
		tsx         bool
		responses   []string
		wantFailure string
	}{
		{
			name: "numeric candidate cycle",
			block: assemblyline.TypeScriptBlock{
				ID: "calculation.adjust", Signature: "function adjust(value: number): number",
				Contract: "Return the adjusted value.", API: "function adjust(value: number): number",
			},
			responses: []string{
				"function adjust(value: string): number { return Number(value) + 1; }",
				"function adjust(value: string): number { return Number(value) + 2; }",
				"function adjust(value: string): number { return Number(value) + 1; }",
			},
			wantFailure: "repeated candidate/diagnostic correction state",
		},
		{
			name: "visual candidate no-op",
			block: assemblyline.TypeScriptBlock{
				ID: "view.panel", Signature: "function Panel(): ReactElement",
				Contract: "Render one labeled panel.", API: "function Panel(): ReactElement",
			},
			tsx: true,
			responses: []string{
				"function Panel(): JSX.Element { return <section>one</section>; }",
				"function Panel(): JSX.Element { return <section>two</section>; }",
				"function Panel(): JSX.Element { return <section>two</section>; }",
			},
			wantFailure: errDirectCodingTypeScriptUnchangedCorrection.Error(),
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, CorrectionModel: "corrector",
				Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
					if calls >= len(fixture.responses) {
						t.Fatalf("worker dispatched after its terminal state")
					}
					response := fixture.responses[calls]
					calls++
					return response, nil
				}),
			}
			_, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", directCodingTypeScriptFragmentJob{
				block: fixture.block, tsx: fixture.tsx,
			})
			if err == nil || !strings.Contains(err.Error(), fixture.wantFailure) {
				t.Fatalf("terminal error=%v want %q", err, fixture.wantFailure)
			}
			if calls != len(fixture.responses) {
				t.Fatalf("terminal state dispatched %d calls, want %d", calls, len(fixture.responses))
			}
		})
	}
}

func TestTypeScriptFragmentWorkerStopsAtContextAuthority(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	runtime := typedWorkerRuntime{
		Context: ctx, MaxAttempts: 1, CorrectionModel: "corrector",
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			calls++
			cancel()
			return "export function adjust(value: number): number { return value; }", nil
		}),
	}
	_, err := runDirectCodingTypeScriptFragmentWorker(runtime, "coder", directCodingTypeScriptFragmentJob{
		block: assemblyline.TypeScriptBlock{
			ID: "calculation.adjust", Signature: "function adjust(value: number): number",
			Contract: "Return the adjusted value.", API: "function adjust(value: number): number",
		},
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("context authority error=%v", err)
	}
	if calls != 1 {
		t.Fatalf("context cancellation dispatched %d calls, want 1", calls)
	}
}

func TestTypeScriptCorrectionProgressUsesCandidateDiagnosticAndVerificationStage(t *testing.T) {
	t.Parallel()

	progress := newDirectCodingTypeScriptCorrectionProgress()
	states := []directCodingTypeScriptCorrectionState{
		{
			blockID: "inventory.filter", candidate: "function filter(): number { return 0; }",
			verificationStage: "run typecheck", diagnostic: "error TS2322: wrong result type",
		},
		{
			blockID: "inventory.filter", candidate: "function filter(): number { return 0; }",
			verificationStage: "run test", diagnostic: "expected 1 to be 2",
		},
		{
			blockID: "inventory.filter", candidate: "function filter(): number { return 1; }",
			verificationStage: "run test", diagnostic: "expected visible count to be 2",
		},
		{
			blockID: "profile.render", candidate: "function Profile(): ReactElement { return <main />; }",
			verificationStage: "run typecheck", diagnostic: "error TS2741: label is missing",
		},
		{
			blockID: "profile.render", candidate: "function Profile(): ReactElement { return <main />; }",
			verificationStage: "run build", diagnostic: "error TS2741: label is missing",
		},
	}
	for index, state := range states {
		if err := progress.observe(
			state.blockID, state.candidate, state.verificationStage, state.diagnostic,
		); err != nil {
			t.Fatalf("changing state %d was rejected: %v", index, err)
		}
	}
	repeated := states[1]
	if err := progress.observe(
		repeated.blockID, repeated.candidate, repeated.verificationStage, repeated.diagnostic,
	); err == nil || !strings.Contains(err.Error(), "repeated candidate/diagnostic correction state") {
		t.Fatalf("repeated exact progress state error=%v", err)
	}
}

func TestTypeScriptCorrectionProgressAllowsChangedSourceWithSameNormalizedCompilerDiagnostic(t *testing.T) {
	t.Parallel()

	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.observe(
		"inventory.filter",
		"function filter(): number { return missing; }",
		"run typecheck",
		"error TS2304: Cannot find name 'missing'.",
	); err != nil {
		t.Fatal(err)
	}
	if err := progress.observe(
		"inventory.filter",
		"function filter(): number { return missing + 1; }",
		"run typecheck",
		"  error TS2304: Cannot find name 'missing'.  ",
	); err != nil {
		t.Fatalf("changed source with the same normalized diagnostic was rejected: %v", err)
	}
	err := progress.observe(
		"inventory.filter",
		"function filter(): number { return missing; }",
		"run typecheck",
		"error TS2304: Cannot find name 'missing'.",
	)
	if err == nil || !strings.Contains(err.Error(), "repeated candidate/diagnostic correction state") {
		t.Fatalf("exact correction cycle error=%v", err)
	}
}

func TestTypeScriptSyntaxRepairWidensThenEscalatesAndResetsForNewFailure(t *testing.T) {
	t.Parallel()

	var progress directCodingTypeScriptSyntaxProgress
	first := assemblyline.TypeScriptSyntaxFailure{Kind: "ERROR", Line: 8, Column: 4}
	second := assemblyline.TypeScriptSyntaxFailure{Kind: "missing", Line: 11, Column: 2}
	got := []directCodingTypeScriptSyntaxRepair{
		progress.next(first),
		progress.next(first),
		progress.next(first),
		progress.next(second),
		progress.next(second),
	}
	want := []directCodingTypeScriptSyntaxRepair{
		{radius: 2},
		{radius: 4},
		{wholeDeclaration: true},
		{radius: 2},
		{radius: 4},
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("repairs=%v want %v", got, want)
		}
	}
}

func TestTypeScriptCorrectionPathsHaveNoFixedCountTermination(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"v3_coding_typescript_fragment_worker.go",
		"v3_coding_typescript_stage_workspace.go",
		"v3_application_task_runtime.go",
		"v3_coding_stage.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"maxDirectCodingStageCorrections",
			"maxDirectCodingStageRepeatedCorrections",
			"remainingCorrections",
			"attempt <= runtime.MaxAttempts",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s retains fixed correction termination %q", path, forbidden)
			}
		}
	}
}
