import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { showToast } from "./toast";

describe("showToast", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    document.body.innerHTML = `
      <div id="omni-toast-root" aria-live="polite">
        <div id="omni-toast" hidden></div>
      </div>
    `;
  });

  afterEach(() => vi.useRealTimers());

  it("reuses one server-rendered surface for repeated status updates", () => {
    showToast("Connecting", "busy");
    showToast("Live", "ok");

    const root = document.getElementById("omni-toast-root")!;
    const toast = document.getElementById("omni-toast")!;
    expect(root.children).toHaveLength(1);
    expect(toast).toHaveTextContent("Live");
    expect(toast).not.toHaveAttribute("hidden");
  });

  it("fails loudly when the server-rendered surface is missing", () => {
    document.body.innerHTML = "";
    expect(() => showToast("No surface", "error")).toThrow("server-rendered toast surface");
  });
});
