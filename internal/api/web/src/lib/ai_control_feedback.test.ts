import { describe, expect, it } from "vitest";
import { aiControlDegradedMessage, validateAIControlMutationPayload } from "./ai_control_feedback";

describe("AI control commit feedback", () => {
  it("distinguishes a complete commit from a committed degraded result", () => {
    expect(aiControlDegradedMessage({ commit_state: "committed" })).toBe("");
    expect(aiControlDegradedMessage({
      commit_state: "committed_degraded",
      operation_error: "Scrum reconciliation failed",
    })).toBe("Scrum reconciliation failed");
    expect(() => aiControlDegradedMessage({ commit_state: "committed_degraded" })).toThrow(/requires one explicit/);
  });

  it("rejects an unregistered commit state", () => {
    expect(() => aiControlDegradedMessage({ commit_state: "success" as never })).toThrow(/exact registered state/);
  });

  it("requires a commit state and rejects contradictory mutation feedback", () => {
    expect(() => aiControlDegradedMessage({} as never)).toThrow(/commit_state is required/);
    expect(() => aiControlDegradedMessage({
      commit_state: "committed",
      operation_error: "hidden failure",
    })).toThrow(/must not contain/);
    expect(() => aiControlDegradedMessage({
      commit_state: "committed_degraded",
      operation_error: "   ",
    })).toThrow(/bounded nonblank string/);
    expect(() => aiControlDegradedMessage({
      commit_state: "committed_degraded",
      operation_error: { hidden: true } as never,
    })).toThrow(/bounded nonblank string/);
  });

  it.each([
    ["object error", { operation_error: { hidden: true } }],
    ["noncanonical timestamp", { updated_at: "2026-08-13T12:00:00.000Z" }],
    ["contradictory publication", { realtime_published: true, realtime_error: "publish failed" }],
  ])("rejects a mutation response with %s", (_name, override) => {
    expect(() => validateAIControlMutationPayload({
      commit_state: "committed_degraded",
      paused: true,
      canceled_jobs: 1,
      resumed: false,
      updated_at: "2026-08-13T12:00:00.123456Z",
      realtime_published: false,
      operation_error: "post-commit reconciliation failed",
      ...override,
    })).toThrow();
  });
});
