import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectMutationCoordinator, type ProjectMutationHost } from "./project_mutation_coordinator";
import { HTTPResponseError } from "./api";

describe("project mutation interaction lock", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it.each([
    ["save", "saveProject", "PATCH", "Save project"],
    ["survey", "rescanProject", "POST", "Detect stack"],
    ["auto-work", "saveScrumAutomation", "PATCH", "Save automation"],
    ["delete", "deleteProject", "DELETE", "Delete"],
  ] as const)("suppresses a second %s request while the first response is pending", async (_name, method, verb, label) => {
    let release!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => { release = resolve; });
    let mutationRequests = 0;
    vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      mutationRequests += 1;
      expect(init?.method).toBe(verb);
      return pending;
    }));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const detail = document.createElement("div");
    detail.innerHTML = `
      <input data-projects-field="name" value="Project">
      <input data-projects-field="location" value="/srv/project">
      <textarea data-projects-field="description">Exact</textarea>
      <input type="checkbox" data-projects-field="autoWorkEnabled" checked>
      <input type="checkbox" data-projects-field="autoWorkColumn" data-auto-work-column="assigned" checked>
      <button data-action="projects#${method}" data-project-id="7" data-project-updated-at="2026-08-13T12:00:00Z">${label}</button>
    `;
    document.body.append(detail);
    const failure = vi.fn();
    const coordinator = new ProjectMutationCoordinator(host(detail, failure));
    const button = detail.querySelector("button");
    if (!(button instanceof HTMLButtonElement)) throw new Error("Missing project mutation test button.");
    const event = { preventDefault() {}, stopPropagation() {}, currentTarget: button } as unknown as Event;

    const first = coordinator[method](event);
    await vi.waitFor(() => expect(button).toBeDisabled());
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button.textContent).toContain("…");
    const second = coordinator[method](event);
    await second;
    expect(mutationRequests).toBe(1);
    expect(failure).toHaveBeenCalledWith(expect.objectContaining({ message: "A project mutation is already in progress." }));

    release(jsonResponse(method));
    await first;
    expect(button).not.toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "false");
    expect(button.textContent).toBe(label);
    expect(mutationRequests).toBe(1);
  });

  it.each([undefined, "07", "7 ", "9007199254740992"])(
    "rejects inexact server project identity %s before transport",
    async (projectID) => {
      const fetchMock = vi.fn();
      vi.stubGlobal("fetch", fetchMock);
      const detail = document.createElement("div");
      detail.innerHTML = `
        <input data-projects-field="name" value="Project">
        <input data-projects-field="location" value="/srv/project">
        <textarea data-projects-field="description">Exact</textarea>
        <button data-action="projects#saveProject" data-project-updated-at="2026-08-13T12:00:00Z">Save project</button>
      `;
      document.body.append(detail);
      const button = detail.querySelector("button");
      if (!(button instanceof HTMLButtonElement)) throw new Error("Missing project mutation test button.");
      if (projectID !== undefined) button.dataset.projectId = projectID;
      const coordinator = new ProjectMutationCoordinator(host(detail, vi.fn()));

      await expect(coordinator.saveProject({
        preventDefault() {}, currentTarget: button,
      } as unknown as Event)).rejects.toThrow(/project id|safe integer/);
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each([undefined, " 2026-08-13T12:00:00Z", "2026-08-13T12:00:00.000Z", "2026-08-13T12:00:00.0000000Z", "2026-08-13T08:00:00-04:00", "2026-02-30T12:00:00Z"])(
    "rejects inexact server project revision %s before transport",
    async (revision) => {
      const fetchMock = vi.fn();
      vi.stubGlobal("fetch", fetchMock);
      const detail = document.createElement("div");
      detail.innerHTML = `
        <input data-projects-field="name" value="Project">
        <input data-projects-field="location" value="/srv/project">
        <textarea data-projects-field="description">Exact</textarea>
        <button data-action="projects#saveProject" data-project-id="7">Save project</button>
      `;
      const button = detail.querySelector("button");
      if (!(button instanceof HTMLButtonElement)) throw new Error("Missing project mutation test button.");
      if (revision !== undefined) button.dataset.projectUpdatedAt = revision;
      const coordinator = new ProjectMutationCoordinator(host(detail, vi.fn()));
      await expect(coordinator.saveProject({ preventDefault() {}, currentTarget: button } as unknown as Event)).rejects.toThrow(/revision/);
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it("reloads authoritative project list and detail after a failed revision mutation", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("response lost"); }));
    const detail = projectDetail("saveProject");
    const reloadList = vi.fn(async () => undefined);
    const reloadDetail = vi.fn(async () => undefined);
    const failure = vi.fn();
    const coordinator = new ProjectMutationCoordinator({
      ...host(detail, failure), reloadList, reloadDetail,
    });

    await coordinator.saveProject(projectEvent(detail));

    expect(reloadList).toHaveBeenCalledTimes(1);
    expect(reloadDetail).toHaveBeenCalledTimes(1);
    expect(failure).toHaveBeenCalledWith(expect.objectContaining({ message: "response lost" }));
  });

  it("uses an authoritative 404 to reconcile a deletion whose response was lost", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("response lost"); }));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const detail = projectDetail("deleteProject");
    const projectDeleted = vi.fn();
    const reloadList = vi.fn(async () => undefined);
    const reloadDetail = vi.fn(async () => { throw new HTTPResponseError(404, "project not found"); });
    const failure = vi.fn();
    const coordinator = new ProjectMutationCoordinator({
      ...host(detail, failure), projectDeleted, reloadList, reloadDetail,
    });

    await coordinator.deleteProject(projectEvent(detail));

    expect(reloadList).toHaveBeenCalledTimes(1);
    expect(reloadDetail).toHaveBeenCalledTimes(1);
    expect(projectDeleted).toHaveBeenCalledTimes(1);
    expect(failure).toHaveBeenCalledWith(expect.objectContaining({ message: "response lost" }));
  });

  it("reports an authoritative reconciliation failure explicitly", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("mutation failed"); }));
    const detail = projectDetail("saveProject");
    const failure = vi.fn();
    const coordinator = new ProjectMutationCoordinator({
      ...host(detail, failure),
      reloadList: async () => { throw new Error("list unavailable"); },
    });

    await coordinator.saveProject(projectEvent(detail));

    expect(failure).toHaveBeenCalledWith(expect.objectContaining({
      message: "mutation failed Authoritative project reconciliation failed: list unavailable",
    }));
  });
});

function projectDetail(action: string): HTMLElement {
  const detail = document.createElement("div");
  detail.innerHTML = `
    <input data-projects-field="name" value="Project">
    <input data-projects-field="location" value="/srv/project">
    <textarea data-projects-field="description">Exact</textarea>
    <button data-action="projects#${action}" data-project-id="7" data-project-updated-at="2026-08-13T12:00:00Z">Act</button>
  `;
  document.body.append(detail);
  return detail;
}

function projectEvent(detail: HTMLElement): Event {
  const button = detail.querySelector("button");
  if (!(button instanceof HTMLButtonElement)) throw new Error("Missing project mutation button.");
  return { preventDefault() {}, stopPropagation() {}, currentTarget: button } as unknown as Event;
}

function host(detail: HTMLElement, failure: (error: unknown) => void): ProjectMutationHost {
  return {
    detailRoot: () => detail,
    reloadDetail: async () => undefined,
    reloadList: async () => undefined,
    projectDeleted: () => undefined,
    setStatus: () => undefined,
    success: () => undefined,
    failure,
  };
}

function jsonResponse(method: string): Response {
  if (method === "deleteProject") return new Response('{"commit_state":"committed","project_id":7,"expected_updated_at":"2026-08-13T12:00:00Z","deleted":true}', { status: 200 });
  if (method === "saveScrumAutomation") return new Response(JSON.stringify({
    commit_state: "committed",
    auto_work: { enabled: true, source_columns: ["assigned"] },
  }), { status: 200 });
  return new Response(JSON.stringify({
    commit_state: "committed",
    project: {
      id: 7, name: "Project", location: "/srv/project", description: "Exact", project_state: "",
      last_seen_at: "2026-08-13T12:00:00Z", created_at: "2026-08-13T12:00:00Z", updated_at: "2026-08-13T12:00:01Z",
    },
  }), { status: 200 });
}
