package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptDeterministicClosureExceedsSemanticBudgetAcrossUnrelatedFixtures(t *testing.T) {
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
			name: "inventory defaults", blockID: "inventory.summary",
			signature: "function InventorySummary(defaults: InventoryDefaults): string",
			preamble: `type InventoryValue = null | boolean | number | string;
interface InventoryDefaults {
  readonly title: InventoryValue;
  readonly count: InventoryValue;
  readonly visible: InventoryValue;
  readonly note: InventoryValue;
}
declare function recordText(value: string | ((previous: string) => string)): void;
declare function recordNumber(value: number | ((previous: number) => number)): void;
declare function recordBoolean(value: boolean | ((previous: boolean) => boolean)): void;`,
			source: `function InventorySummary(defaults: InventoryDefaults): string {
  const title: string = defaults.title ?? '';
  const count: number = defaults.count ?? 0;
  const visible: boolean = defaults.visible ?? false;
  const note: string = defaults.note ?? '';
  recordText(defaults.title);
  recordNumber(defaults.count);
  recordBoolean(defaults.visible);
  recordText(defaults.note);
  return title + count.toString() + visible.toString() + note;
}`,
			want: []string{
				"defaults.title ?? ''", "defaults.count ?? 0",
				"defaults.visible ?? false", "defaults.note ?? ''",
				"defaults.title", "defaults.count", "defaults.visible", "defaults.note",
			},
		},
		{
			name: "travel preferences", blockID: "travel.preferences",
			signature: "function TravelPreferences(preferences: TravelPreferencesInput): string",
			preamble: `type PreferenceValue = null | boolean | number | string;
interface TravelPreferencesInput {
  readonly origin: PreferenceValue;
  readonly destination: PreferenceValue;
  readonly stops: PreferenceValue;
  readonly direct: PreferenceValue;
}
declare function recordText(value: string | ((previous: string) => string)): void;
declare function recordNumber(value: number | ((previous: number) => number)): void;
declare function recordBoolean(value: boolean | ((previous: boolean) => boolean)): void;`,
			source: `function TravelPreferences(preferences: TravelPreferencesInput): string {
  const origin: string = preferences.origin ?? '';
  const destination: string = preferences.destination ?? '';
  const stops: number = preferences.stops ?? 0;
  const direct: boolean = preferences.direct ?? false;
  recordText(preferences.origin);
  recordText(preferences.destination);
  recordNumber(preferences.stops);
  recordBoolean(preferences.direct);
  return origin + destination + stops.toString() + direct.toString();
}`,
			want: []string{
				"preferences.origin ?? ''", "preferences.destination ?? ''",
				"preferences.stops ?? 0", "preferences.direct ?? false",
				"preferences.origin", "preferences.destination", "preferences.stops", "preferences.direct",
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
					Contract: "Return one summary from typed primitive defaults.", API: fixture.signature,
				}},
			}
			program := directCodingProgram{
				StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
				Source:    assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{document}},
				Generated: map[string]string{fixture.blockID: fixture.source},
			}
			progress := newDirectCodingTypeScriptCorrectionProgress()
			if err := progress.beginStage(); err != nil {
				t.Fatal(err)
			}

			transitions := 0
			for {
				if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
					t.Fatal(err)
				}
				diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
					context.Background(), root, program, [][]string{{"run", "typecheck"}},
				)
				if err != nil {
					t.Fatal(err)
				}
				if diagnostic == nil {
					break
				}
				if transitions >= len(fixture.want) {
					t.Fatalf("unexpected additional compiler transition: %+v", diagnostic)
				}
				scope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *diagnostic)
				if err != nil {
					t.Fatal(err)
				}
				if len(scope.DeterministicRepairs) != 1 ||
					scope.DeterministicRepairs[0].Source != fixture.want[transitions] {
					t.Fatalf("transition %d repair projection=%+v", transitions+1, scope.DeterministicRepairs)
				}
				wantMechanism := directCodingTypeScriptPrimitiveNullishNarrowing
				if transitions >= 4 {
					wantMechanism = directCodingTypeScriptPrimitiveReferenceNarrowing
				}
				if scope.DeterministicRepairs[0].Mechanism != wantMechanism {
					t.Fatalf("transition %d mechanism=%q want=%q", transitions+1, scope.DeterministicRepairs[0].Mechanism, wantMechanism)
				}
				authorized, err := progress.authorizeDeterministicRepair(
					fixture.blockID, scope.DeterministicRepairs[0],
				)
				if err != nil || !authorized {
					t.Fatalf("transition %d authorization: %v", transitions+1, err)
				}
				candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(
					program.Generated[fixture.blockID], scope,
				)
				if err != nil {
					t.Fatal(err)
				}
				if !repaired || strings.TrimSpace(candidate) == strings.TrimSpace(program.Generated[fixture.blockID]) {
					t.Fatalf("transition %d did not produce an exact source delta", transitions+1)
				}
				failure, err := directCodingTypeScriptStageModelFeedback(diagnostic)
				if err != nil {
					t.Fatal(err)
				}
				if err := progress.observeDeterministic(
					fixture.blockID, diagnostic.VerificationStage, failure,
				); err != nil {
					t.Fatalf("transition %d was blocked: %v", transitions+1, err)
				}
				if err := progress.recordDeterministicRepair(
					fixture.blockID, scope.DeterministicRepairs[0],
				); err != nil {
					t.Fatal(err)
				}
				program.Generated[fixture.blockID] = candidate
				transitions++
			}
			if transitions != len(fixture.want) || transitions <= maxDirectCodingTypeScriptStageSemanticCorrections {
				t.Fatalf("deterministic transitions=%d want=%d and greater than semantic limit", transitions, len(fixture.want))
			}
		})
	}

	for _, fixture := range []struct {
		name, blockID, signature, preamble, source string
	}{
		{
			name: "side effecting finance fallback", blockID: "finance.label",
			signature: "function FinanceLabel(state: FinanceState): void",
			preamble: `type FinanceValue = null | number | string;
interface FinanceState { readonly label: FinanceValue }
declare function readFinanceFallback(): string;
declare function recordText(value: string | ((previous: string) => string)): void;`,
			source: `function FinanceLabel(state: FinanceState): void {
  const label: string = typeof state.label === 'string' ? state.label : readFinanceFallback();
  recordText(state.label);
  void label;
}`,
		},
		{
			name: "ambiguous profile fallbacks", blockID: "profile.label",
			signature: "function ProfileLabel(state: ProfileState): void",
			preamble: `type ProfileValue = null | number | string;
interface ProfileState { readonly label: ProfileValue }
declare function recordText(value: string | ((previous: string) => string)): void;`,
			source: `function ProfileLabel(state: ProfileState): void {
  const primary: string = typeof state.label === 'string' ? state.label : 'primary';
  const secondary: string = typeof state.label === 'string' ? state.label : 'secondary';
  recordText(state.label);
  void primary;
			void secondary;
}`,
		},
		{
			name: "shadowed settings reference", blockID: "settings.label",
			signature: "function SettingsLabel(state: SettingsState): void",
			preamble: `type SettingsValue = null | number | string;
interface SettingsState { readonly label: SettingsValue }
declare function recordText(value: string | ((previous: string) => string)): void;`,
			source: `function SettingsLabel(state: SettingsState): void {
  const nested = (() => {
    const state: { readonly label: number | string } = { label: 'nested' };
    return typeof state.label === 'string' ? state.label : 'nested';
  })();
  recordText(state.label);
  void nested;
}`,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			document := assemblyline.SourceDocument{
				ID: fixture.blockID + ".document", Path: "src/fixture.ts",
				AdapterID: "typescript", Preamble: fixture.preamble,
				Blocks: []assemblyline.SourceBlock{{
					ID: fixture.blockID, Signature: fixture.signature,
					Contract: "Apply one typed label value.", API: fixture.signature,
				}},
			}
			program := directCodingProgram{
				StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
				Source:    assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{document}},
				Generated: map[string]string{fixture.blockID: fixture.source},
			}
			if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
				t.Fatal(err)
			}
			diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
				context.Background(), root, program, [][]string{{"run", "typecheck"}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if diagnostic == nil || !diagnostic.CompilerIssue {
				t.Fatal("fixture did not produce one compiler-owned contextual mismatch")
			}
			scope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			if len(scope.DeterministicRepairs) != 0 {
				t.Fatalf("inexact fallback authority produced deterministic repair: %+v", scope.DeterministicRepairs)
			}
		})
	}
}
