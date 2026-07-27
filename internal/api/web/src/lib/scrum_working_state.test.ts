import { beforeEach, describe, expect, it, vi } from "vitest";
import { ScrumWorkingState } from "./scrum_working_state";

describe("ScrumWorkingState", () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <div data-overlay class="hidden">
        <span data-message>Idle</span>
      </div>`;
  });

  it("keeps nested work visible and restores the current operation message", () => {
    const globalLoading = vi.fn();
    const overlay = document.querySelector<HTMLElement>("[data-overlay]");
    const message = document.querySelector<HTMLElement>("[data-message]");
    const state = new ScrumWorkingState(() => ({ overlay, message }), globalLoading);

    const finishBoardLoad = state.start("Loading board…");
    const finishMove = state.start("Moving card…");

    expect(overlay?.classList.contains("flex")).toBe(true);
    expect(message?.textContent).toBe("Moving card…");
    expect(globalLoading).toHaveBeenCalledTimes(1);
    expect(globalLoading).toHaveBeenLastCalledWith(true);

    finishMove();
    expect(overlay?.classList.contains("flex")).toBe(true);
    expect(message?.textContent).toBe("Loading board…");
    expect(globalLoading).toHaveBeenCalledTimes(1);

    finishBoardLoad();
    expect(overlay?.classList.contains("hidden")).toBe(true);
    expect(globalLoading).toHaveBeenLastCalledWith(false);
  });

  it("rejects blank labels and duplicate operation completion", () => {
    const overlay = document.querySelector<HTMLElement>("[data-overlay]");
    const message = document.querySelector<HTMLElement>("[data-message]");
    if (!overlay || !message) throw new Error("Test working surface is unavailable.");
    const state = new ScrumWorkingState(
      () => ({ overlay, message }),
      () => undefined,
    );

    expect(() => state.start("   ")).toThrow("requires a visible status message");
    const finish = state.start("Working…");
    finish();
    expect(finish).toThrow("already completed");
  });
});
