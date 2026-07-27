import { describe, expect, it, vi } from "vitest";
import { renderRecyclrBundle } from "./recyclr";
import { RecyclrBundleQueue } from "./recyclr_bundle_queue";

describe("renderRecyclrBundle", () => {
  it("fails loudly when the page-scoped Recyclr host is missing", async () => {
    await expect(renderRecyclrBundle(null, "app-panel", `<p>Projects ready</p>`)).rejects.toThrow(
      "page-scoped Recyclr controller is unavailable",
    );
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

describe("RecyclrBundleQueue", () => {
  it("renders every bundle queued in the same frame without overwriting unrelated sinks", async () => {
    const render = vi.fn();
    const callbacks: Array<() => void> = [];
    const queue = new RecyclrBundleQueue(render, (callback) => {
      callbacks.push(callback);
      return Promise.resolve();
    });

    const first = queue.enqueue(`<template data-recyclr-target="board"><p>Board</p></template>`);
    const second = queue.enqueue(`<template data-recyclr-target="metrics"><p>Metrics</p></template>`);

    expect(callbacks).toHaveLength(1);
    callbacks[0]();
    await Promise.all([first, second]);

    expect(render).toHaveBeenCalledOnce();
    expect(render.mock.calls[0][0]).toEqual([
      expect.objectContaining({ selector: `[data-recyclr-sink="board"]`, selection: `<p>Board</p>` }),
      expect.objectContaining({ selector: `[data-recyclr-sink="metrics"]`, selection: `<p>Metrics</p>` }),
    ]);
  });

  it("rejects malformed bundles instead of pretending they rendered", async () => {
    const callbacks: Array<() => void> = [];
    const queue = new RecyclrBundleQueue(vi.fn(), (callback) => {
      callbacks.push(callback);
      return Promise.resolve();
    });
    const result = queue.enqueue(`<p>Missing routing target</p>`);

    callbacks[0]();

    await expect(result).rejects.toThrow("does not contain a Recyclr target");
  });
});
