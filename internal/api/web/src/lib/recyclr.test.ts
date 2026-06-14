import { describe, expect, it, vi } from "vitest";
import { renderRecyclrBundle } from "./recyclr";

describe("renderRecyclrBundle", () => {
  it("resolves after a direct sink update", async () => {
    document.body.innerHTML = `<div data-recyclr-sink="app-panel"></div>`;

    const renderPromise = renderRecyclrBundle(null, "app-panel", `<p>Projects ready</p>`);
    expect(document.body).not.toHaveTextContent("Projects ready");

    await renderPromise;

    expect(document.body).toHaveTextContent("Projects ready");
  });

  it("waits for an async host render before resolving", async () => {
    document.body.innerHTML = `<div data-recyclr-sink="app-panel"></div>`;
    const host = {
      renderBundle: vi.fn(
        (bundle: string) =>
          new Promise<void>((resolve) => {
            requestAnimationFrame(() => {
              const doc = new DOMParser().parseFromString(bundle, "text/html");
              const template = doc.querySelector("[data-recyclr-target='app-panel']") as HTMLTemplateElement | null;
              document.querySelector("[data-recyclr-sink='app-panel']")!.innerHTML = template?.innerHTML ?? "";
              resolve();
            });
          }),
      ),
    };

    await renderRecyclrBundle(host, "app-panel", `<p>Host rendered projects</p>`);

    expect(host.renderBundle).toHaveBeenCalledOnce();
    expect(document.body).toHaveTextContent("Host rendered projects");
  });
});
