import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ScrumFeedback } from "./scrum_feedback";

describe("ScrumFeedback", () => {
  let status: HTMLElement;

  beforeEach(() => {
    document.body.innerHTML = `
      <p data-status></p>
      <div id="omni-toast-root"><div id="omni-toast" hidden></div></div>
    `;
    const element = document.querySelector<HTMLElement>("[data-status]");
    if (!element) throw new Error("Test status target is unavailable.");
    status = element;
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("renders one concise status and reports failures accessibly", () => {
    const feedback = new ScrumFeedback(() => status);

    feedback.set("Synchronizing live board…", "busy");
    expect(status.textContent).toBe("Synchronizing live board…");
    expect(status.className).toContain("text-cyan-200");

    feedback.fail(new Error("Realtime connection lost"));
    expect(status.textContent).toBe("Realtime connection lost");
    expect(status.className).toContain("text-rose-300");
    expect(document.querySelector('[role="alert"]')?.textContent).toBe("Realtime connection lost");
  });

  it("rejects blank status text", () => {
    const feedback = new ScrumFeedback(() => status);
    expect(() => feedback.set("  ", "idle")).toThrow("requires a message");
  });
});
