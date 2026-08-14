import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectBrowserCoordinator } from "./project_browser_coordinator";
import { ProjectMutationCoordinator } from "./project_mutation_coordinator";

function committedProjectResponse(status = 200): Response {
  return new Response(JSON.stringify({
    commit_state: "committed",
    project: {
      id: 7,
      name: "Project",
      location: "/srv/project",
      description: "",
      project_state: "",
      last_seen_at: "2026-08-13T12:00:00Z",
      created_at: "2026-08-13T12:00:00Z",
      updated_at: status === 200 ? "2026-08-13T12:00:01Z" : "2026-08-13T12:00:00Z",
    },
  }), { status, headers: { "Content-Type": "application/json" } });
}

describe("project description byte authority", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("does not normalize create or edit descriptions in the browser", async () => {
    const bodies: unknown[] = [];
    vi.stubGlobal("fetch", vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      bodies.push(JSON.parse(String(init?.body)));
      return committedProjectResponse(init?.method === "POST" ? 201 : 200);
    }));

    const modal = document.createElement("div");
    modal.innerHTML = `
      <form>
        <input data-projects-field="selectedPath" value="/srv/project">
        <input data-projects-field="createName" value=" Project ">
        <textarea data-projects-field="createDesc">  create\n\t </textarea>
        <p data-projects-modal-feedback></p>
      </form>
    `;
    document.body.append(modal);
    const browser = new ProjectBrowserCoordinator({
      detailRoot: () => document.body,
      modalPanel: () => modal,
      openModal: async () => undefined,
      closeModal: vi.fn(),
      setStatus: vi.fn(),
      projectCreated: async () => undefined,
      reloadProjects: async () => undefined,
    });
    const createForm = modal.querySelector("form");
    if (!(createForm instanceof HTMLFormElement)) throw new Error("Missing project create form.");
    await browser.submitCreate({ preventDefault() {}, currentTarget: createForm } as unknown as Event);

    const detail = document.createElement("div");
    detail.innerHTML = `
      <input data-projects-field="name" value=" Project ">
      <input data-projects-field="location" value=" /srv/project ">
      <textarea data-projects-field="description">  edit\n\t </textarea>
      <button data-action="projects#saveProject" data-project-id="7" data-project-updated-at="2026-08-13T12:00:00Z">Save project</button>
    `;
    const mutation = new ProjectMutationCoordinator({
      detailRoot: () => detail,
      reloadDetail: async () => undefined,
      reloadList: async () => undefined,
      projectDeleted: vi.fn(),
      setStatus: vi.fn(),
      success: vi.fn(),
      failure: vi.fn(),
    });
    const button = detail.querySelector("button");
    if (!(button instanceof HTMLButtonElement)) throw new Error("Missing test mutation button.");
    await mutation.saveProject({ preventDefault() {}, currentTarget: button } as unknown as Event);

    expect(bodies).toEqual([
      { name: "Project", location: "/srv/project", description: "  create\n\t " },
      { expected_updated_at: "2026-08-13T12:00:00Z", name: "Project", location: "/srv/project", description: "  edit\n\t " },
    ]);
  });
});
