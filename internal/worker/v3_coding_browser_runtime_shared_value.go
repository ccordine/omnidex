package worker

func genericBrowserSharedValueBoundarySource() string {
	return `const maximumSharedValueDepth = 64;
const maximumSharedValueNodes = 10000;

function rejectSharedValue(boundary: string, path: string, reason: string): never {
	throw new Error('Shared value ' + boundary + ' rejects ' + reason + ' at ' + path + '.');
}

function sharedValuePropertyPath(path: string, property: string): string {
	return path + '[' + JSON.stringify(property) + ']';
}

function validateAndFreezeSharedValue<T extends SharedValue>(value: T, boundary: string): T {
	const active = new WeakSet<object>();
	const validated = new WeakSet<object>();
	const containers: object[] = [];
	let nodes = 0;

	function validateProperty(container: object, property: string, path: string, depth: number): void {
		const descriptor = Object.getOwnPropertyDescriptor(container, property);
		if (descriptor === undefined || !Object.hasOwn(descriptor, 'value') || descriptor.enumerable !== true) {
			rejectSharedValue(boundary, path, 'an accessor or hidden property');
		}
		validate(descriptor.value as unknown, path, depth);
	}

	function validate(candidate: unknown, path: string, depth: number): void {
		nodes += 1;
		if (nodes > maximumSharedValueNodes) rejectSharedValue(boundary, path, 'a graph larger than the supported node limit');
		if (depth > maximumSharedValueDepth) rejectSharedValue(boundary, path, 'nesting deeper than the supported depth limit');
		if (candidate === null || typeof candidate === 'boolean' ||
			typeof candidate === 'number' || typeof candidate === 'string') {
			return;
		}
		if (typeof candidate !== 'object') {
			rejectSharedValue(boundary, path, 'an unsupported ' + typeof candidate + ' value');
		}
		const container = candidate as object;
		if (active.has(container)) rejectSharedValue(boundary, path, 'a cyclic reference');
		if (validated.has(container)) return;

		active.add(container);
		try {
			if (Object.getOwnPropertySymbols(container).length !== 0) {
				rejectSharedValue(boundary, path, 'a symbol-keyed property');
			}
			if (Array.isArray(container)) {
				if (Object.getPrototypeOf(container) !== Array.prototype) {
					rejectSharedValue(boundary, path, 'an array with a custom prototype');
				}
				const properties = Object.getOwnPropertyNames(container);
				if (properties.length !== container.length + 1) {
					rejectSharedValue(boundary, path, 'a sparse array or an array with extra properties');
				}
				for (let index = 0; index < container.length; index += 1) {
					validateProperty(container, String(index), path + '[' + index + ']', depth + 1);
				}
			} else {
				const prototype = Object.getPrototypeOf(container);
				if (prototype !== Object.prototype && prototype !== null) {
					rejectSharedValue(boundary, path, 'an object with a custom prototype');
				}
				for (const property of Object.getOwnPropertyNames(container)) {
					validateProperty(container, property, sharedValuePropertyPath(path, property), depth + 1);
				}
			}
		} finally {
			active.delete(container);
		}
		validated.add(container);
		containers.push(container);
	}

	validate(value, '$', 0);
	for (const container of containers) Object.freeze(container);
	return value;
}`
}
