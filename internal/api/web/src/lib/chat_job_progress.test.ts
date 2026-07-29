import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  describeChatJobProgress,
  describeJobStatus,
  describeRealtimeJobPhase,
  recordChatJobProgress,
  type ChatJobProgressHost,
} from "./chat_job_progress";
import { initI18n } from "./i18n";

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
  beforeEach(() => {
    document.documentElement.lang = "en";
    document.documentElement.dir = "ltr";
    initI18n();
  });

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

  it("records accepted coding diffs as reviewable diff events", () => {
    const fixture = createHost();

    recordChatJobProgress(fixture.host, {
      job: { id: 9, status: "running" },
      steps: [{ id: 3, action: "v3_coding", status: "running" }],
      contexts: [{
        id: 41,
        step_id: 3,
        key: "coding_diff",
        value: "path=main.go\n--- a/main.go\n+++ b/main.go\n@@\n-old\n+new",
      }],
    });

    expect(fixture.addEvent).toHaveBeenCalledWith(
      "coding_diff",
      expect.objectContaining({ context_id: 41, key: "coding_diff" }),
      expect.objectContaining({ context: expect.objectContaining({ id: 41 }) }),
    );
  });

  it.each([
    ["es", "Escribiendo código… (#9)"],
    ["zh-Hans", "正在编写代码… (#9)"],
    ["ru", "Написание кода… (#9)"],
    ["ja", "コードを作成中… (#9)"],
  ] as const)("localizes live coding progress in %s", (locale, expected) => {
    document.documentElement.lang = locale;
    initI18n();

    expect(describeChatJobProgress({
      job: { id: 9, status: "running" },
      steps: [{ id: 3, action: "v3_coding", status: "running" }],
    })).toBe(expected);
  });

  it("rejects unknown server states instead of displaying guessed labels", () => {
    expect(() => describeJobStatus("maybe")).toThrow('Unsupported job status "maybe"');
    expect(() => describeRealtimeJobPhase("maybe")).toThrow('Unsupported realtime job phase "maybe"');
  });
});
