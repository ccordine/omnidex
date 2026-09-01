import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createProject,
  deleteProject,
  projectAutoWorkFailure,
  projectMutationFailure,
  projectQuery,
  startProjectAutoWork,
  updateProject,
} from "./project_api";

function response(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("project mutation authority", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("preserves exact accepted description bytes on create and patch", async () => {
    const requests: unknown[] = [];
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      requests.push(JSON.parse(String(init?.body)));
      return response({
        commit_state: "committed",
        project: {
          id: 7,
          name: "Project",
          location: "/srv/project",
          description: "",
          last_seen_at: "2026-08-13T12:00:00Z",
          created_at: "2026-08-13T12:00:00Z",
          updated_at: init?.method === "PATCH" ? "2026-08-13T12:00:01Z" : "2026-08-13T12:00:00Z",
        },
      }, init?.method === "POST" ? 201 : 200);
    });
    vi.stubGlobal("fetch", fetchMock);

    await createProject({ name: "Project", location: "/srv/project", description: "  create\n\t " });
    await updateProject(7, "2026-08-13T12:00:00Z", { description: "  patch\n\t " });

    expect(requests).toEqual([
      { name: "Project", location: "/srv/project", description: "  create\n\t " },
      { expected_updated_at: "2026-08-13T12:00:00Z", description: "  patch\n\t " },
    ]);
  });

  it("returns a typed post-commit degradation for explicit browser feedback", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      commit_state: "committed_degraded",
      project: {
        id: 7,
        name: "Project",
        location: "/srv/project",
        description: "",
        last_seen_at: "2026-08-13T12:00:00Z",
        created_at: "2026-08-13T12:00:00Z",
        updated_at: "2026-08-13T12:00:00Z",
      },
      operation_error: "Project was committed but survey failed.",
    }, 207)));
    const outcome = await createProject({ name: "Project", location: "/srv/project" });
    expect(projectMutationFailure(outcome)).toBe("Project was committed but survey failed.");
  });

  it("rejects malformed mutation outcomes and project identities", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({ project: { id: 7 } })));
    await expect(createProject({ name: "Project", location: "/srv/project" })).rejects.toThrow(/commit_state/);
    for (const id of [0, -1, 1.5, Number.NaN, Number.MAX_SAFE_INTEGER + 1]) {
      expect(() => projectQuery(id)).toThrow(/positive safe-integer project ID/);
    }
  });

  it("binds revision-bound project deletion to the exact response receipt", async () => {
    const requests: Array<{ url: string; body: unknown }> = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({ url: String(input), body: JSON.parse(String(init?.body)) });
      return response({
        commit_state: "committed", project_id: 7,
        expected_updated_at: "2026-08-13T12:00:00.123456Z", deleted: true,
      });
    }));
    await deleteProject(7, "2026-08-13T12:00:00.123456Z");
    expect(requests).toEqual([{
      url: "/v1/projects/7",
      body: { expected_updated_at: "2026-08-13T12:00:00.123456Z" },
    }]);
  });

  it.each([
    ["wrong project", { commit_state: "committed", project_id: 8, expected_updated_at: "2026-08-13T12:00:00Z", deleted: true }],
    ["wrong revision", { commit_state: "committed", project_id: 7, expected_updated_at: "2026-08-13T12:00:01Z", deleted: true }],
    ["unknown field", { commit_state: "committed", project_id: 7, expected_updated_at: "2026-08-13T12:00:00Z", deleted: true, fallback: true }],
  ])("rejects deletion receipt with %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    await expect(deleteProject(7, "2026-08-13T12:00:00Z")).rejects.toThrow();
  });

  it.each([" 2026-08-13T12:00:00Z", "2026-08-13T08:00:00-04:00", "2026-08-13T12:00:00.000Z", "2026-08-13T12:00:00.1234567Z", "2026-02-30T12:00:00Z"])(
    "rejects noncanonical project revision %s before transport",
    async (revision) => {
      const fetchMock = vi.fn();
      vi.stubGlobal("fetch", fetchMock);
      await expect(updateProject(7, revision, { description: "exact" })).rejects.toThrow(/canonical/);
      await expect(deleteProject(7, revision)).rejects.toThrow(/canonical/);
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each([
    ["unknown response field", {
      commit_state: "committed", project: {
        id: 7, name: "Project", location: "/srv/project", description: "",
        last_seen_at: "2026-08-13T12:00:00Z", created_at: "2026-08-13T12:00:00Z", updated_at: "2026-08-13T12:00:00Z",
      }, fallback: true,
    }],
    ["unknown project field", {
      commit_state: "committed", project: {
        id: 7, name: "Project", location: "/srv/project", description: "",
        last_seen_at: "2026-08-13T12:00:00Z", created_at: "2026-08-13T12:00:00Z", updated_at: "2026-08-13T12:00:00Z",
        agent: "forbidden",
      },
    }],
    ["wrong project", {
      commit_state: "committed", project: {
        id: 8, name: "Project", location: "/srv/project", description: "",
        last_seen_at: "2026-08-13T12:00:00Z", created_at: "2026-08-13T12:00:00Z", updated_at: "2026-08-13T12:00:00Z",
      },
    }],
  ])("rejects %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    await expect(updateProject(7, "2026-08-13T12:00:00Z", { description: "exact" })).rejects.toThrow();
  });

  it("retains authoritative auto-work state when post-commit projection fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      commit_state: "committed_degraded",
      project_id: 7,
      auto_work: { enabled: true, source_columns: ["assigned"] },
      active_card_id: "card-7",
      active_card_updated_at: "2026-08-13T12:00:00Z",
      job_id: 19,
      paused_cards: 0,
      message: "auto-work enabled and job queued",
      operation_error: "Board projection failed after commit.",
    }, 207)));

    const outcome = await startProjectAutoWork(7);
    expect(outcome.active_card_id).toBe("card-7");
    expect(outcome.job_id).toBe(19);
    expect(projectAutoWorkFailure(outcome)).toBe("Board projection failed after commit.");
  });

  it("rejects contradictory auto-work response authority", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      commit_state: "committed_degraded",
      project_id: 7,
      auto_work: { enabled: true, source_columns: ["assigned"] },
      active_card_id: "",
      active_card_updated_at: "2026-08-13T12:00:00Z",
      job_id: 19,
      paused_cards: 0,
      message: "",
      operation_error: "projection failed",
    }, 207)));
    await expect(startProjectAutoWork(7)).rejects.toThrow(/contradictory active-card authority/);
  });

  it.each([
    ["unknown field", {
      commit_state: "committed_degraded", project_id: 7,
      auto_work: { enabled: true, source_columns: ["assigned"] },
      active_card_id: "", active_card_updated_at: "", job_id: 0, paused_cards: 0,
      message: "auto-work enabled", operation_error: "projection failed", agent: "forbidden",
    }, 207],
    ["wrong project", {
      commit_state: "committed_degraded", project_id: 8,
      auto_work: { enabled: true, source_columns: ["assigned"] },
      active_card_id: "", active_card_updated_at: "", job_id: 0, paused_cards: 0,
      message: "auto-work enabled", operation_error: "projection failed",
    }, 207],
    ["HTTP state contradiction", {
      commit_state: "committed_degraded", project_id: 7,
      auto_work: { enabled: true, source_columns: ["assigned"] },
      active_card_id: "", active_card_updated_at: "", job_id: 0, paused_cards: 0,
      message: "auto-work enabled", operation_error: "projection failed",
    }, 200],
    ["implicit source column", {
      commit_state: "committed_degraded", project_id: 7,
      auto_work: { enabled: true, source_columns: [" assigned"] },
      active_card_id: "", active_card_updated_at: "", job_id: 0, paused_cards: 0,
      message: "auto-work enabled", operation_error: "projection failed",
    }, 207],
    ["missing degradation evidence", {
      commit_state: "committed_degraded", project_id: 7,
      auto_work: { enabled: true, source_columns: ["assigned"] },
      active_card_id: "", active_card_updated_at: "", job_id: 0, paused_cards: 0,
      message: "auto-work enabled",
    }, 207],
  ])("rejects %s in project auto-work authority", async (_name, payload, status) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload, status)));
    await expect(startProjectAutoWork(7)).rejects.toThrow();
  });
});
