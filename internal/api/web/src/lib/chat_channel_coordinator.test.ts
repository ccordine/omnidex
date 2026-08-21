import { beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel, fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import type { ChannelCreationContext } from "./channel_api";
import { fetchChannelOptionsPage, fetchNeutralChatTranscript } from "./chat_component_api";
import {
  ChatChannelCoordinator,
  type ChatChannelHost,
} from "./chat_channel_coordinator";

vi.mock("./channel_api", () => ({
  createUserChannel: vi.fn(),
  fetchChannelTranscript: vi.fn(),
  sendChannelMessage: vi.fn(),
}));
vi.mock("./chat_component_api", () => ({
  fetchChannelOptionsPage: vi.fn(),
  fetchNeutralChatTranscript: vi.fn(),
}));

const channel = {
  id: "chat-42",
  scope: "user" as const,
  name: "Exact chat",
  tags: ["user-channel"],
  project_id: 42,
  workspace_root: "/workspace/project",
  mode: "assistant" as const,
  created_at: "2026-08-12T10:00:00Z",
  updated_at: "2026-08-12T10:00:00Z",
};
const transcript = {
  channel_id: channel.id,
  has_more: false,
  html: { bundle: '<template data-recyclr-target="channel-transcript-messages">server transcript</template>' },
};

function channelBundle(
  id = channel.id,
  name = channel.name,
  location = "innerHTML",
  mode: "assistant" | "roleplay" = "assistant",
): string {
  const controls = location === "innerHTML"
    ? `<option value="" disabled selected>New conversation</option>`
    : "";
  return `<template data-recyclr-target="channel-options" data-recyclr-location="${location}">` +
    controls + `<option value="${id}" data-channel-mode="${mode}">${name}</option></template>`;
}

function emptyChannelBundle(): string {
  return `<template data-recyclr-target="channel-options" data-recyclr-location="innerHTML">` +
    `<option value="" disabled selected>New conversation</option></template>`;
}

function createHost() {
  const networkURL = document.createElement("a");
  const transport = document.createElement("div");
  const channelSelect = document.createElement("select");
  channelSelect.innerHTML = `<option value="" disabled selected>New conversation</option>`;
  let queueEnabled = true;
	const workspaceRoot = vi.fn<() => string | null>(() => "/workspace/project");
	const newChannelDataSourceID = vi.fn<() => string | undefined>(() => undefined);
  const newChannelCreationContext = vi.fn<() => ChannelCreationContext>(() => ({ mode: "assistant" }));
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
	workspaceRoot,
		newChannelDataSourceID,
    newChannelCreationContext,
    setActivityLabel: vi.fn(),
    renderProgressActivity: vi.fn(),
    waitForJob: vi.fn(async () => undefined),
		synchronizeRoleplay: vi.fn(async () => undefined),
		roleplayConfigured: () => true,
		refreshRoleplay: vi.fn(async () => undefined),
  };
  return { host, networkURL, channelSelect, renderComponentBundle, workspaceRoot, newChannelDataSourceID, newChannelCreationContext };
}

describe("ChatChannelCoordinator", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.resetAllMocks();
  });

  it("applies a nonempty server option list but waits for an explicit selection", async () => {
    const storageRead = vi.spyOn(Storage.prototype, "getItem");
    const storageWrite = vi.spyOn(Storage.prototype, "setItem");
    const bundle = channelBundle("internal-looking-but-authoritative", "Server scoped");
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle },
    });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect(fixture.renderComponentBundle).toHaveBeenCalledWith(bundle);
    expect([...fixture.channelSelect.options].map((option) => option.value)).toEqual([
      "", "internal-looking-but-authoritative",
    ]);
    expect(coordinator.selectedID()).toBe("");
    expect(fixture.channelSelect.value).toBe("");
    expect(fixture.channelSelect.disabled).toBe(false);
    expect(fetchChannelTranscript).not.toHaveBeenCalled();
	expect(storageRead).not.toHaveBeenCalled();
	expect(storageWrite).not.toHaveBeenCalled();
	expect(fixture.host.synchronizeRoleplay).toHaveBeenCalledWith("", "assistant");
  });

  it("keeps an empty server channel list in the neutral conversation state", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: emptyChannelBundle() },
    });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect([...fixture.channelSelect.options].map((option) => option.value)).toEqual([""]);
    expect(fixture.channelSelect.value).toBe("");
    expect(fixture.channelSelect.disabled).toBe(false);
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
      .mockResolvedValueOnce({ has_more: true, next_offset: 1, html: { bundle: first } })
      .mockResolvedValueOnce({ has_more: false, html: { bundle: second } });
    vi.mocked(fetchChannelTranscript)
      .mockResolvedValueOnce(transcript)
      .mockRejectedValueOnce(new Error("database unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    expect(fetchChannelOptionsPage).toHaveBeenNthCalledWith(1, 0);
    expect(fetchChannelOptionsPage).toHaveBeenNthCalledWith(2, 1);
    expect([...fixture.channelSelect.options].map((option) => option.value)).toEqual([
      "", channel.id, "chat-43",
    ]);
    fixture.channelSelect.value = channel.id;
    await coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event);
    fixture.channelSelect.value = "chat-43";

    await expect(coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event))
      .rejects.toThrow("database unavailable");
    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
  });

  it("fails closed when an automatic server page cursor does not advance", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: true,
      next_offset: 0,
      html: { bundle: channelBundle() },
    });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect(fixture.channelSelect.disabled).toBe(true);
    expect(fixture.host.addEvent).toHaveBeenCalledWith("channels_load_failed", {
      error: "The server channel page cursor did not advance.",
    });
  });

  it("fails closed when automatic channel pagination exceeds its page cap", async () => {
    vi.mocked(fetchChannelOptionsPage).mockImplementation(async (offset) => ({
      has_more: true,
      next_offset: offset + 20,
      html: { bundle: channelBundle(`chat-${offset + 42}`, `Channel ${offset}`, offset === 0 ? "innerHTML" : "beforeend") },
    }));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);

    await coordinator.loadChannels();

    expect(fetchChannelOptionsPage).toHaveBeenCalledTimes(100);
    expect(fixture.channelSelect.disabled).toBe(true);
    expect(fixture.host.addEvent).toHaveBeenCalledWith("channels_load_failed", {
      error: "Channel pagination exceeded 100 server pages.",
    });
  });

  it("returns after the accepted message and reconciles the final transcript separately", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
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
    fixture.channelSelect.value = channel.id;
    await coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event);

    const receipt = await coordinator.submit("exact request");

    expect(sendChannelMessage).toHaveBeenCalledWith(channel.id, "exact request");
    expect(fixture.host.waitForJob).toHaveBeenCalledWith(73);
    expect(fetchChannelTranscript).toHaveBeenNthCalledWith(2, channel.id, {
      beforeID: undefined,
      requiredMessageID: 91,
    });
    expect(fetchChannelTranscript).toHaveBeenCalledTimes(2);
    expect(fixture.host.refreshRoleplay).not.toHaveBeenCalled();

    await coordinator.reconcileTurn(receipt);

    expect(fetchChannelTranscript).toHaveBeenCalledTimes(3);
    expect(fixture.host.refreshRoleplay).toHaveBeenCalledOnce();
  });

  it("enters a server-rendered neutral state without eagerly creating a channel", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: channelBundle() },
    });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce(transcript);
    vi.mocked(fetchNeutralChatTranscript).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: "server-neutral-transcript" },
    });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    fixture.channelSelect.value = channel.id;
    await coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event);

    await coordinator.beginNewConversation();

    expect(coordinator.selectedID()).toBe("");
    expect(fixture.channelSelect.value).toBe("");
    expect(fixture.host.renderTranscriptBundle).toHaveBeenLastCalledWith("server-neutral-transcript", false);
    expect(createUserChannel).not.toHaveBeenCalled();
  });

  it("preserves and refreshes the selected conversation when the chat panel is rendered again", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValue({
      has_more: false,
      html: { bundle: channelBundle() },
    });
    vi.mocked(fetchChannelTranscript).mockResolvedValue(transcript);
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    fixture.channelSelect.value = channel.id;
    await coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event);

    await coordinator.loadChannels();

    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
    expect(fetchChannelTranscript).toHaveBeenCalledTimes(2);
    expect(fixture.host.synchronizeRoleplay).toHaveBeenLastCalledWith(channel.id, "assistant");
  });

  it("keeps the selected conversation and reports an exact neutral reset failure", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: channelBundle() },
    });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce(transcript);
    vi.mocked(fetchNeutralChatTranscript).mockRejectedValueOnce(new Error("neutral component unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    fixture.channelSelect.value = channel.id;
    await coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event);

    await expect(coordinator.beginNewConversation()).rejects.toThrow("neutral component unavailable");

    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith("neutral component unavailable", "error");
    expect(fixture.host.addEvent).toHaveBeenCalledWith("channel_neutral_failed", {
      error: "neutral component unavailable",
    });
  });

  it("loads older markup only from the server cursor and preserves loading on success", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: channelBundle() },
    });
    vi.mocked(fetchChannelTranscript)
      .mockResolvedValueOnce(transcript)
      .mockResolvedValueOnce({ ...transcript, html: { bundle: "older-server-bundle" } });
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    fixture.channelSelect.value = channel.id;
    await coordinator.select({ currentTarget: fixture.channelSelect } as unknown as Event);
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
