import { afterEach, describe, expect, it, vi } from "vitest";
import { requestRealtimeSync } from "./realtime_sync";
import { observeRealtimeJob, type RealtimeJobSnapshot } from "./realtime_job_observer";

type JobData = { result?: string };

describe("observeRealtimeJob", () => {
  const observations: Array<{ cancel: (reason?: string) => void }> = [];

  afterEach(() => {
    observations.forEach((observation) => observation.cancel("Test cleanup"));
    observations.length = 0;
  });

  it("reconciles from the server immediately and on matching live job events", async () => {
    const snapshots: RealtimeJobSnapshot<JobData>[] = [
      { status: "running", data: {} },
      { status: "completed", data: { result: "done" } },
    ];
    const load = vi.fn(async () => {
      const snapshot = snapshots.shift();
      if (!snapshot) throw new Error("Unexpected extra job read.");
      return snapshot;
    });
    const onUpdate = vi.fn();
    const observation = observeRealtimeJob({ jobID: 17, load, onUpdate });
    observations.push(observation);

    await observation.initial;
    document.dispatchEvent(new CustomEvent("omni:job-progress", { detail: { jobID: 17, phase: "finished" } }));

    await expect(observation.completion).resolves.toEqual({ status: "completed", data: { result: "done" } });
    expect(load).toHaveBeenCalledTimes(2);
    expect(onUpdate).toHaveBeenCalledTimes(2);

    document.dispatchEvent(new CustomEvent("omni:job-progress", { detail: { jobID: 99, phase: "finished" } }));
    await Promise.resolve();
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("registers its server read with full realtime reconciliation", async () => {
    const load = vi
      .fn<() => Promise<RealtimeJobSnapshot<JobData>>>()
      .mockResolvedValueOnce({ status: "running", data: {} })
      .mockResolvedValueOnce({ status: "completed", data: { result: "synced" } });
    const observation = observeRealtimeJob({ jobID: 21, load });
    observations.push(observation);
    await observation.initial;

    await requestRealtimeSync("replay_gap", 44);

    await expect(observation.completion).resolves.toEqual({ status: "completed", data: { result: "synced" } });
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("rejects failed and malformed authoritative job state", async () => {
    const failed = observeRealtimeJob<JobData>({
      jobID: 7,
      load: async () => ({ status: "failed", error: "Query rejected", data: {} }),
    });
    observations.push(failed);
    await expect(failed.completion).rejects.toThrow("Query rejected");

    const malformed = observeRealtimeJob<JobData>({
      jobID: 8,
      load: async () => ({ status: "", data: {} }),
    });
    observations.push(malformed);
    await expect(malformed.completion).rejects.toThrow("returned no status");
  });

  it("does not apply a late server response after cancellation", async () => {
    let resolveLoad!: (snapshot: RealtimeJobSnapshot<JobData>) => void;
    const load = vi.fn(() => new Promise<RealtimeJobSnapshot<JobData>>((resolve) => {
      resolveLoad = resolve;
    }));
    const onUpdate = vi.fn();
    const observation = observeRealtimeJob({ jobID: 31, load, onUpdate });
    observations.push(observation);
    const completion = observation.completion.catch((error) => error);

    observation.cancel("Modal closed.");
    resolveLoad({ status: "completed", data: { result: "late" } });
    await observation.initial;

    expect(onUpdate).not.toHaveBeenCalled();
    await expect(completion).resolves.toMatchObject({ message: "Modal closed." });
  });
});
