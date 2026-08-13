import { beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel, fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import { fetchChannelOptionsPage } from "./chat_component_api";
import { ChatChannelCoordinator, type ChatChannelHost } from "./chat_channel_coordinator";

vi.mock("./channel_api", () => ({
  createUserChannel: vi.fn(),
  fetchChannelTranscript: vi.fn(),
  sendChannelMessage: vi.fn(),
}));
vi.mock("./chat_component_api", () => ({ fetchChannelOptionsPage: vi.fn() }));

const channel = {
  id: "chat-42",
  scope: "user" as const,
  name: "Exact chat",
  tags: ["user-channel"],
  project_id: 42,
  workspace_root: "/workspace/project",
  created_at: "2026-08-12T10:00:00Z",
  updated_at: "2026-08-12T10:00:00Z",
};
const transcript = {
  channel_id: channel.id,
  has_more: false,
  html: { bundle: '<template data-recyclr-target="channel-transcript-messages">server transcript</template>' },
};

function channelBundle(id = channel.id, name = channel.name, location = "innerHTML"): string {
  return `<template data-recyclr-target="channel-options" data-recyclr-location="${location}">` +
    `<option value="${id}">${name}</option></template>`;
}

function createHost() {
  const networkURL = document.createElement("a");
  const transport = document.createElement("div");
  const channelSelect = document.createElement("select");
  let queueEnabled = true;
  const renderComponentBundle = vi.fn(async (bundle: string) => {
    const documentFragment = new DOMParser().parseFromString(bundle, "text/html");
    for (const template of documentFragment.querySelectorAll<HTMLTemplateElement>("template[data-recyclr-target]")) {
      if (template.dataset.recyclrTarget !== "channel-options") continue;
      if (template.dataset.recyclrLocation === "beforeend") {
        channelSelect.insertAdjacentHTML("beforeend", template.innerHTML);
      } else {
        channelSelect.innerHTML = template.innerHTML;
      }
    }
  });
  const host: ChatChannelHost = {
    hasNetworkURL: () => true,
    networkURL: () => networkURL,
    hasTransport: () => true,
    transport: () => transport,
    hasChannelSelect: () => true,
    channelSelect: () => channelSelect,
    queueEnabled: () => queueEnabled,
    setQueueEnabled: (enabled) => { queueEnabled = enabled; },
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    renderComponentBundle,
    renderTranscriptBundle: vi.fn(async () => undefined),
    workspaceRoot: () => "/workspace/project",
    setActivityLabel: vi.fn(),
    renderProgressActivity: vi.fn(),
    setBusy: vi.fn(),
    waitForJob: vi.fn(async () => undefined),
  };
  return { host, networkURL, channelSelect, renderComponentBundle };
}

describe("ChatChannelCoordinator", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    const values = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      removeItem: (key: string) => { values.delete(key); },
    });
  });

  it("applies the exact server options bundle and selects its authoritative default", async () => {
    const bundle = channelBundle("internal-looking-but-authoritative", "Server scoped");
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      default_channel_id: "internal-looking-but-authoritative",
      has_more: false,
      html: { bundle },
    });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce({
      ...transcript,
      channel_id: "internal-looking-but-authoritative",
    });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect(fixture.renderComponentBundle).toHaveBeenCalledWith(bundle);
    expect(coordinator.selectedID()).toBe("internal-looking-but-authoritative");
    expect(fixture.channelSelect.value).toBe("internal-looking-but-authoritative");
  });

  it("fails closed when the server options component cannot be loaded", async () => {
    vi.mocked(fetchChannelOptionsPage).mockRejectedValueOnce(new Error("channel database unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect(fixture.channelSelect.disabled).toBe(true);
    expect(fixture.host.setStatus).toHaveBeenCalledWith("channels unavailable", "error");
    expect(fixture.host.addEvent).toHaveBeenCalledWith("channels_load_failed", {
      error: "channel database unavailable",
    });
  });

  it("updates the pre-rendered network anchor without creating component markup", () => {
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    const url = "https://example.test/?value=<img src=x onerror=alert(1)>";

    coordinator.setNetworkURL(url);

    expect(fixture.networkURL.textContent).toBe(url);
    expect(fixture.networkURL.getAttribute("href")).toBe(url);
    expect(fixture.networkURL.querySelector("img")).toBeNull();
  });

  it("does not commit a selection whose transcript cannot be confirmed", async () => {
    const first = channelBundle();
    const second = channelBundle("chat-43", "Second", "beforeend");
    vi.mocked(fetchChannelOptionsPage)
      .mockResolvedValueOnce({ default_channel_id: channel.id, has_more: true, next_offset: 1, html: { bundle: first } })
      .mockResolvedValueOnce({ has_more: false, html: { bundle: second } });
    vi.mocked(fetchChannelTranscript)
      .mockResolvedValueOnce(transcript)
      .mockRejectedValueOnce(new Error("database unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    const pageButton = document.createElement("button");
    pageButton.dataset.pageSection = "channels";
    pageButton.dataset.nextOffset = "1";
    await coordinator.loadMoreChannels({ currentTarget: pageButton } as unknown as Event);
    fixture.channelSelect.value = "chat-43";

    await expect(coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event))
      .rejects.toThrow("database unavailable");
    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
  });

  it("reloads the accepted message, waits for its job, then reloads the transcript", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      default_channel_id: channel.id,
      has_more: false,
      html: { bundle: channelBundle() },
    });
    vi.mocked(fetchChannelTranscript)
      .mockResolvedValueOnce(transcript)
      .mockResolvedValueOnce(transcript)
      .mockResolvedValueOnce(transcript);
    vi.mocked(sendChannelMessage).mockResolvedValueOnce({
      channel,
      user_message: {
        id: 91, channel_id: channel.id, role: "user", content: "exact request", created_at: channel.created_at,
      },
      job: { id: 73, instruction: "exact request", pipeline: "chat", status: "pending" },
    });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();

    await coordinator.submit("exact request");

    expect(fixture.host.waitForJob).toHaveBeenCalledWith(73);
    expect(fetchChannelTranscript).toHaveBeenNthCalledWith(2, channel.id, {
      beforeID: undefined,
      requiredMessageID: 91,
    });
    expect(fetchChannelTranscript).toHaveBeenCalledTimes(3);
  });

  it("loads older markup only from the server cursor and preserves loading on success", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      default_channel_id: channel.id,
      has_more: false,
      html: { bundle: channelBundle() },
    });
    vi.mocked(fetchChannelTranscript)
      .mockResolvedValueOnce(transcript)
      .mockResolvedValueOnce({ ...transcript, html: { bundle: "older-server-bundle" } });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    const button = document.createElement("button");
    button.dataset.beforeId = "41";

    await coordinator.loadOlder({ currentTarget: button } as unknown as Event);

    expect(fetchChannelTranscript).toHaveBeenLastCalledWith(channel.id, {
      beforeID: 41,
      requiredMessageID: undefined,
    });
    expect(fixture.host.renderTranscriptBundle).toHaveBeenLastCalledWith("older-server-bundle", true);
    expect(button.disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("true");
  });
});
