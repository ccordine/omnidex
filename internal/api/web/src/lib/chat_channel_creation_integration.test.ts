import { beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel, fetchChannelTranscript } from "./channel_api";
import type { ChannelCreationContext } from "./channel_api";
import { fetchChannelOptionsPage } from "./chat_component_api";
import {
  ChatChannelCoordinator,
  NEW_CONVERSATION_OPTION_VALUE,
  type ChatChannelHost,
} from "./chat_channel_coordinator";

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
  mode: "assistant" as const,
  created_at: "2026-08-19T02:00:00Z",
  updated_at: "2026-08-19T02:00:00Z",
};
const secondIdentity = { id: "chat-43", name: "Conversation second" };

function channelBundle(
  id = channel.id,
  name = channel.name,
  mode: "assistant" | "roleplay" = "assistant",
): string {
  return `<template data-recyclr-target="channel-options" data-recyclr-location="innerHTML">` +
    `<option value="" disabled selected>Choose a conversation</option>` +
    `<option value="${NEW_CONVERSATION_OPTION_VALUE}">+ New conversation…</option>` +
    `<option value="${id}" data-channel-mode="${mode}">${name}</option></template>`;
}

function transcript(channelID = channel.id) {
  return {
    channel_id: channelID,
    has_more: false,
    html: { bundle: '<template data-recyclr-target="channel-transcript-messages">transcript</template>' },
  };
}

function createHost() {
  const channelSelect = document.createElement("select");
  const transport = document.createElement("div");
  channelSelect.innerHTML = `<option value="" disabled selected>Choose a conversation</option>` +
    `<option value="${NEW_CONVERSATION_OPTION_VALUE}">+ New conversation…</option>`;
  const workspaceRoot = vi.fn<() => string | null>(() => channel.workspace_root);
  const dataSourceID = vi.fn<() => string | undefined>(() => undefined);
  const creationContext = vi.fn<() => ChannelCreationContext>(() => ({ mode: "assistant" }));
  const renderComponentBundle = vi.fn(async (bundle: string) => {
    const parsed = new DOMParser().parseFromString(bundle, "text/html");
    const template = parsed.querySelector<HTMLTemplateElement>('template[data-recyclr-target="channel-options"]');
    if (template) channelSelect.innerHTML = template.innerHTML;
  });
  const host: ChatChannelHost = {
    hasNetworkURL: () => false,
    networkURL: () => document.createElement("a"),
    hasTransport: () => true,
    transport: () => transport,
    hasChannelSelect: () => true,
    channelSelect: () => channelSelect,
    queueEnabled: () => true,
    setQueueEnabled: vi.fn(),
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    renderComponentBundle,
    renderTranscriptBundle: vi.fn(async () => undefined),
    workspaceRoot,
    newChannelDataSourceID: dataSourceID,
    newChannelCreationContext: creationContext,
    setActivityLabel: vi.fn(),
    renderProgressActivity: vi.fn(),
    setBusy: vi.fn(),
    waitForJob: vi.fn(async () => undefined),
    synchronizeRoleplay: vi.fn(async () => undefined),
    roleplayConfigured: vi.fn(() => true),
    refreshRoleplay: vi.fn(async () => undefined),
  };
  return { host, channelSelect, workspaceRoot, dataSourceID, creationContext };
}

function selectEvent(select: HTMLSelectElement): Event {
  return { currentTarget: select } as unknown as Event;
}

describe("ChatChannelCoordinator creation authority", () => {
  beforeEach(() => vi.resetAllMocks());

  it("creates immediately from the sentinel with exact assistant data settings", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce({ ...channel, data_source_id: "ds.primary-1" });
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: channelBundle(channel.id, "Exact chat · data connected") },
    });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce(transcript());
    const fixture = createHost();
    fixture.dataSourceID.mockReturnValue("ds.primary-1");
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;

    await coordinator.select(selectEvent(fixture.channelSelect));

    expect(createUserChannel).toHaveBeenCalledWith({
      id: channel.id,
      name: channel.name,
      tags: ["user-channel"],
      workspace_root: channel.workspace_root,
      data_source_id: "ds.primary-1",
      mode: "assistant",
    });
    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
    expect(fixture.host.transport().textContent).not.toContain(NEW_CONVERSATION_OPTION_VALUE);
  });

  it("omits workspace authority when no project is open and accepts the server binding", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce(channel);
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({ has_more: false, html: { bundle: channelBundle() } });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce(transcript());
    const fixture = createHost();
    fixture.workspaceRoot.mockReturnValue(null);
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;

    await coordinator.select(selectEvent(fixture.channelSelect));

    expect(createUserChannel).toHaveBeenCalledWith({
      id: channel.id,
      name: channel.name,
      tags: ["user-channel"],
      mode: "assistant",
    });
    expect(coordinator.selectedID()).toBe(channel.id);
  });

  it("restores the prior explicit channel after a pre-201 create failure", async () => {
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({ has_more: false, html: { bundle: channelBundle() } });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce(transcript());
    vi.mocked(createUserChannel).mockRejectedValueOnce(new Error("database unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host, () => secondIdentity);
    await coordinator.loadChannels();
    fixture.channelSelect.value = channel.id;
    await coordinator.select(selectEvent(fixture.channelSelect));
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;

    await coordinator.select(selectEvent(fixture.channelSelect));

    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
    expect(createUserChannel).toHaveBeenCalledWith(expect.objectContaining(secondIdentity));
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith("database unavailable", "error");
  });

  it("passes exact roleplay creation settings through the single typed flow", async () => {
    const roleplay = {
      ...channel,
      mode: "roleplay" as const,
      roleplay_viewpoint_character_id: "rpc_0123456789abcdef0123456789abcdef",
    };
    vi.mocked(createUserChannel).mockResolvedValueOnce(roleplay);
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: channelBundle(channel.id, channel.name, "roleplay") },
    });
    vi.mocked(fetchChannelTranscript).mockResolvedValueOnce(transcript());
    const fixture = createHost();
    fixture.creationContext.mockReturnValue({
      mode: "roleplay",
      roleplay_world_name: "Harbor Kingdom",
      roleplay_viewpoint_name: "Alice",
    });
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;

    await coordinator.select(selectEvent(fixture.channelSelect));

    expect(createUserChannel).toHaveBeenCalledWith({
      id: channel.id,
      name: channel.name,
      tags: ["user-channel"],
      workspace_root: channel.workspace_root,
      mode: "roleplay",
      roleplay_world_name: "Harbor Kingdom",
      roleplay_viewpoint_name: "Alice",
    });
    expect(fixture.host.synchronizeRoleplay).toHaveBeenCalledWith(channel.id, "roleplay");
  });

  it("retains post-201 authority and blocks duplicate creation when reconciliation fails", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce(channel);
    vi.mocked(fetchChannelOptionsPage).mockRejectedValueOnce(new Error("options unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;

    await coordinator.select(selectEvent(fixture.channelSelect));
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;
    await coordinator.select(selectEvent(fixture.channelSelect));

    expect(createUserChannel).toHaveBeenCalledOnce();
    expect(coordinator.selectedID()).toBe("");
    expect(fixture.channelSelect.value).toBe("");
    expect(fixture.host.addEvent).toHaveBeenCalledWith("channel_creation_reconciliation_failed", {
      channel_id: channel.id,
      error: "options unavailable",
    });
    expect(fixture.host.addEvent).toHaveBeenCalledWith(
      "channel_create_blocked_pending_reconciliation",
      { channel_id: channel.id },
    );
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith(
      `Conversation ${channel.id} was created but could not be reconciled. Reload and select it before creating another.`,
      "error",
    );
  });
});
