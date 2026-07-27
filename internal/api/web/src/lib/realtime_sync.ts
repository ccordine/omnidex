export type RealtimeSyncDetail = {
  reason: string;
  latestID: number;
  waitUntil: (task: Promise<unknown>) => void;
};

/** Waits for every mounted server-state surface to finish its authoritative reconciliation read. */
export async function requestRealtimeSync(reason: string, latestID: number): Promise<void> {
  const tasks: Promise<unknown>[] = [];
  let acceptingTasks = true;
  const detail: RealtimeSyncDetail = {
    reason,
    latestID,
    waitUntil(task) {
      if (!acceptingTasks) {
        throw new Error("Realtime synchronization work must be registered during event dispatch.");
      }
      tasks.push(Promise.resolve(task));
    },
  };
  document.dispatchEvent(new CustomEvent<RealtimeSyncDetail>("omni:realtime-sync-required", { detail }));
  acceptingTasks = false;
  if (tasks.length === 0) {
    throw new Error("No server-authoritative realtime synchronization handler is mounted.");
  }
  await Promise.all(tasks);
}
