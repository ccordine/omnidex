export type LifecycleOperationID = `lifecycle_operation_${string}`;

export function newLifecycleOperationID(): LifecycleOperationID {
  if (!globalThis.crypto?.getRandomValues) {
    throw new Error("Lifecycle operation identity requires browser cryptographic randomness.");
  }
  const bytes = new Uint8Array(32);
  globalThis.crypto.getRandomValues(bytes);
  return `lifecycle_operation_${Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("")}`;
}

type PendingLifecycleOperation = {
  key: string;
  operationID: LifecycleOperationID;
};

const maxPendingLifecycleOperations = 64;

export type LifecycleOperationAttemptKey = Readonly<{
  scope: string;
  action: string;
  content: string;
}>;

export class LifecycleOperationAttempt {
  private readonly pending = new Map<string, PendingLifecycleOperation>();

  acquire(input: LifecycleOperationAttemptKey): LifecycleOperationID {
    const { slot, key } = lifecycleOperationAttemptKeys(input);
    const pending = this.pending.get(slot);
    if (pending?.key === key) return pending.operationID;
    if (!pending && this.pending.size >= maxPendingLifecycleOperations) {
      throw new Error(`Lifecycle operation attempts exceed the ${maxPendingLifecycleOperations} pending-operation limit.`);
    }
    const operationID = newLifecycleOperationID();
    this.pending.set(slot, { key, operationID });
    return operationID;
  }

  confirm(input: LifecycleOperationAttemptKey, operationID: LifecycleOperationID): boolean {
    const { slot, key } = lifecycleOperationAttemptKeys(input);
    const pending = this.pending.get(slot);
    if (pending?.key !== key || pending.operationID !== operationID) return false;
    this.pending.delete(slot);
    return true;
  }
}

function lifecycleOperationAttemptKeys(input: LifecycleOperationAttemptKey): { slot: string; key: string } {
  const exactScope = input.scope.trim();
  const exactAction = input.action.trim();
  const exactContent = input.content.trim();
  if (!exactScope) throw new Error("Lifecycle operation attempt requires a scope.");
  if (!exactAction) throw new Error("Lifecycle operation attempt requires an action.");
  if (!exactContent) throw new Error("Lifecycle operation attempt requires content.");
  return {
    slot: JSON.stringify([exactScope, exactAction]),
    key: JSON.stringify([exactScope, exactAction, exactContent]),
  };
}
