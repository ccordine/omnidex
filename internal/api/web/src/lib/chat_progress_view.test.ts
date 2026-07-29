import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderChatProgress, type ChatProgressViewHost } from "./chat_progress_view";
import { initI18n } from "./i18n";

function createHost() {
  const state = document.createElement("div");
  const recycle = vi.fn();
  const host: ChatProgressViewHost = {
    hasProgressState: () => true,
    progressState: () => state,
    recycle,
  };
  return { host, recycle, state };
}

describe("renderChatProgress", () => {
  beforeEach(() => {
    document.documentElement.lang = "es";
    document.documentElement.dir = "ltr";
    initI18n();
  });

  it("localizes authoritative job status and progress labels", () => {
    const fixture = createHost();
    renderChatProgress(fixture.host, {
      job: { id: 14, status: "running", updated_at: "2026-07-28T12:00:00Z" },
      steps: [{ id: 2, action: "v3_coding", status: "running" }],
      contexts: [],
    });

    expect(fixture.state.textContent).toBe("en curso");
    const html = fixture.recycle.mock.calls[0][1] as string;
    expect(html).toContain("Escribiendo código…");
    expect(html).toContain("Paso actual");
    expect(html).toContain("contextos");
    expect(html).not.toContain(">running<");
  });

  it("fails loudly when authoritative timing state is missing", () => {
    const fixture = createHost();
    expect(() => renderChatProgress(fixture.host, {
      job: { id: 14, status: "running" },
      steps: [],
      contexts: [],
    })).toThrow("updated_at");
  });
});
