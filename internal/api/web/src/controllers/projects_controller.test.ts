import { Application, Controller } from "@hotwired/stimulus";
import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsController from "./projects_controller";

function projectsPanelHTML(): string {
  return `
    <section data-panel-name="projects">
      <span data-projects-target="status">Open to load</span>
      <div data-projects-target="list" data-recyclr-sink="projects-list"></div>
      <div data-projects-target="detail" data-recyclr-sink="project-detail" class="hidden"></div>
    </section>
  `;
}

describe("ProjectsController panel loading", () => {
  let application: Application | null = null;
  let fetchMock: ReturnType<typeof vi.fn>;
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    document.body.dataset.controller = "recyclr";
    document.body.innerHTML = `
      <main data-controller="projects"></main>
      <div id="omni-toast-root"><div id="omni-toast" hidden></div></div>
    `;
    fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/v1/ui/projects?offset=0") {
        return new Response(JSON.stringify({
          count: 0,
          html: { bundle: '<template data-recyclr-target="projects-list"><p>No projects yet</p></template>' },
        }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(async () => {
    document.body.innerHTML = "";
    delete document.body.dataset.controller;
    await Promise.resolve();
    application?.stop();
    application = null;
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("waits for projects targets before loading a shown projects panel", async () => {
    application = Application.start();
    application.register("recyclr", TestRecyclrController);
    application.register("projects", ProjectsController);
    await Promise.resolve();

    document.dispatchEvent(new CustomEvent("omni:panel-shown", { detail: { panel: "projects" } }));
    expect(fetchMock).not.toHaveBeenCalled();

    document.querySelector("[data-controller='projects']")?.insertAdjacentHTML("beforeend", projectsPanelHTML());

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/v1/ui/projects?offset=0", undefined);
    });
    expect(document.querySelector("[data-projects-target='status']")).toHaveTextContent("0 projects");
    expect(document.querySelector("[data-projects-target='list']")).toHaveTextContent("No projects yet");
    expect(consoleError).not.toHaveBeenCalled();
  });

  it("fails loudly when the server omits the required component bundle", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ count: 0, html: {} }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    document.querySelector("[data-controller='projects']")?.insertAdjacentHTML("beforeend", projectsPanelHTML());
    application = Application.start();
    application.register("recyclr", TestRecyclrController);
    application.register("projects", ProjectsController);
    await Promise.resolve();

    document.dispatchEvent(new CustomEvent("omni:panel-shown", { detail: { panel: "projects" } }));
    await waitFor(() => expect(document.querySelector("[data-projects-target='status']")).toHaveTextContent(
      "Component response did not include its required server-rendered bundle.",
    ));
  });
});

class TestRecyclrController extends Controller {
  async renderBundle(bundle: string): Promise<void> {
    const fragment = new DOMParser().parseFromString(bundle, "text/html");
    for (const template of fragment.querySelectorAll<HTMLTemplateElement>("template[data-recyclr-target]")) {
      const target = document.querySelector(`[data-recyclr-sink='${template.dataset.recyclrTarget}']`);
      if (!target) throw new Error("Test Recyclr target is unavailable.");
      target.replaceChildren(template.content.cloneNode(true));
    }
  }
}
