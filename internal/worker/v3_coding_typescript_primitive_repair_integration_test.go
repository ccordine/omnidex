package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypeScriptCompilerScopeInspectorProjectsRealUnionMismatch(t *testing.T) {
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
	for _, testCase := range []struct {
		name, source, marker, wantExpression, wantBinding, wantReplacement string
	}{
		{
			name: "numeric hook state",
			source: `import { useState } from 'react';
type MetricValue = null | boolean | number | string;
type MetricState = { readonly [key: string]: MetricValue };
function ReadMetric(state: MetricState): number {
  const [reading] = useState<number>(() => state.reading ?? 0);
  return reading;
}`,
			marker: "() => state.reading", wantExpression: "state.reading ?? 0", wantBinding: "state",
			wantReplacement: "typeof state.reading === 'number' ? state.reading : 0",
		},
		{
			name: "text hook state",
			source: `import { useState } from 'react';
type LabelValue = null | boolean | number | string;
type LabelState = { readonly [key: string]: LabelValue };
function ReadLabel(state: LabelState): string {
  const [label] = useState<string>(() => state.label ?? '');
  return label;
}`,
			marker: "() => state.label", wantExpression: "state.label ?? ''", wantBinding: "state",
			wantReplacement: "typeof state.label === 'string' ? state.label : ''",
		},
		{
			name: "direct numeric hook state",
			source: `import { useState } from 'react';
type CapacityValue = null | boolean | number | string;
type CapacityState = { readonly [key: string]: CapacityValue };
function ReadCapacity(state: CapacityState): number {
  const [capacity] = useState<number>(state.capacity ?? 0);
  return capacity;
}`,
			marker: "state.capacity ?? 0", wantExpression: "state.capacity ?? 0", wantBinding: "state",
			wantReplacement: "typeof state.capacity === 'number' ? state.capacity : 0",
		},
		{
			name: "direct text hook state",
			source: `import { useState } from 'react';
type DestinationValue = null | boolean | number | string;
type DestinationState = { readonly [key: string]: DestinationValue };
function ReadDestination(state: DestinationState): string {
  const [destination] = useState<string>(state.destination ?? '');
  return destination;
}`,
			marker: "state.destination ?? ''", wantExpression: "state.destination ?? ''", wantBinding: "state",
			wantReplacement: "typeof state.destination === 'string' ? state.destination : ''",
		},
		{
			name: "direct boolean hook state",
			source: `import { useState } from 'react';
type ToggleValue = null | boolean | number | string;
type ToggleState = { readonly [key: string]: ToggleValue };
function ReadEnabled(state: ToggleState): boolean {
  const [enabled] = useState<boolean>(state.enabled ?? false);
  return enabled;
}`,
			marker: "state.enabled ?? false", wantExpression: "state.enabled ?? false", wantBinding: "state",
			wantReplacement: "typeof state.enabled === 'boolean' ? state.enabled : false",
		},
		{
			name: "generic callback or numeric value",
			source: `declare const initialize: <T>(value: T | (() => T)) => T;
type DelayValue = null | boolean | number | string;
interface DelayPolicy { readonly delay: DelayValue }
function ReadDelay(policy: DelayPolicy): number {
  const delay = initialize<number>(policy.delay ?? 0);
  return delay;
}`,
			marker: "policy.delay ?? 0", wantExpression: "policy.delay ?? 0", wantBinding: "policy",
			wantReplacement: "typeof policy.delay === 'number' ? policy.delay : 0",
		},
		{
			name: "numeric reading",
			source: `type MetricValue = null | boolean | number | string;
interface MetricState { readonly reading: MetricValue }
function ReadMetric(state: MetricState): number {
  const reading: number = state.reading ?? 0;
  return reading;
}`,
			marker: "state.reading ?? 0", wantExpression: "state.reading ?? 0", wantBinding: "state",
			wantReplacement: "typeof state.reading === 'number' ? state.reading : 0",
		},
		{
			name: "text label",
			source: `type LabelValue = null | number | string;
interface LabelState { readonly label: LabelValue }
function ReadLabel(entry: LabelState): string {
  const label: string = entry.label ?? '';
  return label;
}`,
			marker: "entry.label ?? ''", wantExpression: "entry.label ?? ''", wantBinding: "entry",
			wantReplacement: "typeof entry.label === 'string' ? entry.label : ''",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "src/fixture.ts"), []byte(testCase.source), 0o600); err != nil {
				t.Fatal(err)
			}
			line, column := typeScriptLexicalScopeTestLocation(t, testCase.source, testCase.marker)
			functionLine, _ := typeScriptLexicalScopeTestLocation(t, testCase.source, "function ")
			scope, err := inspectDirectCodingTypeScriptScope(
				context.Background(), root,
				directCodingStageDiagnostic{
					CompilerIssue: true, DocumentPath: "src/fixture.ts",
					DocumentLine: line, DocumentColumn: column,
					DocumentBlockStartLine: functionLine,
					DocumentBlockEndLine:   strings.Count(testCase.source, "\n") + 1,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(scope.ExpressionEvidence) != 1 ||
				scope.ExpressionEvidence[0].Source != testCase.wantExpression {
				t.Fatalf("compiler expression projection=%+v", scope.ExpressionEvidence)
			}
			if len(scope.Bindings) != 1 || scope.Bindings[0].Name != testCase.wantBinding {
				t.Fatalf("compiler binding projection=%+v", scope.Bindings)
			}
			if len(scope.DeterministicRepairs) != 1 ||
				scope.DeterministicRepairs[0].Replacement != testCase.wantReplacement {
				t.Fatalf("deterministic repair projection=%+v", scope.DeterministicRepairs)
			}
			declarationStart := strings.Index(testCase.source, "function ")
			if declarationStart < 0 {
				t.Fatal("fixture lacks function declaration")
			}
			current := testCase.source[declarationStart:]
			candidate, repaired, err := applyDirectCodingTypeScriptDeterministicRepair(current, scope)
			if err != nil {
				t.Fatal(err)
			}
			if !repaired || !strings.Contains(candidate, testCase.wantReplacement) {
				t.Fatalf("candidate=%q repaired=%t", candidate, repaired)
			}
			if err := os.WriteFile(
				filepath.Join(root, "src/fixture.ts"),
				[]byte(testCase.source[:declarationStart]+candidate), 0o600,
			); err != nil {
				t.Fatal(err)
			}
			output, err := runDirectCodingStageCommand(
				context.Background(), root, directCodingStageTimeout,
				"npm", "run", "typecheck",
			)
			if err != nil {
				t.Fatalf("deterministic repair did not compile: %v\n%s", err, output)
			}
		})
	}

	t.Run("side effecting value is not rewritten", func(t *testing.T) {
		source := `type MetricValue = null | boolean | number | string;
declare function readMetric(): MetricValue;
function ReadMetric(): number {
  const reading: number = readMetric() ?? 0;
  return reading;
}`
		if err := os.WriteFile(filepath.Join(root, "src/fixture.ts"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		line, column := typeScriptLexicalScopeTestLocation(t, source, "readMetric() ?? 0")
		functionOffset := strings.LastIndex(source, "function ReadMetric")
		functionLine := strings.Count(source[:functionOffset], "\n") + 1
		scope, err := inspectDirectCodingTypeScriptScope(
			context.Background(), root,
			directCodingStageDiagnostic{
				CompilerIssue: true, DocumentPath: "src/fixture.ts",
				DocumentLine: line, DocumentColumn: column,
				DocumentBlockStartLine: functionLine,
				DocumentBlockEndLine:   strings.Count(source, "\n") + 1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(scope.DeterministicRepairs) != 0 {
			t.Fatalf("side-effecting expression received deterministic repair: %+v", scope.DeterministicRepairs)
		}
	})

	t.Run("compatible callable value is not rewritten", func(t *testing.T) {
		source := `type DeferredLabel = () => string;
type LabelValue = null | number | string | DeferredLabel;
interface LabelState { readonly label: LabelValue }
declare function initialize(value: string | DeferredLabel): string;
function ReadLabel(state: LabelState): string {
  return initialize(state.label ?? '');
}`
		if err := os.WriteFile(filepath.Join(root, "src/fixture.ts"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		line, column := typeScriptLexicalScopeTestLocation(t, source, "state.label ?? ''")
		functionOffset := strings.LastIndex(source, "function ReadLabel")
		functionLine := strings.Count(source[:functionOffset], "\n") + 1
		scope, err := inspectDirectCodingTypeScriptScope(
			context.Background(), root,
			directCodingStageDiagnostic{
				CompilerIssue: true, DocumentPath: "src/fixture.ts",
				DocumentLine: line, DocumentColumn: column,
				DocumentBlockStartLine: functionLine,
				DocumentBlockEndLine:   strings.Count(source, "\n") + 1,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(scope.DeterministicRepairs) != 0 {
			t.Fatalf("compatible callable value received deterministic repair: %+v", scope.DeterministicRepairs)
		}
	})
}
