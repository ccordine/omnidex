package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptNullablePrimitiveClosureAcrossUnrelatedFixtures(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned TypeScript compiler")
	}
	root := t.TempDir()
	writeTypeScriptLexicalScopeFixtureProject(t, root)
	output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingTypeScriptInstallTimeout,
		"npm", directCodingNPMInstallArgs()...,
	)
	if err != nil {
		t.Fatalf("install pinned TypeScript compiler: %v\n%s", err, output)
	}
	if err := writeDirectCodingTypeScriptScopeInspector(root); err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name      string
		blockID   string
		signature string
		preamble  string
		source    string
		want      []string
	}{
		{
			name: "inventory nullable draft", blockID: "inventory.draft",
			signature: "function InventoryDraft(state: InventoryState): void",
			preamble: `type InventoryValue = null | boolean | number | string | { readonly code: string };
interface InventoryState {
  readonly count: InventoryValue;
  readonly note: InventoryValue;
  readonly visible: InventoryValue;
  readonly threshold: InventoryValue;
}
type SetStateAction<T> = T | ((previous: T) => T);
type Dispatch<A> = (value: A) => void;
declare function useState<T>(value: T | (() => T)): readonly [T, Dispatch<SetStateAction<T>>];`,
			source: `function InventoryDraft(state: InventoryState): void {
  const [count, setCount] = useState<number | null>(state.count ?? null);
  const [note, setNote] = useState<string | null>(state.note ?? null);
  const [visible, setVisible] = useState<boolean | null>(state.visible ?? null);
  const [threshold, setThreshold] = useState<number | null>(state.threshold ?? null);
  if (state.count !== undefined) setCount(state.count);
  if (state.note !== undefined) setNote(state.note);
  if (state.visible !== undefined) setVisible(state.visible);
  if (state.threshold !== undefined) setThreshold(state.threshold);
  void count;
  void note;
  void visible;
  void threshold;
}`,
			want: []string{
				"state.count ?? null", "state.note ?? null",
				"state.visible ?? null", "state.threshold ?? null",
				"state.count", "state.note", "state.visible", "state.threshold",
			},
		},
		{
			name: "travel nullable preferences", blockID: "travel.preferences",
			signature: "function TravelPreferences(state: TravelState): void",
			preamble: `type TravelValue = null | boolean | number | string | readonly string[];
interface TravelState {
  readonly direct: TravelValue;
  readonly label: TravelValue;
  readonly stops: TravelValue;
  readonly origin: TravelValue;
}
type SetStateAction<T> = T | ((previous: T) => T);
type Dispatch<A> = (value: A) => void;
declare function useState<T>(value: T | (() => T)): readonly [T, Dispatch<SetStateAction<T>>];`,
			source: `function TravelPreferences(state: TravelState): void {
  const [direct, setDirect] = useState<boolean | null>(state.direct ?? null);
  const [label, setLabel] = useState<string | null>(state.label ?? null);
  const [stops, setStops] = useState<number | null>(state.stops ?? null);
  const [origin, setOrigin] = useState<string | null>(state.origin ?? null);
  if (state.direct !== undefined) setDirect(state.direct);
  if (state.label !== undefined) setLabel(state.label);
  if (state.stops !== undefined) setStops(state.stops);
  if (state.origin !== undefined) setOrigin(state.origin);
  void direct;
  void label;
  void stops;
  void origin;
}`,
			want: []string{
				"state.direct ?? null", "state.label ?? null",
				"state.stops ?? null", "state.origin ?? null",
				"state.direct", "state.label", "state.stops", "state.origin",
			},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			document := assemblyline.SourceDocument{
				ID: fixture.blockID + ".document", Path: "src/fixture.ts",
				AdapterID: "typescript", Preamble: fixture.preamble,
				Blocks: []assemblyline.SourceBlock{{
					ID: fixture.blockID, Signature: fixture.signature,
					Contract: "Apply nullable primitive state.", API: fixture.signature,
				}},
			}
			program := directCodingProgram{
				StackID:          genericTypeScriptBrowserAdapter,
				VersionProfileID: typeScriptBrowserVersionProfileV1,
				Source:           assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{document}},
				Generated:        map[string]string{fixture.blockID: fixture.source},
			}
			progress := newDirectCodingTypeScriptCorrectionProgress()
			if err := progress.beginStage(); err != nil {
				t.Fatal(err)
			}

			for transition, wantSource := range fixture.want {
				if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
					t.Fatal(err)
				}
				diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
					context.Background(), root, program, [][]string{{"run", "typecheck"}},
				)
				if err != nil || diagnostic == nil {
					t.Fatalf("transition %d diagnostic=%+v error=%v", transition+1, diagnostic, err)
				}
				scope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *diagnostic)
				if err != nil || len(scope.DeterministicRepairs) != 1 {
					t.Fatalf("transition %d scope=%+v error=%v", transition+1, scope, err)
				}
				repair := scope.DeterministicRepairs[0]
				if repair.Source != wantSource {
					t.Fatalf("transition %d source=%q want=%q", transition+1, repair.Source, wantSource)
				}
				wantMechanism := directCodingTypeScriptPrimitiveNullishNarrowing
				if transition >= 4 {
					wantMechanism = directCodingTypeScriptPrimitiveReferenceNarrowing
				}
				if repair.Mechanism != wantMechanism || !strings.Contains(repair.Replacement, ": null") {
					t.Fatalf("transition %d repair=%+v", transition+1, repair)
				}
				authorized, err := progress.authorizeDeterministicRepair(fixture.blockID, repair)
				if err != nil || !authorized {
					t.Fatalf("transition %d authorization=%t error=%v", transition+1, authorized, err)
				}
				candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(
					program.Generated[fixture.blockID], scope,
				)
				if err != nil || !repaired {
					t.Fatalf("transition %d repaired=%t error=%v", transition+1, repaired, err)
				}
				failure, err := directCodingTypeScriptStageModelFeedback(diagnostic)
				if err != nil {
					t.Fatal(err)
				}
				if err := progress.observeDeterministic(
					fixture.blockID, diagnostic.VerificationStage, failure,
				); err != nil {
					t.Fatal(err)
				}
				if err := progress.recordDeterministicRepair(fixture.blockID, repair); err != nil {
					t.Fatal(err)
				}
				program.Generated[fixture.blockID] = candidate
			}

			if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
				t.Fatal(err)
			}
			diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
				context.Background(), root, program, [][]string{{"run", "typecheck"}},
			)
			if err != nil || diagnostic != nil {
				t.Fatalf("closed diagnostic=%+v error=%v", diagnostic, err)
			}
			if progress.stageSemanticCorrections[fixture.blockID] != 0 {
				t.Fatalf("nullable deterministic closure consumed semantic corrections")
			}
			if progress.stageDeterministicCorrections[fixture.blockID] !=
				maxDirectCodingTypeScriptStageDeterministicCorrections {
				t.Fatalf(
					"deterministic transitions=%d want=%d",
					progress.stageDeterministicCorrections[fixture.blockID],
					maxDirectCodingTypeScriptStageDeterministicCorrections,
				)
			}
		})
	}
}
