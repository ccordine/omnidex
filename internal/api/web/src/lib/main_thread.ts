/** Yield so fetch handlers and other work can paint before heavy DOM updates. */
export function yieldToMain(): Promise<void> {
  const browserScheduler = (globalThis as { scheduler?: { yield?: () => Promise<void> } }).scheduler;
  if (typeof browserScheduler?.yield === "function") {
    return browserScheduler.yield();
  }
  return new Promise((resolve) => {
    requestAnimationFrame(() => resolve());
  });
}

type IdleDeadline = { didTimeout: boolean; timeRemaining: () => number };

/** Run work when the browser is idle (with a soon timeout fallback). */
export function runWhenIdle(task: () => void, timeoutMs = 80): void {
  if (typeof requestIdleCallback === "function") {
    requestIdleCallback(() => task(), { timeout: timeoutMs });
    return;
  }
  window.setTimeout(task, 0);
}

type DomFrameCallback = {
  task: () => void;
  resolve: () => void;
  reject: (error: unknown) => void;
};

const domFrameCallbacks = new Set<DomFrameCallback>();
let domFrameScheduled = false;

/** Coalesce DOM writes into the next animation frame (one flush per frame). */
export function scheduleDomUpdate(task: () => void): Promise<void> {
  return new Promise((resolve, reject) => {
    domFrameCallbacks.add({ task, resolve, reject });
    if (domFrameScheduled) return;
    domFrameScheduled = true;
    requestAnimationFrame(() => {
      domFrameScheduled = false;
      const batch = [...domFrameCallbacks];
      domFrameCallbacks.clear();
      for (const callback of batch) {
        try {
          callback.task();
          callback.resolve();
        } catch (error) {
          console.error("Scheduled DOM update failed", error);
          callback.reject(error);
        }
      }
    });
  });
}

export function debounce<T extends (...args: never[]) => void>(fn: T, waitMs: number): (...args: Parameters<T>) => void {
  let timer: number | null = null;
  return (...args: Parameters<T>) => {
    if (timer != null) window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      timer = null;
      fn(...args);
    }, waitMs);
  };
}
