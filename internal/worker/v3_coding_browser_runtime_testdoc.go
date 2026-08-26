package worker

import (
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericBrowserRuntimeTestDocument(
	requirements []assemblyline.Requirement,
) assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_runtime_test", Path: "src/runtime.test.tsx",
		Preamble: `import '@testing-library/jest-dom/vitest';
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { FeatureBoundary, createApplicationRuntime, createFeatureRuntime, publishCapability, useCapabilityState, useOwnCapabilityState } from './runtime';
import type { CapabilityID, FeatureViewProps } from './runtime';`,
		Blocks: []assemblyline.SourceBlock{{
			ID: "tests.runtime", Static: genericBrowserRuntimeTestSource(requirements),
			API: "tests code-owned application capability runtime", DependsOn: []string{"runtime.factory"},
		}},
	}
}

func genericBrowserRuntimeTestSource(requirements []assemblyline.Requirement) string {
	capability := strconv.Quote(genericApplicationCapabilityID(1))
	return fmt.Sprintf(`function ActionProbe({ state, actions }: FeatureViewProps) {
	return <button onClick={() => actions.set('ready', true)}>{String(state.ready ?? false)}</button>;
}

describe('application runtime', () => {
	it('shares one capability state between independent renderers', () => {
		const runtime = createApplicationRuntime();
		const feature = createFeatureRuntime(runtime, %s);
		const owner = renderHook(() => useOwnCapabilityState(feature, 'idle'));
		const observer = renderHook(() => useCapabilityState(feature, %s, 'idle'));
		act(() => publishCapability(feature, 'working'));
		expect(observer.result.current).toBe('working');
		expect(owner.result.current[0]).toBe('working');
	});

	it('commits feature actions to the observable capability state', async () => {
		const runtime = createFeatureRuntime(createApplicationRuntime(), %s);
		render(<FeatureBoundary runtime={runtime} view={ActionProbe} />);
		fireEvent.click(screen.getByRole('button'));
		expect(screen.getByRole('button').textContent).toBe('true');
		await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Updated.'));
	});

	it('rejects identifiers outside the code-owned capability graph', () => {
		const runtime = createApplicationRuntime();
		expect(() => createFeatureRuntime(runtime, 'unknown' as CapabilityID)).toThrow(/Unknown application capability/);
	});
});`, capability, capability, capability)
}
