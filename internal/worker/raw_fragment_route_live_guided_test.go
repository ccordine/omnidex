package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type liveGuidedTSXFixture struct {
	name, functionName, role, matcher string
	verifierSource, failureMessage    string
	target                            assemblyline.SourceBlock
	available, current                string
	observedNames                     []string
	requiredFeedback                  []string
	supplementSentinel, behaviorTest  string
}

func runLiveQwenGuidedTSXRepairQualification(
	t *testing.T,
	runtime typedWorkerRuntime,
	modelName string,
	provider *liveRawFragmentStationClient,
	pool *pgxpool.Pool,
	jobID int64,
) {
	t.Helper()
	const dialect = "TypeScript 5.9.3 TSX function syntax"
	for _, fixture := range liveGuidedTSXFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			regexAuthority, err := assemblyline.TypeScriptRegularExpressionLiterals(
				fixture.verifierSource, true,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(regexAuthority) != 1 || regexAuthority[0] != fixture.matcher {
				t.Fatalf("verifier regex authority=%q want exactly %q", regexAuthority, fixture.matcher)
			}
			feedback, err := directCodingTypeScriptStructuredTestModelFailure(
				directCodingVitestFailureEvidence{
					FailureClass: directCodingStageFailureVitestBehavior,
					Name:         "TestingLibraryElementError",
					Message:      fixture.failureMessage,
					AccessibilityObservation: &directCodingTestingLibraryRoleObservation{
						Schema:        directCodingTestingLibraryRoleObservationSchemaV1,
						RequestedRole: fixture.role,
						Visibility:    directCodingTestingLibraryRoleVisibilityAccessible,
						Status:        directCodingTestingLibraryRoleObservationStatusComplete,
						ElementCount:  int64(len(fixture.observedNames)),
						Names:         append([]string(nil), fixture.observedNames...),
					},
				},
				assemblyline.ArtifactIdentityProvenance{}, regexAuthority...,
			)
			if err != nil {
				t.Fatal(err)
			}
			feedback, err = directCodingTypeScriptStageModelFeedback(
				&directCodingStageDiagnostic{BlockID: fixture.target.ID, ModelFeedback: feedback},
			)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(feedback, fixture.matcher) ||
				strings.Contains(feedback, fixture.supplementSentinel) ||
				!strings.Contains(feedback, "plain text") {
				t.Fatalf("strict feedback retained raw matcher or lost canonical authority: %q", feedback)
			}
			for _, required := range fixture.requiredFeedback {
				if !strings.Contains(feedback, required) {
					t.Fatalf("strict feedback omitted %q: %q", required, feedback)
				}
			}

			guidanceJob, err := assemblyline.NewFragmentRepairGuidanceJob(
				assemblyline.FragmentRepairGuidanceInput{
					Language: "typescript", Dialect: dialect,
					Signature: fixture.target.Signature, Capabilities: []string{fixture.available},
					PermittedSymbols:   append([]string(nil), fixture.target.Globals...),
					CurrentDeclaration: fixture.current, Diagnostic: feedback,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			start := provider.callCount()
			source, convergenceErr := convergeDirectCodingTypeScriptGuidedRepairWithRuntime(
				runtime, modelName, modelName, directCodingTypeScriptRepairEvents{},
				fixture.target, true, dialect, fixture.available, fixture.current, nil, feedback,
			)
			calls := provider.callsFrom(start)
			if convergenceErr != nil {
				t.Fatalf(
					"guided repair failed after %d calls: %v\nguidance response: %q\nexecutor response: %q",
					len(calls), convergenceErr, liveRawFragmentCallContent(calls, 0),
					liveRawFragmentCallContent(calls, 1),
				)
			}
			if len(calls) != 2 || calls[0].Err != nil || calls[1].Err != nil {
				t.Fatalf(
					"guided repair provider calls=%d first_error=%v second_error=%v, want exactly two successful calls",
					len(calls), liveRawFragmentCallError(calls, 0), liveRawFragmentCallError(calls, 1),
				)
			}
			guidance, err := assemblyline.DecodeFragmentRepairGuidanceResult(
				guidanceJob, calls[0].Generation.Content,
			)
			if err != nil {
				t.Fatal(err)
			}
			correctionJob, err := newDirectCodingTypeScriptPortableJob(
				directCodingTypeScriptFragmentJob{
					block: fixture.target, tsx: true, available: fixture.available,
					current: fixture.current, repairGuidance: guidance.Instruction,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			assertLiveProductionRawFragment(
				t, pool, jobID, guidanceJob, guidance.Instruction,
				queue.StationGapProjectionExactResponse, calls[:1],
			)
			assertLiveProductionRawFragment(
				t, pool, jobID, correctionJob, source,
				queue.StationGapProjectionTypeScriptFunction, calls[1:],
			)
			assertLiveGuidedTSXRepairQualification(
				t, fixture.name, fixture.functionName, fixture.target,
				fixture.available, fixture.current, fixture.verifierSource,
				fixture.behaviorTest, fixture.matcher, fixture.supplementSentinel,
				dialect, feedback, guidance.Instruction, source, calls,
			)
		})
	}
}

func liveGuidedTSXFixtures() []liveGuidedTSXFixture {
	return []liveGuidedTSXFixture{
		{
			name: "parcel dispatch button", functionName: "DispatchPanel",
			role: "button", matcher: `/dispatch parcels/i`,
			verifierSource: `async function VerifyDispatch(): Promise<void> {
  expect(screen.getByRole('button', { name: /dispatch parcels/i })).toBeInTheDocument();
}`,
			failureMessage: "Unable to find an accessible element with the role \"button\" and name `/dispatch parcels/i`.\n\nPARCEL_DOM_SUPPLEMENT_SENTINEL <button>7 ready</button>",
			target: assemblyline.SourceBlock{
				ID:        "fixture.parcel.presentation",
				Signature: "function DispatchPanel({ waiting, dispatch }: DispatchPanelProps): ReactElement",
				Contract:  "Render pending parcel work with an accessible dispatch control.",
				API:       "function DispatchPanel({ waiting, dispatch }: DispatchPanelProps): ReactElement",
				Globals:   []string{"ReactElement"},
				Policy:    assemblyline.SourceFunctionPolicy{RequiredElementNames: []string{"button", "output"}},
			},
			available:     "interface DispatchPanelProps { readonly waiting: number; readonly dispatch: () => void }",
			observedNames: []string{"7 ready"},
			current: `function DispatchPanel({ waiting, dispatch }: DispatchPanelProps): ReactElement {
  return (
    <section aria-label="Outbound work">
      <button type="button" onClick={dispatch}>
        <output aria-live="polite">{String(waiting)} ready</output>
      </button>
    </section>
  );
}`,
			requiredFeedback: []string{
				`plain text "dispatch parcels" (case-insensitive)`,
				`computed accessible name exact text "7 ready"`,
			},
			supplementSentinel: "PARCEL_DOM_SUPPLEMENT_SENTINEL",
			behaviorTest: `const dispatch = vi.fn();
render(<DispatchPanel waiting={7} dispatch={dispatch} />);
const control = screen.getByRole('button', { name: /dispatch parcels/i });
expect(control).toHaveTextContent('7 ready');
fireEvent.click(control);
expect(dispatch).toHaveBeenCalledTimes(1);`,
		},
		{
			name: "room climate heading", functionName: "ClimatePanel",
			role: "heading", matcher: `/room temperature/i`,
			verifierSource: `async function VerifyClimate(): Promise<void> {
  expect(screen.getByRole('heading', { level: 2, name: /room temperature/i })).toBeInTheDocument();
}`,
			failureMessage: "Unable to find an accessible element with the role \"heading\" and name `/room temperature/i`.\n\nCLIMATE_DOM_SUPPLEMENT_SENTINEL <h2>21 °C</h2>",
			target: assemblyline.SourceBlock{
				ID:        "fixture.climate.presentation",
				Signature: "function ClimatePanel({ temperature, refresh }: ClimatePanelProps): ReactElement",
				Contract:  "Render a room reading with an accessible temperature heading and refresh control.",
				API:       "function ClimatePanel({ temperature, refresh }: ClimatePanelProps): ReactElement",
				Globals:   []string{"ReactElement"},
				Policy:    assemblyline.SourceFunctionPolicy{RequiredElementNames: []string{"h2", "output", "button"}},
			},
			available:     "interface ClimatePanelProps { readonly temperature: number; readonly refresh: () => void }",
			observedNames: []string{"21 °C"},
			current: `function ClimatePanel({ temperature, refresh }: ClimatePanelProps): ReactElement {
  return (
    <section aria-label="Environmental reading">
      <h2><output aria-live="polite">{String(temperature)} °C</output></h2>
      <button type="button" onClick={refresh}>Refresh reading</button>
    </section>
  );
}`,
			requiredFeedback: []string{
				`plain text "room temperature" (case-insensitive)`,
				`computed accessible name exact text "21 °C"`,
			},
			supplementSentinel: "CLIMATE_DOM_SUPPLEMENT_SENTINEL",
			behaviorTest: `const refresh = vi.fn();
render(<ClimatePanel temperature={21} refresh={refresh} />);
const heading = screen.getByRole('heading', { level: 2, name: /room temperature/i });
expect(heading).toHaveTextContent('21 °C');
fireEvent.click(screen.getByRole('button', { name: /refresh reading/i }));
expect(refresh).toHaveBeenCalledTimes(1);`,
		},
	}
}
