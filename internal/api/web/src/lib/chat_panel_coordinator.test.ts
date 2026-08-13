import { beforeEach, describe, expect, it, vi } from "vitest";
import { ChatPanelCoordinator, type ChatPanelHost } from "./chat_panel_coordinator";

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function createFixture() {
  const root = document.createElement("div");
  for (const panel of ["chat", "jobs"]) {
    const button = document.createElement("button");
    button.className = "nav-button";
    button.dataset.panel = panel;
    root.append(button);
  }
  const renderPanel = vi.fn(async () => undefined);
  const loadPanelData = vi.fn();
  const pushRoute = vi.fn();
  const addEvent = vi.fn();
  const reportError = vi.fn();
  const host: ChatPanelHost = {
    root: () => root,
    locale: () => "en",
    renderPanel,
    loadPanelData,
    pushRoute,
    addEvent,
    reportError,
  };
  return { root, host, renderPanel, loadPanelData, pushRoute, addEvent, reportError };
}

describe("ChatPanelCoordinator", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.history.replaceState({}, "", "/chat");
  });

  it("commits navigation only after receiving the requested server panel", async () => {
	const bundle = '<template data-recyclr-target="app-panel"><section>Jobs</section></template>';
	const fetchMock = vi.fn(async (_input: RequestInfo | URL) => response({
	  panel: "jobs", locale: "en", html: { bundle },
	}));
    vi.stubGlobal("fetch", fetchMock);
    const fixture = createFixture();
    const coordinator = new ChatPanelCoordinator(fixture.host);

    await coordinator.activate("jobs", { pushHistory: true });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain("/v1/ui/panel?panel=jobs");
    expect(String(fetchMock.mock.calls[0]?.[0])).not.toContain("/v1/ui/session");
	expect(fixture.renderPanel).toHaveBeenCalledWith(bundle);
    expect(coordinator.current()).toBe("jobs");
    expect(fixture.loadPanelData).toHaveBeenCalledWith("jobs");
    expect(fixture.pushRoute).toHaveBeenCalledWith("/chat?panel=jobs");
    expect(fixture.root.querySelector('[data-panel="jobs"]')).toHaveClass("is-active");
  });

  it("rejects a mismatched server response without changing client state", async () => {
	vi.stubGlobal("fetch", vi.fn(async () => response({
	  panel: "memory", locale: "en", html: { bundle: "server-wrong-panel" },
	})));
    const fixture = createFixture();
    const coordinator = new ChatPanelCoordinator(fixture.host);

    await expect(coordinator.activate("jobs")).rejects.toThrow('requested "jobs"');

    expect(coordinator.current()).toBe("chat");
    expect(fixture.renderPanel).not.toHaveBeenCalled();
    expect(fixture.addEvent).toHaveBeenCalledWith("ui_panel_error", expect.objectContaining({ panel: "jobs" }));
    expect(fixture.reportError).toHaveBeenCalledTimes(1);
  });

  it("rejects invalid Stimulus panel actions instead of routing to a fallback", async () => {
    const fixture = createFixture();
    const coordinator = new ChatPanelCoordinator(fixture.host);
    const button = document.createElement("button");
    button.dataset.panel = "legacy";

    await expect(coordinator.show({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event)).rejects.toThrow('Invalid Omni panel "legacy"');
  });
});
