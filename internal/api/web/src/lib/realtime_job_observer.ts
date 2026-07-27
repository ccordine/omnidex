import type { RealtimeSyncDetail } from "./realtime_sync";

export type RealtimeJobSnapshot<T> = {
  status: string;
  data: T;
  error?: string;
};

export type RealtimeJobObservation<T> = {
  completion: Promise<RealtimeJobSnapshot<T>>;
  initial: Promise<void>;
  refresh: () => Promise<void>;
  cancel: (reason?: string) => void;
};

type ObserveRealtimeJobOptions<T> = {
  jobID: number;
  load: () => Promise<RealtimeJobSnapshot<T>>;
  onUpdate?: (snapshot: RealtimeJobSnapshot<T>) => void | Promise<void>;
  timeoutMs?: number;
};

const DEFAULT_TIMEOUT_MS = 10 * 60 * 1000;

export function observeRealtimeJob<T>(options: ObserveRealtimeJobOptions<T>): RealtimeJobObservation<T> {
  const { jobID, load, onUpdate } = options;
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  if (!Number.isSafeInteger(jobID) || jobID <= 0) throw new Error("Realtime job observer requires a positive integer job id.");
  if (typeof load !== "function") throw new Error(`Realtime job #${jobID} observer requires a server loader.`);
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) throw new Error(`Realtime job #${jobID} observer requires a positive timeout.`);

  let settled = false;
  let refreshRequested = false;
  let refreshInFlight: Promise<void> | null = null;
  let resolveCompletion!: (snapshot: RealtimeJobSnapshot<T>) => void;
  let rejectCompletion!: (error: unknown) => void;
  const completion = new Promise<RealtimeJobSnapshot<T>>((resolve, reject) => {
    resolveCompletion = resolve;
    rejectCompletion = reject;
  });

  const cleanup = () => {
    document.removeEventListener("omni:job-progress", progressHandler);
    document.removeEventListener("omni:realtime-sync-required", syncHandler);
    window.clearTimeout(timeout);
  };
  const fail = (error: unknown) => {
    if (settled) return;
    settled = true;
    cleanup();
    rejectCompletion(error);
  };
  const complete = (snapshot: RealtimeJobSnapshot<T>) => {
    if (settled) return;
    settled = true;
    cleanup();
    resolveCompletion(snapshot);
  };

  const runRefresh = async () => {
    do {
      refreshRequested = false;
      const snapshot = await load();
      if (settled) return;
      const status = snapshot.status.trim().toLowerCase();
      if (!status) throw new Error(`Authoritative job #${jobID} returned no status.`);
      const normalized = { ...snapshot, status };
      await onUpdate?.(normalized);
      if (settled) return;
      if (status === "completed") {
        complete(normalized);
        return;
      }
      if (status === "failed" || status === "canceled") {
        const message = snapshot.error?.trim() || `Job #${jobID} ${status}.`;
        throw new Error(message);
      }
    } while (refreshRequested && !settled);
  };
  const refresh = (): Promise<void> => {
    if (settled) return Promise.resolve();
    if (refreshInFlight) {
      refreshRequested = true;
      return refreshInFlight;
    }
    let task: Promise<void>;
    task = runRefresh().finally(() => {
      if (refreshInFlight === task) refreshInFlight = null;
    });
    refreshInFlight = task;
    void task.catch((error) => fail(error));
    return task;
  };
  const progressHandler: EventListener = (event) => {
    const detail = (event as CustomEvent<{ jobID?: number }>).detail;
    if (Number(detail?.jobID ?? 0) !== jobID) return;
    void refresh().catch((error) => fail(error));
  };
  const syncHandler: EventListener = (event) => {
    const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
    if (!detail || typeof detail.waitUntil !== "function") {
      throw new Error("Realtime synchronization event is missing waitUntil().");
    }
    detail.waitUntil(refresh());
  };
  const cancel = (reason = `Realtime job #${jobID} observation canceled.`) => {
    if (settled) return;
    const message = reason.trim();
    fail(new Error(message || `Realtime job #${jobID} observation canceled.`));
  };

  document.addEventListener("omni:job-progress", progressHandler);
  document.addEventListener("omni:realtime-sync-required", syncHandler);
  const timeout = window.setTimeout(() => {
    fail(new Error(`Timed out waiting for authoritative completion of job #${jobID}.`));
  }, timeoutMs);
  const initial = refresh();
  void initial.catch((error) => fail(error));

  return { completion, initial, refresh, cancel };
}
