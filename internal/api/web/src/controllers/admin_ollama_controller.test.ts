import { Application, Controller } from "@hotwired/stimulus";
import { afterEach, describe, expect, it, vi } from "vitest";
import AdminController from "./admin_controller";

describe("AdminController Ollama downloads", () => {
  afterEach(() => {
    document.body.innerHTML = "";
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("clears the submitted model immediately while the durable enqueue is pending", async () => {
    let release!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => { release = resolve; });
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/v1/ui/admin?tab=overview") {
        return componentResponse(`<template data-recyclr-target="admin-tab-panel"><p>Overview</p></template>`);
      }
      if (url === "/v1/ollama/models" && init?.method === "POST") return pending;
      throw new Error(`Unexpected fetch: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    document.body.dataset.controller = "recyclr";
    document.body.innerHTML = `
      <section data-controller="admin">
        <nav data-admin-target="tabNav"></nav>
        <span data-admin-target="adminStatus"></span>
        <div data-admin-target="tabPanel" data-recyclr-sink="admin-tab-panel"></div>
        <form><input data-admin-target="pullModel" value="dolphin3:latest" /></form>
      </section>
      <div id="recyclr-global-loading-indicator" class="hidden"></div>
      <div id="omni-toast-root"><div id="omni-toast" hidden></div></div>
    `;
    const application = Application.start();
    application.register("recyclr", TestRecyclrController);
    application.register("admin", AdminController);
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/v1/ui/admin?tab=overview", undefined));
    const element = document.querySelector<HTMLElement>("[data-controller='admin']");
    if (!element) throw new Error("Admin controller element is missing.");
    const controller = application.getControllerForElementAndIdentifier(element, "admin") as AdminController | null;
    if (!controller) throw new Error("Admin controller did not connect.");
    const input = document.querySelector<HTMLInputElement>("[data-admin-target='pullModel']");
    if (!input) throw new Error("Pull model field is missing.");

    const submitting = controller.pullModel({ preventDefault() {} } as Event);
    expect(input.value).toBe("");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const request = fetchMock.mock.calls[1];
    expect(request?.[0]).toBe("/v1/ollama/models");
    expect(JSON.parse(String(request?.[1]?.body))).toEqual({ model: "dolphin3:latest" });

    release(new Response(JSON.stringify({ download: { state: "queued" } }), { status: 202 }));
    await submitting;
    application.stop();
  });
});

function componentResponse(bundle: string): Response {
  return new Response(JSON.stringify({ html: { bundle } }), {
    status: 200, headers: { "Content-Type": "application/json" },
  });
}

class TestRecyclrController extends Controller {
  async renderBundle(bundle: string): Promise<void> {
    const fragment = new DOMParser().parseFromString(bundle, "text/html");
    for (const template of fragment.querySelectorAll<HTMLTemplateElement>("template[data-recyclr-target]")) {
      const target = document.querySelector(`[data-recyclr-sink='${template.dataset.recyclrTarget}']`);
      if (!target) throw new Error("Test Recyclr target is unavailable.");
      target.replaceChildren(template.content.cloneNode(true));
    }
  }

  pushRoute(): void {}
}
