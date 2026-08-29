package worker

import (
	"context"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptReferenceNormalizationRequiresExactCodeOwnedOccurrence(t *testing.T) {
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

	t.Run("model authored normalization remains semantic", func(t *testing.T) {
		program := typeScriptNormalizationAuthorityProgram(`function LedgerEntry(state: LedgerState): void {
  const normalized: string = typeof state.value === 'string' ? state.value : '';
  recordText(state.value);
  void normalized;
}`)
		if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
			t.Fatal(err)
		}
		diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
			context.Background(), root, program, [][]string{{"run", "typecheck"}},
		)
		if err != nil || diagnostic == nil {
			t.Fatalf("compiler diagnostic=%+v error=%v", diagnostic, err)
		}
		scope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *diagnostic)
		if err != nil {
			t.Fatal(err)
		}
		if len(scope.DeterministicRepairs) != 1 ||
			scope.DeterministicRepairs[0].Mechanism != directCodingTypeScriptPrimitiveReferenceNarrowing {
			t.Fatalf("expected one optional reference candidate, got %+v", scope.DeterministicRepairs)
		}
		progress := newDirectCodingTypeScriptCorrectionProgress()
		if err := progress.beginStage(); err != nil {
			t.Fatal(err)
		}
		authorized, err := progress.authorizeDeterministicRepair(
			"ledger.entry", scope.DeterministicRepairs[0],
		)
		if err != nil || authorized {
			t.Fatalf("unowned candidate authorization=%t error=%v", authorized, err)
		}
		scope.DeterministicRepairs = nil
		candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(
			program.Generated["ledger.entry"], scope,
		)
		if err != nil || repaired || candidate != program.Generated["ledger.entry"] {
			t.Fatalf("unowned candidate bypass candidate=%q repaired=%t error=%v", candidate, repaired, err)
		}
	})

	t.Run("model authored nullable normalization remains semantic", func(t *testing.T) {
		program := typeScriptNormalizationAuthorityProgram(`function LedgerEntry(state: LedgerState): void {
  const normalized: string | null = typeof state.value === 'string' ? state.value : null;
  recordNullableText(state.value);
  void normalized;
}`)
		if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
			t.Fatal(err)
		}
		diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
			context.Background(), root, program, [][]string{{"run", "typecheck"}},
		)
		if err != nil || diagnostic == nil {
			t.Fatalf("compiler diagnostic=%+v error=%v", diagnostic, err)
		}
		scope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *diagnostic)
		if err != nil {
			t.Fatal(err)
		}
		if len(scope.DeterministicRepairs) != 1 ||
			scope.DeterministicRepairs[0].Mechanism != directCodingTypeScriptPrimitiveReferenceNarrowing ||
			scope.DeterministicRepairs[0].Replacement !=
				"typeof state.value === 'string' ? state.value : null" {
			t.Fatalf("expected one optional nullable reference candidate, got %+v", scope.DeterministicRepairs)
		}
		progress := newDirectCodingTypeScriptCorrectionProgress()
		if err := progress.beginStage(); err != nil {
			t.Fatal(err)
		}
		authorized, err := progress.authorizeDeterministicRepair(
			"ledger.entry", scope.DeterministicRepairs[0],
		)
		if err != nil || authorized {
			t.Fatalf("unowned nullable candidate authorization=%t error=%v", authorized, err)
		}
	})

	t.Run("multiline model authored normalization remains semantic", func(t *testing.T) {
		program := typeScriptNormalizationAuthorityProgram(`function LedgerEntry(state: LedgerState): void {
  const normalized: string =
    typeof state.value === 'string'
      ? state.value
      : '';
  recordText(state.value);
  void normalized;
}`)
		if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
			t.Fatal(err)
		}
		diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
			context.Background(), root, program, [][]string{{"run", "typecheck"}},
		)
		if err != nil || diagnostic == nil {
			t.Fatalf("compiler diagnostic=%+v error=%v", diagnostic, err)
		}
		scope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *diagnostic)
		if err != nil {
			t.Fatal(err)
		}
		if len(scope.DeterministicRepairs) != 0 {
			t.Fatalf("multiline unowned normalization produced deterministic repair: %+v", scope.DeterministicRepairs)
		}
	})

	t.Run("shadowed equal bytes do not collide", func(t *testing.T) {
		program := typeScriptNormalizationAuthorityProgram(`function LedgerEntry(state: LedgerState): void {
  const nested = (() => {
    const state: LedgerState = { value: null };
    const normalized: string = state.value ?? '';
    return normalized;
  })();
  const normalized: string = typeof state.value === 'string' ? state.value : '';
  recordText(state.value);
  void nested;
  void normalized;
}`)
		progress := newDirectCodingTypeScriptCorrectionProgress()
		if err := progress.beginStage(); err != nil {
			t.Fatal(err)
		}
		if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
			t.Fatal(err)
		}
		first, err := verifyDirectCodingTypeScriptStageCommands(
			context.Background(), root, program, [][]string{{"run", "typecheck"}},
		)
		if err != nil || first == nil {
			t.Fatalf("first compiler diagnostic=%+v error=%v", first, err)
		}
		firstScope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *first)
		if err != nil || len(firstScope.DeterministicRepairs) != 1 {
			t.Fatalf("first scope=%+v error=%v", firstScope, err)
		}
		owned := firstScope.DeterministicRepairs[0]
		if owned.Mechanism != directCodingTypeScriptPrimitiveNullishNarrowing {
			t.Fatalf("first mechanism=%q", owned.Mechanism)
		}
		candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(
			program.Generated["ledger.entry"], firstScope,
		)
		if err != nil || !repaired {
			t.Fatalf("first deterministic repair=%t error=%v", repaired, err)
		}
		if err := progress.recordDeterministicRepair("ledger.entry", owned); err != nil {
			t.Fatal(err)
		}
		program.Generated["ledger.entry"] = candidate
		if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
			t.Fatal(err)
		}
		second, err := verifyDirectCodingTypeScriptStageCommands(
			context.Background(), root, program, [][]string{{"run", "typecheck"}},
		)
		if err != nil || second == nil {
			t.Fatalf("second compiler diagnostic=%+v error=%v", second, err)
		}
		secondScope, err := inspectDirectCodingTypeScriptScope(context.Background(), root, *second)
		if err != nil || len(secondScope.DeterministicRepairs) != 1 {
			t.Fatalf("second scope=%+v error=%v", secondScope, err)
		}
		unowned := secondScope.DeterministicRepairs[0]
		if unowned.Mechanism != directCodingTypeScriptPrimitiveReferenceNarrowing ||
			unowned.Replacement != owned.Replacement ||
			*unowned.NormalizationStartByte == *owned.NormalizationStartByte {
			t.Fatalf("occurrence receipts owned=%+v unowned=%+v", owned, unowned)
		}
		authorized, err := progress.authorizeDeterministicRepair("ledger.entry", unowned)
		if err != nil || authorized {
			t.Fatalf("shadowed equal-byte occurrence authorization=%t error=%v", authorized, err)
		}
	})
}

func typeScriptNormalizationAuthorityProgram(source string) directCodingProgram {
	const blockID = "ledger.entry"
	document := assemblyline.SourceDocument{
		ID: "ledger", Path: "src/fixture.ts", AdapterID: "typescript",
		Preamble: `type LedgerValue = null | number | string;
interface LedgerState { readonly value: LedgerValue }
declare function recordText(value: string | ((previous: string) => string)): void;
declare function recordNullableText(value: string | null | ((previous: string | null) => string | null)): void;`,
		Blocks: []assemblyline.SourceBlock{{
			ID: blockID, Signature: "function LedgerEntry(state: LedgerState): void",
			Contract: "Apply one typed ledger value.", API: "function LedgerEntry(state: LedgerState): void",
		}},
	}
	return directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		Source:    assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{document}},
		Generated: map[string]string{blockID: source},
	}
}
