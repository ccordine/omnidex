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
}`,
			source: `function InventorySummary(defaults: InventoryDefaults): string {
  const title: string = defaults.title ?? '';
  const count: number = defaults.count ?? 0;
  const visible: boolean = defaults.visible ?? false;
  const note: string = defaults.note ?? '';
  return title + count.toString() + visible.toString() + note;
}`,
			want: []string{
				"defaults.title ?? ''", "defaults.count ?? 0",
				"defaults.visible ?? false", "defaults.note ?? ''",
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
}`,
			source: `function TravelPreferences(preferences: TravelPreferencesInput): string {
  const origin: string = preferences.origin ?? '';
  const destination: string = preferences.destination ?? '';
  const stops: number = preferences.stops ?? 0;
  const direct: boolean = preferences.direct ?? false;
  return origin + destination + stops.toString() + direct.toString();
}`,
			want: []string{
				"preferences.origin ?? ''", "preferences.destination ?? ''",
				"preferences.stops ?? 0", "preferences.direct ?? false",
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
				program.Generated[fixture.blockID] = candidate
				transitions++
			}
			if transitions != len(fixture.want) || transitions <= maxDirectCodingTypeScriptStageSemanticCorrections {
				t.Fatalf("deterministic transitions=%d want=%d and greater than semantic limit", transitions, len(fixture.want))
			}
		})
	}
}
