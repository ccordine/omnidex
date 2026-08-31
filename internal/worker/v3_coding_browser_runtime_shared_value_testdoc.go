package worker

import "fmt"

func genericBrowserSharedValueTestSource(capability string) string {
	return fmt.Sprintf(`const observedEmptyStates: FeatureState[] = [];

function SharedValueActionProbe({ state, actions }: FeatureViewProps) {
	observedEmptyStates.push(state);
	const count = Array.isArray(state.items) ? state.items.length : 0;
	return (
		<div>
			<button type="button" onClick={() => actions.set('items', [{ label: 'first' }])}>Seed items</button>
			<button type="button" onClick={() => actions.set('items', [{ label: 'second' }, { label: 'third' }])}>Replace items</button>
			<output aria-label="Item count">{count}</output>
		</div>
	);
}

describe('shared value boundaries', () => {
	it('deep-freezes direct and server-rendered fallbacks and keeps the empty state stable', () => {
		const directRuntime = createApplicationRuntime();
		const directFallback = { nested: [{ label: 'direct' }] };
		expect(directRuntime.read(%[1]s, directFallback)).toBe(directFallback);
		expect(Object.isFrozen(directFallback)).toBe(true);
		expect(Object.isFrozen(directFallback.nested)).toBe(true);
		expect(Object.isFrozen(directFallback.nested[0])).toBe(true);
		expect(() => { directFallback.nested[0].label = 'changed'; }).toThrow(TypeError);

		const serverFeature = createFeatureRuntime(createApplicationRuntime(), %[1]s);
		const serverFallback = { nested: [{ label: 'server' }] };
		function ServerFallbackProbe() {
			const value = useCapabilityState(serverFeature, %[1]s, serverFallback);
			return <span>{value.nested[0].label}</span>;
		}
		expect(renderToString(<ServerFallbackProbe />)).toContain('server');
		expect(Object.isFrozen(serverFallback)).toBe(true);
		expect(Object.isFrozen(serverFallback.nested)).toBe(true);
		expect(Object.isFrozen(serverFallback.nested[0])).toBe(true);

		observedEmptyStates.length = 0;
		const emptyFeature = createFeatureRuntime(createApplicationRuntime(), %[1]s);
		const rendered = render(<FeatureBoundary runtime={emptyFeature} view={SharedValueActionProbe} />);
		const first = observedEmptyStates[0];
		rendered.rerender(<FeatureBoundary runtime={emptyFeature} view={SharedValueActionProbe} />);
		expect(first).toBeDefined();
		expect(Object.isFrozen(first)).toBe(true);
		expect(observedEmptyStates.every((state) => state === first)).toBe(true);
	});

	it('accepts shared aliases and rejects unsupported publications atomically', () => {
		const runtime = createApplicationRuntime();
		const shared = { label: 'shared' };
		const valid = { left: shared, right: shared, values: [shared] };
		runtime.publish(%[1]s, valid);
		const stored = runtime.snapshot()[%[1]s] as unknown as typeof valid;
		expect(stored.left).toBe(stored.right);
		expect(stored.left).toBe(stored.values[0]);
		expect(Object.isFrozen(stored)).toBe(true);
		expect(Object.isFrozen(stored.values)).toBe(true);
		expect(Object.isFrozen(shared)).toBe(true);

		const accessor = {};
		let getterCalled = false;
		Object.defineProperty(accessor, 'value', {
			enumerable: true,
			get() { getterCalled = true; return true; },
		});
		const hidden = {};
		Object.defineProperty(hidden, 'value', { value: true });
		const symbolKey = {};
		Object.defineProperty(symbolKey, Symbol('hidden'), { value: true, enumerable: true });
		const sparse: unknown[] = [];
		sparse.length = 1;
		const extra = [] as unknown[] & { extra?: boolean };
		extra.extra = true;
		class CustomArray extends Array<unknown> {}
		const customArray = new CustomArray();
		const customPrototype = Object.create({ inherited: true });
		const cycle: { self?: unknown } = {};
		cycle.self = cycle;
		const validPrefix = { retained: true };
		const invalidTail = { validPrefix, invalid: new Date() };
		const tooDeep: { next?: unknown } = {};
		let cursor = tooDeep;
		for (let depth = 0; depth <= 64; depth += 1) {
			const next: { next?: unknown } = {};
			cursor.next = next;
			cursor = next;
		}
		const tooWide = Array.from({ length: 10000 }, () => null);
		const invalid: unknown[] = [
			undefined, () => true, 1n, Symbol('value'), new Date(), new Map(), new Set(),
			accessor, hidden, symbolKey, sparse, extra, customArray, customPrototype, cycle, invalidTail,
			tooDeep, tooWide,
		];
		const before = runtime.snapshot();
		let capabilityChanges = 0;
		let allChanges = 0;
		runtime.subscribe(%[1]s, () => { capabilityChanges += 1; });
		runtime.subscribeAll(() => { allChanges += 1; });
		for (const candidate of invalid) {
			expect(() => runtime.publish(%[1]s, candidate as unknown as SharedValue)).toThrow(/Shared value publication/);
		}
		expect(runtime.snapshot()).toBe(before);
		expect(capabilityChanges).toBe(0);
		expect(allChanges).toBe(0);
		expect(getterCalled).toBe(false);
		expect(Object.isFrozen(validPrefix)).toBe(false);
	});

	it('keeps repeated set updates immutable', async () => {
		observedEmptyStates.length = 0;
		const feature = createFeatureRuntime(createApplicationRuntime(), %[1]s);
		render(<FeatureBoundary runtime={feature} view={SharedValueActionProbe} />);
		fireEvent.click(screen.getByRole('button', { name: 'Seed items' }));
		await waitFor(() => expect(screen.getByRole('status', { name: 'Item count' })).toHaveTextContent(/^1$/));
		const firstState = feature.application.snapshot()[%[1]s] as FeatureState;
		const firstItems = firstState.items as readonly SharedValue[];

		fireEvent.click(screen.getByRole('button', { name: 'Replace items' }));
		await waitFor(() => expect(screen.getByRole('status', { name: 'Item count' })).toHaveTextContent(/^2$/));
		const secondState = feature.application.snapshot()[%[1]s] as FeatureState;
		const secondItems = secondState.items as readonly SharedValue[];
		expect(secondState).not.toBe(firstState);
		expect(secondItems).not.toBe(firstItems);
		expect(firstItems).toEqual([{ label: 'first' }]);
		expect(secondItems).toEqual([{ label: 'second' }, { label: 'third' }]);
		expect(Object.isFrozen(secondState)).toBe(true);
		expect(Object.isFrozen(secondItems)).toBe(true);
		expect(Object.isFrozen(secondItems[0] as unknown as object)).toBe(true);
		expect(() => { (secondItems[0] as unknown as { label: string }).label = 'changed'; }).toThrow(TypeError);
	});
});`, capability)
}
