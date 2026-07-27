import { beforeEach, describe, expect, it, vi } from "vitest";
import { ChatExecutionCoordinator, type ChatExecutionHost } from "./chat_execution_coordinator";

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function createHost() {
  const jobBadge = document.createElement("div");
  let activityLabel = "";
  const host: ChatExecutionHost = {
    openedProjectID: () => 42,
    openedProjectLocation: () => "/workspace/project",
    currentPanel: () => "chat",
    hasJobBadge: () => true,
    jobBadge: () => jobBadge,
    setActivityLabel: (label) => { activityLabel = label; },
    setStatus: vi.fn(),
    renderProgressActivity: vi.fn(),
    recordJobProgress: vi.fn(),
    renderMessages: vi.fn(),
    renderJobDetails: vi.fn(),
    addEvent: vi.fn(),
    addMessage: vi.fn(),
    setBusy: vi.fn(),
    loadJobs: vi.fn(async () => undefined),
    loadGlobalActivity: vi.fn(async () => undefined),
    reportError: vi.fn(),
  };
  return { host, jobBadge, activityLabel: () => activityLabel };
}

describe("ChatExecutionCoordinator", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("completes a queued job from server-confirmed state", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ job: { id: 17, status: "pending" } }))
      .mockResolvedValueOnce(response({
        job: { id: 17, status: "completed", result: "Server result" },
        steps: [],
        contexts: [],
      }));
    vi.stubGlobal("fetch", fetchMock);
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await coordinator.submit("Do the work");

    expect(coordinator.currentJobID()).toBe(17);
    expect(fixture.jobBadge.textContent).toBe("#17");
    expect(fixture.host.addMessage).toHaveBeenCalledWith("assistant", "Server result");
    expect(fixture.host.setStatus).toHaveBeenCalledWith("completed", "ready");
    expect(fixture.host.setBusy).toHaveBeenCalledWith(false);
    expect(fixture.host.addEvent).toHaveBeenCalledWith(
      "job_created",
      { id: 17, status: "pending" },
      expect.objectContaining({ job: { id: 17, status: "pending" } }),
    );
  });

  it("rejects a queued response without a valid job id", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({ job: { id: 0, status: "pending" } })));
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await expect(coordinator.submit("Do the work")).rejects.toThrow("valid positive integer id");
    expect(coordinator.currentJobID()).toBeNull();
  });

  it("rejects unknown server job states", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({ job: { id: 17, status: "maybe" } })));
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);

    await expect(coordinator.submit("Do the work")).rejects.toThrow('invalid status "maybe"');
    expect(coordinator.currentJobID()).toBeNull();
  });

  it("rejects a second submission before creating another server job", async () => {
    const neverCompletes = new Promise<Response>(() => undefined);
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ job: { id: 17, status: "pending" } }))
      .mockReturnValueOnce(neverCompletes);
    vi.stubGlobal("fetch", fetchMock);
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);
    const firstResult = coordinator.submit("First job").catch((error) => error as Error);
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

    await expect(coordinator.submit("Second job")).rejects.toThrow("Job #17 is already active");
    expect(fetchMock).toHaveBeenCalledTimes(2);

    coordinator.disconnect();
    const error = await firstResult;
    expect(error).toBeInstanceOf(Error);
    if (!(error instanceof Error)) throw new Error("Expected the active job promise to reject.");
    expect(error.message).toContain("disconnected before the active job completed");
  });

  it("fails loudly on malformed realtime events", () => {
    const fixture = createHost();
    const coordinator = new ChatExecutionCoordinator(fixture.host);
    const event = new CustomEvent("omni:job-progress", { detail: { phase: "running" } });

    expect(() => coordinator.handleProgress(event)).toThrow("valid positive integer id");
    expect(fixture.host.loadJobs).not.toHaveBeenCalled();
  });
});
