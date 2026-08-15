package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestExactTypeScriptConvergenceContinuesBeyondThreeChangingFailures(t *testing.T) {
	point, input := convergenceTestPoint(t)
	sources := []string{
		convergenceFunction("one"), convergenceFunction("two"),
		convergenceFunction("three"), convergenceFunction("four"),
		convergenceFunction("five"),
	}
	diagnosticLines := []string{
		"error TS2304: first",
		"error TS2322: second",
		"error TS2345: third",
		"error TS2552: fourth",
		"error TS7053: fifth",
	}
	diagnostics := map[string]*ExactTypeScriptReplayDiagnostic{
		input.CurrentDeclaration: convergenceDiagnostics(diagnosticLines, input.CurrentDeclaration),
		sources[0]:               convergenceDiagnostics(diagnosticLines[1:], sources[0]),
		sources[1]:               convergenceDiagnostics(diagnosticLines[2:], sources[1]),
		sources[2]:               convergenceDiagnostics(diagnosticLines[3:], sources[2]),
		sources[3]:               convergenceDiagnostics(diagnosticLines[4:], sources[3]),
		sources[4]:               nil,
	}
	input.Diagnostic = diagnosticLines[0]
	point = convergencePointForInput(t, input)
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			return diagnostics[source], nil
		},
		replay: func(_ context.Context, job assemblyline.PortableJob, iteration int, _ *llm.ExactPreparedTemperature) (ExactStationReplay, error) {
			calls++
			var correction assemblyline.FragmentCorrectionInput
			if err := decodeReplayCorrectionInput(job, &correction); err != nil {
				return ExactStationReplay{}, err
			}
			wantSource := input.CurrentDeclaration
			if iteration > 1 {
				wantSource = sources[iteration-2]
			}
			wantDiagnostic := diagnostics[wantSource].ModelFeedback
			if correction.CurrentDeclaration != "" || correction.RepairRegion == nil ||
				correction.RepairRegion.Source != wantSource || correction.Diagnostic != wantDiagnostic {
				return ExactStationReplay{}, fmt.Errorf("iteration %d correction changed retained authority", iteration)
			}
			return convergenceReplay(job, sources[iteration-1]), nil
		},
	}

	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "model:test", runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != ExactTypeScriptConvergenceCompiled || calls != 5 || len(result.Iterations) != 5 {
		t.Fatalf("terminal=%s calls=%d iterations=%d", result.Terminal, calls, len(result.Iterations))
	}
	if result.FinalSource != sources[4] {
		t.Fatalf("final source=%q", result.FinalSource)
	}
}

func TestExactTypeScriptConvergenceExhaustsTemperatureOnNoOpAndNonImprovingCycle(t *testing.T) {
	for name, candidates := range map[string][]string{
		"no-op": {convergenceFunction("initial")},
		"cycle": {convergenceFunction("changed"), convergenceFunction("initial")},
	} {
		t.Run(name, func(t *testing.T) {
			point, input := convergenceTestPoint(t)
			input.CurrentDeclaration = convergenceFunction("initial")
			point = convergencePointForInput(t, input)
			calls := 0
			runtime := exactTypeScriptConvergenceRuntime{
				verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
					if source == convergenceFunction("changed") {
						return convergenceDiagnostic("error TS2322: changed", source), nil
					}
					return convergenceDiagnostic(input.Diagnostic, source), nil
				},
				replay: func(_ context.Context, job assemblyline.PortableJob, _ int, temperature *llm.ExactPreparedTemperature) (ExactStationReplay, error) {
					candidate := candidates[calls%len(candidates)]
					calls++
					return convergenceReplay(job, candidate, temperature), nil
				},
			}
			result, err := convergeExactTypeScriptStationWithRuntime(
				context.Background(), point, "model:test", runtime,
			)
			if err == nil || !strings.Contains(err.Error(), "exhausted profile temperature") ||
				result.Terminal != ExactTypeScriptConvergenceExplorationExhausted || calls != 6 {
				t.Fatalf("terminal=%s calls=%d error=%v", result.Terminal, calls, err)
			}
			if name == "no-op" && (len(result.Iterations) != 6 ||
				result.Iterations[0].AfterDiagnostic == nil ||
				result.Iterations[0].AfterDiagnostic.ModelFeedback != input.Diagnostic) {
				t.Fatalf("no-op lost its inherited compiler state: %+v", result.Iterations)
			}
		})
	}
}

func TestExactTypeScriptConvergenceRaisesTemperatureAcrossExactNoOpsThenCompiles(t *testing.T) {
	point, input := convergenceTestPoint(t)
	fixed := convergenceFunction("fixed")
	wantTemperatures := []float64{0, 0.2, 0.4}
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			if source == fixed {
				return nil, nil
			}
			return convergenceDiagnostic(input.Diagnostic, source), nil
		},
		replay: func(
			_ context.Context,
			job assemblyline.PortableJob,
			_ int,
			temperature *llm.ExactPreparedTemperature,
		) (ExactStationReplay, error) {
			if convergenceTemperatureValue(temperature) != wantTemperatures[calls] {
				return ExactStationReplay{}, fmt.Errorf("call %d temperature=%v", calls+1, temperature)
			}
			calls++
			if calls < len(wantTemperatures) {
				return convergenceReplay(job, input.CurrentDeclaration, temperature), nil
			}
			return convergenceReplay(job, fixed, temperature), nil
		},
	}

	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "model:test", runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != ExactTypeScriptConvergenceCompiled || calls != 3 || len(result.Iterations) != 3 {
		t.Fatalf("terminal=%s calls=%d iterations=%d", result.Terminal, calls, len(result.Iterations))
	}
	if result.Iterations[0].NextTemperature == nil ||
		float64(*result.Iterations[0].NextTemperature) != 0.2 ||
		result.Iterations[1].NextTemperature == nil ||
		float64(*result.Iterations[1].NextTemperature) != 0.4 {
		t.Fatalf("temperature escalation evidence=%+v", result.Iterations)
	}
}

func TestExactTypeScriptConvergenceResetsTemperatureAfterVerifiedProgress(t *testing.T) {
	point, input := convergenceTestPoint(t)
	progressed := convergenceFunction("progressed")
	fixed := convergenceFunction("fixed")
	wantTemperatures := []float64{0, 0.2, 0}
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			switch source {
			case fixed:
				return nil, nil
			case progressed:
				return convergenceDiagnostic("error TS2322: narrower", source), nil
			default:
				return &ExactTypeScriptReplayDiagnostic{
					Stage: ExactTypeScriptVerificationTypecheck, ModelFeedback: input.Diagnostic,
					ModelFeedbackSHA256:  replaySHA256(input.Diagnostic),
					CompilerOutputSHA256: replaySHA256(input.Diagnostic),
					CompilerDiagnostics:  []string{input.Diagnostic, "error TS2322: second"}, Count: 2,
					RepairRegion: &assemblyline.TypeScriptFragmentRepairRegion{
						Kind: assemblyline.TypeScriptRepairRegionCompilerOwner, StartLine: 1, EndLine: 1, Source: source,
						Bindings: convergenceTestBindings(),
					},
				}, nil
			}
		},
		replay: func(
			_ context.Context,
			job assemblyline.PortableJob,
			_ int,
			temperature *llm.ExactPreparedTemperature,
		) (ExactStationReplay, error) {
			if convergenceTemperatureValue(temperature) != wantTemperatures[calls] {
				return ExactStationReplay{}, fmt.Errorf("call %d temperature=%v", calls+1, temperature)
			}
			calls++
			switch calls {
			case 1:
				return convergenceReplay(job, input.CurrentDeclaration, temperature), nil
			case 2:
				return convergenceReplay(job, progressed, temperature), nil
			default:
				return convergenceReplay(job, fixed, temperature), nil
			}
		},
	}

	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "model:test", runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != ExactTypeScriptConvergenceCompiled || calls != 3 {
		t.Fatalf("terminal=%s calls=%d", result.Terminal, calls)
	}
}

func TestExactTypeScriptConvergenceRetainsValidSourceAcrossOneMalformedResponse(t *testing.T) {
	point, input := convergenceTestPoint(t)
	fixed := convergenceFunction("fixed")
	calls := 0
	runtime := exactTypeScriptConvergenceRuntime{
		verify: func(_ context.Context, source string) (*ExactTypeScriptReplayDiagnostic, error) {
			if source == fixed {
				return nil, nil
			}
			return convergenceDiagnostic(input.Diagnostic, source), nil
		},
		replay: func(_ context.Context, job assemblyline.PortableJob, iteration int, temperature *llm.ExactPreparedTemperature) (ExactStationReplay, error) {
			calls++
			if iteration == 1 {
				replay := convergenceReplay(job, "", temperature)
				replay.Generation = llm.PreparedGeneration{Content: "not a declaration"}
				return replay, &ExactStationReplayArtifactError{Cause: fmt.Errorf("missing required function")}
			}
			var correction assemblyline.FragmentCorrectionInput
			if err := decodeReplayCorrectionInput(job, &correction); err != nil {
				return ExactStationReplay{}, err
			}
			if correction.CurrentDeclaration != "" || correction.RepairRegion == nil ||
				correction.RepairRegion.Source != input.CurrentDeclaration ||
				!strings.Contains(correction.Diagnostic, "CORRECTION_REJECTION: missing required function") {
				return ExactStationReplay{}, fmt.Errorf("malformed response changed retained source authority")
			}
			return convergenceReplay(job, fixed, temperature), nil
		},
	}
	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "model:test", runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != ExactTypeScriptConvergenceCompiled || calls != 2 ||
		len(result.Iterations) != 2 || result.Iterations[0].ArtifactError == "" {
		t.Fatalf("terminal=%s calls=%d iterations=%+v", result.Terminal, calls, result.Iterations)
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
		replay: func(_ context.Context, job assemblyline.PortableJob, _ int, temperature *llm.ExactPreparedTemperature) (ExactStationReplay, error) {
			return convergenceReplay(job, candidate, temperature), nil
		},
	}

	result, err := convergeExactTypeScriptStationWithRuntime(
		context.Background(), point, "model:test", runtime,
	)
	if !errors.Is(err, verificationErr) {
		t.Fatalf("error=%v want verification failure", err)
	}
	if len(result.Iterations) != 1 || result.Iterations[0].Replay.Artifact.Source != candidate {
		t.Fatalf("dispatched candidate evidence was lost: %+v", result.Iterations)
	}
	if result.FinalSource != input.CurrentDeclaration ||
		result.LastCandidate != candidate || result.LastCandidateSHA256 != replaySHA256(candidate) {
		t.Fatalf("best/candidate evidence was lost: final=%q candidate=%q", result.FinalSource, result.LastCandidate)
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
		identity := renderTaskStageIdentity(program)
		for _, forbidden := range []string{"acceptance", "runtime", "application"} {
			if strings.Contains(identity, forbidden) {
				t.Fatalf("compiler context exposed %q: %s", forbidden, identity)
			}
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
	if got := exactTypeScriptReplayDiagnosticCount(raw); got != 2 {
		t.Fatalf("diagnostic count=%d want 2", got)
	}
	want := []string{
		"[source]:10:3: error TS2304: Cannot find name 'missingValue'.",
		"[source]:14:7: error TS2322: Type 'string' is not assignable to type 'number'.",
	}
	got := exactTypeScriptReplayDiagnosticLines(raw)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("diagnostic lines=%q want %q", got, want)
	}
}

func convergenceTestPoint(t *testing.T) (queue.StationCallReplayPoint, assemblyline.FragmentCorrectionInput) {
	t.Helper()
	input := assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function Repair(value: string): string",
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
		StartLine: 1, EndLine: 1, Source: source,
		Bindings: convergenceTestBindings(),
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

func convergenceReplay(
	job assemblyline.PortableJob,
	source string,
	temperatures ...*llm.ExactPreparedTemperature,
) ExactStationReplay {
	zero := llm.ExactPreparedTemperature(0)
	temperature := &zero
	if len(temperatures) > 0 && temperatures[0] != nil {
		temperature = temperatures[0]
	}
	return ExactStationReplay{
		Job: job, ExpectedIdentity: convergenceTestProviderIdentity(), Temperature: temperature,
		Artifact: ExactStationReplayArtifact{
			Kind: "typescript_function", Source: source, SourceSHA256: replaySHA256(source), ChangedFromBase: true,
		}}
}

func convergenceTestProviderIdentity() llm.ProviderIdentityExpectation {
	return llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "model:test", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: 8192, TokenizerProfile: llm.ExactPreparedTokenizerProfileQwen25Coder,
	}
}

func convergenceTemperatureValue(temperature *llm.ExactPreparedTemperature) float64 {
	if temperature == nil {
		return 0
	}
	return float64(*temperature)
}

func decodeReplayCorrectionInput(job assemblyline.PortableJob, target *assemblyline.FragmentCorrectionInput) error {
	return json.Unmarshal(job.Payload, target)
}
