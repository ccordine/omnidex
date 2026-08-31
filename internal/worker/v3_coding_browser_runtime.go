package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericBrowserRuntimeDocument(
	requirements []assemblyline.Requirement,
) assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "src/runtime.tsx",
		Preamble: `import { useCallback, useMemo, useSyncExternalStore } from 'react';
import type { ReactElement } from 'react';`,
		Blocks: []assemblyline.SourceBlock{
			{
				ID: "runtime.api", Static: genericBrowserRuntimeSource(requirements),
				API: genericBrowserRuntimeAPI(requirements),
			},
			{
				ID: "runtime.factory", Static: genericBrowserRuntimeFactorySource(),
				API: genericBrowserRuntimeFactoryAPI(), DependsOn: []string{"runtime.api"},
			},
		},
	}
}

func genericBrowserRuntimeAPI(requirements []assemblyline.Requirement) string {
	return strings.Join([]string{
		"type SharedValue = null | boolean | number | string | readonly SharedValue[] | { readonly [key: string]: SharedValue }",
		"type FeatureState = { readonly [key: string]: SharedValue }",
		"type CapabilityID = " + genericBrowserCapabilityUnion(requirements),
		"type CapabilitySnapshot = Readonly<Partial<Record<CapabilityID, SharedValue>>>",
		genericBrowserFeatureActionsAPI(),
		"interface FeatureViewProps { state: FeatureState; capabilities: CapabilitySnapshot; actions: FeatureActions }",
	}, "\n")
}

func genericBrowserFeatureActionsAPI() string {
	return `interface FeatureActions {
  set(key: string, value: SharedValue): void;
}`
}

func genericBrowserCapabilityUnion(requirements []assemblyline.Requirement) string {
	values := make([]string, 0, len(requirements))
	for index := range requirements {
		values = append(values, strconv.Quote(genericApplicationCapabilityID(index+1)))
	}
	return strings.Join(values, " | ")
}

func genericApplicationCapabilityID(sequence int) string {
	return fmt.Sprintf("capability_%03d", sequence)
}

func genericBrowserRuntimeSource(requirements []assemblyline.Requirement) string {
	allowed := make([]string, 0, len(requirements))
	for index := range requirements {
		allowed = append(allowed, strconv.Quote(genericApplicationCapabilityID(index+1)))
	}
	return fmt.Sprintf(`export type SharedValue = null | boolean | number | string | readonly SharedValue[] | { readonly [key: string]: SharedValue };
export type WidenShared<T extends SharedValue> = T extends string ? string : T extends number ? number : T extends boolean ? boolean : T;
export type FeatureState = { readonly [key: string]: SharedValue };
export type CapabilityID = %s;
export type CapabilitySnapshot = Readonly<Partial<Record<CapabilityID, SharedValue>>>;
export interface FeatureActions {
  set(key: string, value: SharedValue): void;
}
export interface FeatureViewProps {
  state: FeatureState;
  capabilities: CapabilitySnapshot;
  actions: FeatureActions;
}
type ChangeListener = () => void;

%s

const emptyFeatureState: FeatureState = Object.freeze({});

export class ApplicationRuntime {
	private readonly allowed = new Set<string>([%s]);
	private readonly changes = new Map<string, Set<ChangeListener>>();
	private readonly allChanges = new Set<ChangeListener>();
	private snapshotValue: CapabilitySnapshot;

	constructor() {
		this.snapshotValue = Object.freeze({});
	}

	assertCapability(capability: string): void {
		if (!this.allowed.has(capability)) throw new Error('Unknown application capability: ' + capability);
	}

	read<T extends SharedValue>(capability: CapabilityID, initial: T): T {
		this.assertCapability(capability);
		const frozenInitial = validateAndFreezeSharedValue(initial, 'initial value for ' + capability);
		if (!Object.hasOwn(this.snapshotValue, capability)) return frozenInitial;
		const value = this.snapshotValue[capability];
		return value as T;
	}

	subscribe(capability: CapabilityID, listener: ChangeListener): () => void {
    this.assertCapability(capability);
    const listeners = this.changes.get(capability) ?? new Set<ChangeListener>();
    listeners.add(listener);
    this.changes.set(capability, listeners);
		return () => listeners.delete(listener);
	}

	subscribeAll(listener: ChangeListener): () => void {
		this.allChanges.add(listener);
		return () => this.allChanges.delete(listener);
	}

	snapshot(): CapabilitySnapshot { return this.snapshotValue; }

	publish<T extends SharedValue>(capability: CapabilityID, value: T): void {
		this.assertCapability(capability);
		const frozenValue = validateAndFreezeSharedValue(value, 'publication for ' + capability);
		this.snapshotValue = Object.freeze({ ...this.snapshotValue, [capability]: frozenValue });
		this.changes.get(capability)?.forEach((listener) => listener());
		this.allChanges.forEach((listener) => listener());
	}
}

export class FeatureRuntime {
  constructor(
    readonly application: ApplicationRuntime,
    readonly capability: CapabilityID,
  ) { application.assertCapability(capability); }
}

export interface FeatureProps { runtime: FeatureRuntime }

interface FeatureBoundaryProps {
  readonly runtime: FeatureRuntime;
  readonly view: (props: FeatureViewProps) => ReactElement;
}

function useCapabilityValue<T extends SharedValue>(
  runtime: ApplicationRuntime, capability: CapabilityID, fallback: T,
): readonly [T, (next: T) => void] {
	const frozenFallback = useMemo(
		() => validateAndFreezeSharedValue(fallback, 'hook fallback for ' + capability),
		[capability, fallback],
	);
  const value = useSyncExternalStore(
    (listener) => runtime.subscribe(capability, listener),
    () => runtime.read(capability, frozenFallback),
    () => frozenFallback,
  );
	const setValue = useCallback((next: T) => runtime.publish(capability, next), [runtime, capability]);
  return [value, setValue] as const;
}

export function useOwnCapabilityState<T extends SharedValue>(
	runtime: FeatureRuntime, initial: T,
): readonly [WidenShared<T>, (next: WidenShared<T>) => void] {
	return useCapabilityValue(runtime.application, runtime.capability, initial) as unknown as readonly [WidenShared<T>, (next: WidenShared<T>) => void];
}

export function useCapabilityState<T extends SharedValue>(
	runtime: FeatureRuntime, capability: CapabilityID, fallback: T,
): WidenShared<T> {
	return useCapabilityValue(runtime.application, capability, fallback)[0] as WidenShared<T>;
}

export function publishCapability<T extends SharedValue>(
	runtime: FeatureRuntime, value: T,
): void { runtime.application.publish(runtime.capability, value); }

function useCapabilitySnapshot(runtime: FeatureRuntime): CapabilitySnapshot {
	return useSyncExternalStore(
		(listener) => runtime.application.subscribeAll(listener),
		() => runtime.application.snapshot(),
		() => runtime.application.snapshot(),
	);
}

function useFeatureActions(
	runtime: FeatureRuntime,
): FeatureActions {
	return useMemo(() => {
			const current = (): FeatureState => runtime.application.read(runtime.capability, emptyFeatureState);
			const commit = (update: () => FeatureState): void => {
				runtime.application.publish(runtime.capability, update());
		};
		return {
			set(key, value) {
				commit(() => ({ ...current(), [key]: value }));
			},
		};
	}, [runtime]);
}

export function FeatureBoundary({ runtime, view }: FeatureBoundaryProps): ReactElement {
	const View = view;
	const [state] = useOwnCapabilityState(runtime, emptyFeatureState);
	const capabilities = useCapabilitySnapshot(runtime);
	const actions = useFeatureActions(runtime);
	return <View state={state} capabilities={capabilities} actions={actions} />;
}
`, genericBrowserCapabilityUnion(requirements), genericBrowserSharedValueBoundarySource(), strings.Join(allowed, ", "))
}

func genericBrowserRuntimeFactoryAPI() string {
	return strings.Join([]string{
		"function createApplicationRuntime(): ApplicationRuntime",
		"function createFeatureRuntime(runtime: ApplicationRuntime, capability: CapabilityID): FeatureRuntime",
	}, "\n")
}

func genericBrowserRuntimeFactorySource() string {
	return `export function createApplicationRuntime(): ApplicationRuntime { return new ApplicationRuntime(); }
export function createFeatureRuntime(runtime: ApplicationRuntime, capability: CapabilityID): FeatureRuntime {
  return new FeatureRuntime(runtime, capability);
}`
}
