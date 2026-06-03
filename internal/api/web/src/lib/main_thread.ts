/** Yield so fetch handlers and other work can paint before heavy DOM updates. */
export function yieldToMain(): Promise<void> {
  if (typeof scheduler !== "undefined" && "yield" in scheduler) {
    return (scheduler as Scheduler & { yield: () => Promise<void> }).yield();
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

const domFrameCallbacks = new Set<() => void>();
let domFrameScheduled = false;

/** Coalesce DOM writes into the next animation frame (one flush per frame). */
export function scheduleDomUpdate(task: () => void): void {
  domFrameCallbacks.add(task);
  if (domFrameScheduled) return;
  domFrameScheduled = true;
  requestAnimationFrame(() => {
    domFrameScheduled = false;
    const batch = [...domFrameCallbacks];
    domFrameCallbacks.clear();
    for (const fn of batch) {
      try {
        fn();
      } catch {
        /* individual sink updates must not break the batch */
      }
    }
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
