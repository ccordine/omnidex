import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectMutationCoordinator } from "./project_mutation_coordinator";

describe("project Scrum automation authority", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("reconciles committed settings before reporting post-commit scheduling failure", async () => {
    const detail = document.createElement("div");
    detail.innerHTML = `
      <input type="checkbox" data-projects-field="autoWorkEnabled" checked>
      <input type="checkbox" data-projects-field="autoWorkColumn" data-auto-work-column="assigned" checked>
      <button data-action="projects#saveScrumAutomation" data-project-id="7">Save automation</button>
    `;
    document.body.append(detail);
    const calls: string[] = [];
    const failure = vi.fn();
    document.addEventListener("omni:scrum-refresh", () => calls.push("board-refresh"), { once: true });
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      commit_state: "committed_degraded",
      auto_work: { enabled: true, source_columns: ["assigned"] },
      operation_error: "Settings committed, but scheduling failed.",
    }), { status: 207 })));
    const coordinator = new ProjectMutationCoordinator({
      detailRoot: () => detail,
      reloadDetail: async () => { calls.push("detail-reload"); },
      reloadList: async () => { calls.push("list-reload"); },
      projectDeleted: vi.fn(),
      setStatus: vi.fn(),
      success: vi.fn(),
      failure: (error) => { calls.push("failure"); failure(error); },
    });

    const button = detail.querySelector("button");
    if (!(button instanceof HTMLButtonElement)) throw new Error("Missing test mutation button.");
    await coordinator.saveScrumAutomation({ preventDefault() {}, currentTarget: button } as unknown as Event);

    expect(calls).toEqual(["list-reload", "detail-reload", "board-refresh", "failure"]);
    expect(failure).toHaveBeenCalledWith(expect.objectContaining({
      message: "Settings committed, but scheduling failed.",
    }));
  });
});
