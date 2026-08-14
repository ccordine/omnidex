import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectBrowserCoordinator, type ProjectBrowserHost } from "./project_browser_coordinator";

describe("project browser mutation authority", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("locks project creation and suppresses a second request", async () => {
    let release!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => { release = resolve; });
    const fetchMock = vi.fn(async () => pending);
    vi.stubGlobal("fetch", fetchMock);
    const { coordinator, modal } = fixture();
    const form = modal.querySelector("form");
    if (!(form instanceof HTMLFormElement)) throw new Error("Missing project create form.");
    const event = { preventDefault() {}, currentTarget: form } as unknown as Event;

    const first = coordinator.submitCreate(event);
    await vi.waitFor(() => expect(modal.querySelector("button")).toBeDisabled());
    expect(form).toHaveAttribute("aria-busy", "true");
    await coordinator.submitCreate(event);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    release(projectResponse());
    await first;
    expect(form).toHaveAttribute("aria-busy", "false");
    expect(modal.querySelector("button")).not.toBeDisabled();
  });

  it("reloads the authoritative project inventory after a lost create response", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("response lost"); }));
    const reloadProjects = vi.fn(async () => undefined);
    const { coordinator, modal } = fixture({ reloadProjects });
    const form = modal.querySelector("form");
    if (!(form instanceof HTMLFormElement)) throw new Error("Missing project create form.");

    await coordinator.submitCreate({ preventDefault() {}, currentTarget: form } as unknown as Event);

    expect(reloadProjects).toHaveBeenCalledTimes(1);
    expect(modal.querySelector("[data-projects-modal-feedback]")?.textContent).toBe("response lost");
  });

  it("reports a failed authoritative reconciliation without hiding the original error", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("response lost"); }));
    const { coordinator, modal } = fixture({
      reloadProjects: async () => { throw new Error("inventory unavailable"); },
    });
    const form = modal.querySelector("form");
    if (!(form instanceof HTMLFormElement)) throw new Error("Missing project create form.");

    await coordinator.submitCreate({ preventDefault() {}, currentTarget: form } as unknown as Event);

    expect(modal.querySelector("[data-projects-modal-feedback]")?.textContent).toBe(
      "response lost Authoritative reconciliation failed: inventory unavailable",
    );
  });
});

function fixture(overrides: Partial<ProjectBrowserHost> = {}): {
  coordinator: ProjectBrowserCoordinator;
  modal: HTMLElement;
} {
	const toastRoot = document.createElement("div");
	toastRoot.id = "omni-toast-root";
	const toast = document.createElement("div");
	toast.id = "omni-toast";
	toastRoot.append(toast);
	document.body.append(toastRoot);
  const modal = document.createElement("div");
  modal.innerHTML = `
    <form>
      <input data-projects-field="selectedPath" value="/srv/project">
      <input data-projects-field="createName" value="Project">
      <textarea data-projects-field="createDesc">Exact</textarea>
      <button type="submit">Create</button>
      <p data-projects-modal-feedback class="hidden"></p>
    </form>
  `;
  document.body.append(modal);
  const host: ProjectBrowserHost = {
    detailRoot: () => document.body,
    modalPanel: () => modal,
    openModal: async () => undefined,
    closeModal: () => undefined,
    setStatus: () => undefined,
    projectCreated: async () => undefined,
    reloadProjects: async () => undefined,
    ...overrides,
  };
  return { coordinator: new ProjectBrowserCoordinator(host), modal };
}

function projectResponse(): Response {
  return new Response(JSON.stringify({
    commit_state: "committed",
    project: {
      id: 7, name: "Project", location: "/srv/project", description: "Exact", project_state: "",
      last_seen_at: "2026-08-13T12:00:00Z", created_at: "2026-08-13T12:00:00Z", updated_at: "2026-08-13T12:00:00Z",
    },
  }), { status: 201, headers: { "Content-Type": "application/json" } });
}
