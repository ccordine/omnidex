import { Application } from "@hotwired/stimulus";
import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsController from "./projects_controller";

function projectsPanelHTML(): string {
  return `
    <section data-panel-name="projects">
      <span data-projects-target="status">Open to load</span>
      <div data-projects-target="list"></div>
      <div data-projects-target="detail" class="hidden"></div>
    </section>
  `;
}

describe("ProjectsController panel loading", () => {
  let application: Application | null = null;
  let fetchMock: ReturnType<typeof vi.fn>;
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    document.body.innerHTML = `
      <main data-controller="projects"></main>
      <div id="omni-toast-root"><div id="omni-toast" hidden></div></div>
    `;
    fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/v1/projects") {
        return new Response(JSON.stringify({ projects: [] }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (url === "/v1/recipes") {
        return new Response(JSON.stringify({ recipes: [], root: "" }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(async () => {
    document.body.innerHTML = "";
    await Promise.resolve();
    application?.stop();
    application = null;
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("waits for projects targets before loading a shown projects panel", async () => {
    application = Application.start();
    application.register("projects", ProjectsController);
    await Promise.resolve();

    document.dispatchEvent(new CustomEvent("omni:panel-shown", { detail: { panel: "projects" } }));
    expect(fetchMock).not.toHaveBeenCalled();

    document.querySelector("[data-controller='projects']")?.insertAdjacentHTML("beforeend", projectsPanelHTML());

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/v1/projects", { signal: undefined });
    });
    expect(document.querySelector("[data-projects-target='status']")).toHaveTextContent("0 projects");
    expect(document.querySelector("[data-projects-target='list']")).toHaveTextContent("No projects yet");
    expect(consoleError).not.toHaveBeenCalled();
  });

  it("reports a shown projects panel when required targets never mount", async () => {
    vi.useFakeTimers();
    application = Application.start();
    application.register("projects", ProjectsController);
    await Promise.resolve();

    document.dispatchEvent(new CustomEvent("omni:panel-shown", { detail: { panel: "projects" } }));
    await vi.advanceTimersByTimeAsync(2000);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(consoleError).toHaveBeenCalledWith(
      "Projects panel failed to load because required DOM targets were not mounted.",
      expect.objectContaining({
        reason: "panel-shown",
        hasStatusTarget: false,
        hasListTarget: false,
        hasDetailTarget: false,
      }),
    );
  });
});
