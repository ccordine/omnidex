import { afterEach, describe, expect, it, vi } from "vitest";
import { requestRealtimeSync, type RealtimeSyncDetail } from "./realtime_sync";

describe("requestRealtimeSync", () => {
  const listeners: EventListener[] = [];

  afterEach(() => {
    listeners.forEach((listener) => document.removeEventListener("omni:realtime-sync-required", listener));
    listeners.length = 0;
  });

  it("does not report synchronization complete until registered server reads finish", async () => {
    let release!: () => void;
    const reconciliation = new Promise<void>((resolve) => {
      release = resolve;
    });
    const listener: EventListener = (event) => {
      const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
      detail.waitUntil(reconciliation);
    };
    listeners.push(listener);
    document.addEventListener("omni:realtime-sync-required", listener);
    const completed = vi.fn();

    const sync = requestRealtimeSync("replay_gap", 41).then(completed);
    await Promise.resolve();
    expect(completed).not.toHaveBeenCalled();
    release();
    await sync;

    expect(completed).toHaveBeenCalledOnce();
  });

  it("fails loudly when no server-authoritative reconciliation handler is mounted", async () => {
    await expect(requestRealtimeSync("message_gap", 42)).rejects.toThrow(
      "No server-authoritative realtime synchronization handler is mounted",
    );
  });
});
