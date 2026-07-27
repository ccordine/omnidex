import { describe, expect, it, vi } from "vitest";
import { recordChatJobProgress, type ChatJobProgressHost } from "./chat_job_progress";

function createHost() {
  const seenProgress = new Set<string>();
  const addEvent = vi.fn();
  const addObservedEvent = vi.fn((key: string, type: string, details: Record<string, unknown>, full: unknown) => {
    if (seenProgress.has(key)) return;
    seenProgress.add(key);
    addEvent(type, details, full);
  });
  const host: ChatJobProgressHost = {
    seenProgress,
    renderProgress: vi.fn(),
    indexContexts: vi.fn(),
    addEvent,
    addObservedEvent,
  };
  return { host, addEvent };
}

describe("recordChatJobProgress", () => {
  it("keeps live output visible without flooding the timeline with every chunk", () => {
    const fixture = createHost();
    const details = (output: string, status = "running") => ({
      job: { id: 9, status: "running" },
      steps: [{ id: 3, action: "v3_analysis", status, output }],
      contexts: [],
    });

    recordChatJobProgress(fixture.host, details("first chunk"));
    recordChatJobProgress(fixture.host, details("first chunk plus another"));

    expect(fixture.addEvent.mock.calls.filter(([type]) => type === "job_update")).toHaveLength(1);
    expect(fixture.addEvent.mock.calls.filter(([type]) => type === "step_output")).toHaveLength(0);

    recordChatJobProgress(fixture.host, details("final output", "completed"));
    expect(fixture.addEvent.mock.calls.filter(([type]) => type === "job_update")).toHaveLength(2);
    expect(fixture.addEvent.mock.calls.filter(([type]) => type === "step_output")).toHaveLength(1);
  });
});
