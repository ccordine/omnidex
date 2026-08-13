import { beforeEach, describe, expect, it, vi } from "vitest";
import { ChatExecutionCoordinator, type ChatExecutionHost } from "./chat_execution_coordinator";

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function createHost() {
  const jobBadge = document.createElement("div");
  const host: ChatExecutionHost = {
    currentPanel: () => "chat",
    hasJobBadge: () => true,
    jobBadge: () => jobBadge,
    setActivityLabel: vi.fn(),
    setStatus: vi.fn(),
    renderProgressActivity: vi.fn(),
    renderJobState: vi.fn(async () => undefined),
    addEvent: vi.fn(),
    loadJobs: vi.fn(async () => undefined),
    loadGlobalActivity: vi.fn(async () => undefined),
    reportError: vi.fn(),
  };
  return { host, jobBadge };
}

describe("ChatExecutionCoordinator", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("waits for an existing channel job without creating transcript state", async () => {
    const fetchMock = vi.fn(async () => response({
      job: { id: 73, status: "completed", current_generation: 1, result: "Result persisted to the channel" },
      steps: [],
      progress: { latest_context_id: 0, count: 0 },
	  html: { bundle: "server-job-state" },
    }));
    vi.stubGlobal("fetch", fetchMock);
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await coordinator.waitForExistingJob(73);

    expect(fetchMock).toHaveBeenCalledWith("/v1/ui/chat/jobs/73");
    expect(coordinator.currentJobID()).toBe(73);
    expect(fixture.jobBadge.textContent).toBe("#73");
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith("completed", "ready");
  });

  it("allows a completed channel job without copying its result into browser state", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      job: { id: 75, status: "completed", current_generation: 1 },
      steps: [],
      progress: { latest_context_id: 0, count: 0 },
	  html: { bundle: "server-job-state" },
    })));
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await expect(coordinator.waitForExistingJob(75)).resolves.toBeUndefined();
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith("completed", "ready");
  });

  it("rejects a failed channel job without inventing an error message", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      job: { id: 74, status: "failed", current_generation: 1, error: "Server failure" },
      steps: [],
      progress: { latest_context_id: 0, count: 0 },
	  html: { bundle: "server-job-state" },
    })));
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await expect(coordinator.waitForExistingJob(74)).rejects.toThrow("Server failure");
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith("failed", "error");
  });

  it("rejects overlapping channel jobs", async () => {
    const neverCompletes = new Promise<Response>(() => undefined);
    vi.stubGlobal("fetch", vi.fn(() => neverCompletes));
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);
    const first = coordinator.waitForExistingJob(81).catch((error) => error as Error);
    await vi.waitFor(() => expect(coordinator.currentJobID()).toBe(81));

    await expect(coordinator.waitForExistingJob(82)).rejects.toThrow("Job #81 is already active");
    coordinator.disconnect();
    await expect(first).resolves.toBeInstanceOf(Error);
  });

  it("fails loudly on malformed realtime events", () => {
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);
    const event = new CustomEvent("omni:job-progress", { detail: { phase: "running" } });

    expect(() => coordinator.handleProgress(event)).toThrow("valid positive integer id");
    expect(fixture.host.loadJobs).not.toHaveBeenCalled();
  });

  it("rejects raw or inconsistent progress authority before applying server markup", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      job: { id: 76, status: "running", current_generation: 2 },
      steps: [{ id: 9, action: "v3_coding", status: "running", generation: 1 }],
      contexts: [{ id: 99, key: "llm_prompt", value: "private envelope" }],
      progress: { latest_context_id: 99, count: 25 },
      html: { bundle: "must-not-render" },
    })));
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await expect(coordinator.waitForExistingJob(76)).rejects.toThrow(/current generation|bounded progress/i);
    expect(fixture.host.renderJobState).not.toHaveBeenCalled();
  });
});
