import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchUserChannels } from "./channel_api";
import { ChatChannelCoordinator, type ChatChannelHost } from "./chat_channel_coordinator";

vi.mock("./channel_api", () => ({
  createUserChannel: vi.fn(),
  fetchChannelMessages: vi.fn(),
  fetchUserChannels: vi.fn(),
  isUserChannel: vi.fn(() => true),
  sendChannelMessage: vi.fn(),
}));

function createHost() {
  const networkURL = document.createElement("div");
  const transport = document.createElement("div");
  const channelSelect = document.createElement("select");
  let queueEnabled = true;
  const setStatus = vi.fn();
  const addEvent = vi.fn();
  const host: ChatChannelHost = {
    hasNetworkURL: () => true,
    networkURL: () => networkURL,
    hasTransport: () => true,
    transport: () => transport,
    hasChannelSelect: () => true,
    channelSelect: () => channelSelect,
    queueEnabled: () => queueEnabled,
    setQueueEnabled: (enabled) => { queueEnabled = enabled; },
    setStatus,
    addEvent,
    addMessage: vi.fn(),
    replaceMessages: vi.fn(),
    restorePipelineTranscript: vi.fn(),
    setActivityLabel: vi.fn(),
    renderProgressActivity: vi.fn(),
    setBusy: vi.fn(),
  };
  return { host, networkURL, channelSelect, setStatus, addEvent };
}

describe("ChatChannelCoordinator", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("surfaces channel discovery failure instead of presenting an empty channel list", async () => {
    vi.mocked(fetchUserChannels).mockRejectedValueOnce(new Error("channel database unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect(fixture.channelSelect.disabled).toBe(true);
    expect(fixture.channelSelect.options[0]?.textContent).toBe("Channels unavailable");
    expect(fixture.setStatus).toHaveBeenCalledWith("channels unavailable", "error");
    expect(fixture.addEvent).toHaveBeenCalledWith("channels_load_failed", {
      error: "channel database unavailable",
    });
  });

  it("renders a network URL as text in a real anchor", () => {
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    const url = "https://example.test/?value=<img src=x onerror=alert(1)>";

    coordinator.setNetworkURL(url);

    expect(fixture.networkURL.querySelector("img")).toBeNull();
    const anchor = fixture.networkURL.querySelector("a");
    expect(anchor?.textContent).toBe(url);
    expect(anchor?.getAttribute("href")).toBe(url);
  });
});
