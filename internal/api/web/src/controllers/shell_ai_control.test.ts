import { Application } from "@hotwired/stimulus";
import { afterEach, describe, expect, it, vi } from "vitest";
import ShellController from "./shell_controller";

describe("global AI control interaction authority", () => {
  afterEach(() => {
    document.body.innerHTML = "";
    vi.unstubAllGlobals();
  });

  it("locks the control while one exact mutation is pending", async () => {
    let release!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => { release = resolve; });
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === "/v1/ai/control" && fetchMock.mock.calls.length === 1) {
        return new Response(JSON.stringify(state()), { status: 200 });
      }
      return pending;
    });
    vi.stubGlobal("fetch", fetchMock);
    document.body.innerHTML = `
      <div data-controller="shell">
        <button data-shell-target="aiControlButton" data-action="shell#toggleAIControl">Pause</button>
        <span data-shell-target="aiControlStatus"></span>
      </div>
      <div id="omni-toast-root"><div id="omni-toast" hidden></div></div>
    `;
    const application = Application.start();
    application.register("shell", ShellController);
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const button = document.querySelector("button");
    if (!(button instanceof HTMLButtonElement)) throw new Error("AI control button is missing.");

    button.click();
    button.click();
    await vi.waitFor(() => expect(button).toBeDisabled());
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    release(new Response(JSON.stringify({
      commit_state: "committed",
      ...state(),
      paused: true,
      canceled_jobs: 2,
      realtime_published: true,
    }), { status: 200 }));
    await vi.waitFor(() => expect(button).not.toBeDisabled());
    expect(button).toHaveAttribute("aria-busy", "false");
    application.stop();
  });
});

function state(): Record<string, unknown> {
  return {
    paused: false,
    canceled_jobs: 0,
    resumed: false,
    counts: { pending: 0, running: 0, waiting_input: 0 },
    updated_at: "2026-08-13T12:00:00.123456Z",
  };
}
