package worker

import "testing"

func TestExactTypeScriptDiagnosticDeltaScoresResolvedAndIntroducedMultisets(t *testing.T) {
	t.Parallel()
	before := &ExactTypeScriptReplayDiagnostic{
		Stage: ExactTypeScriptVerificationTypecheck,
		CompilerDiagnostics: []string{
			"[source]:11:3: error TS2304: Cannot find name 'missingItem'.",
			"[source]:24:7: error TS2322: Type 'SharedValue' is not assignable to type 'ReactNode'.",
			"[source]:31:7: error TS2322: Type 'SharedValue' is not assignable to type 'ReactNode'.",
		},
		Count: 3,
	}
	after := &ExactTypeScriptReplayDiagnostic{
		Stage: ExactTypeScriptVerificationTypecheck,
		CompilerDiagnostics: []string{
			"[source]:26:9: error TS2322: Type 'SharedValue' is not assignable to type 'ReactNode'.",
			"[source]:40:5: error TS2345: Argument of type 'string' is not assignable to parameter of type 'number'.",
		},
		Count: 2,
	}

	delta, err := exactTypeScriptDiagnosticDelta(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if delta.Before != 3 || delta.After != 2 || delta.Resolved != 2 ||
		delta.Retained != 1 || delta.Introduced != 1 ||
		delta.Assessment != ExactTypeScriptConvergenceProgress {
		t.Fatalf("delta=%+v", delta)
	}
}

func TestExactTypeScriptDiagnosticDeltaUsesVerificationStageBeforeCounts(t *testing.T) {
	t.Parallel()
	type fixture struct {
		name       string
		before     *ExactTypeScriptReplayDiagnostic
		after      *ExactTypeScriptReplayDiagnostic
		assessment ExactTypeScriptConvergenceAssessment
	}
	fixtures := []fixture{
		{
			name: "schedule typecheck to syntax regression",
			before: &ExactTypeScriptReplayDiagnostic{
				Stage: ExactTypeScriptVerificationTypecheck,
				CompilerDiagnostics: []string{
					"[source]:8:3: error TS2304: Cannot find name 'schedule'.",
					"[source]:9:3: error TS2322: Type 'string' is not assignable to type 'number'.",
				}, Count: 2,
			},
			after: &ExactTypeScriptReplayDiagnostic{
				Stage:               ExactTypeScriptVerificationSyntax,
				CompilerDiagnostics: []string{"TypeScript syntax rejected at line 4 column 2"}, Count: 1,
			},
			assessment: ExactTypeScriptConvergenceRegression,
		},
		{
			name: "inventory syntax to typecheck advance",
			before: &ExactTypeScriptReplayDiagnostic{
				Stage:               ExactTypeScriptVerificationSyntax,
				CompilerDiagnostics: []string{"TypeScript syntax rejected at line 2 column 7"}, Count: 1,
			},
			after: &ExactTypeScriptReplayDiagnostic{
				Stage: ExactTypeScriptVerificationTypecheck,
				CompilerDiagnostics: []string{
					"[source]:7:4: error TS2304: Cannot find name 'inventory'.",
					"[source]:8:4: error TS2322: Type 'number' is not assignable to type 'string'.",
				}, Count: 2,
			},
			assessment: ExactTypeScriptConvergenceProgress,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			delta, err := exactTypeScriptDiagnosticDelta(fixture.before, fixture.after)
			if err != nil {
				t.Fatal(err)
			}
			if delta.Assessment != fixture.assessment {
				t.Fatalf("assessment=%s delta=%+v", delta.Assessment, delta)
			}
		})
	}
}
