import { beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel, fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import type { ChannelCreationContext } from "./channel_api";
import { fetchChannelOptionsPage } from "./chat_component_api";
import { ChatChannelCoordinator, type ChatChannelHost } from "./chat_channel_coordinator";
import type { UserChannel } from "./types";

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
  id: "chat-42", scope: "user" as const, name: "Exact chat", tags: ["user-channel"],
  project_id: 42, workspace_root: "/workspace/project", mode: "assistant" as const,
  created_at: "2026-08-19T02:00:00Z", updated_at: "2026-08-19T02:00:00Z",
};

function channelBundle(mode: "assistant" | "roleplay" = "assistant"): string {
  return `<template data-recyclr-target="channel-options" data-recyclr-location="innerHTML">` +
    `<option value="" disabled selected>New conversation</option>` +
    `<option value="${channel.id}" data-channel-mode="${mode}">${channel.name}</option></template>`;
}

function transcript() {
  return {
    channel_id: channel.id,
    has_more: false,
    html: { bundle: '<template data-recyclr-target="channel-transcript-messages">transcript</template>' },
  };
}

function acceptedTurn(prompt: string, acceptedChannel: UserChannel = channel) {
  return {
    channel: acceptedChannel,
    user_message: { id: 91, channel_id: channel.id, role: "user" as const, content: prompt, created_at: channel.created_at },
    job: { id: 73, instruction: prompt, pipeline: "chat" as const, status: "pending" as const },
  };
}

function createHost() {
  const channelSelect = document.createElement("select");
  const transport = document.createElement("div");
  channelSelect.innerHTML = `<option value="" disabled selected>New conversation</option>`;
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
    waitForJob: vi.fn(async () => undefined),
    synchronizeRoleplay: vi.fn(async () => undefined),
    roleplayConfigured: vi.fn(() => true),
    refreshRoleplay: vi.fn(async () => undefined),
  };
  return { host, channelSelect, workspaceRoot, dataSourceID, creationContext };
}

describe("ChatChannelCoordinator creation authority", () => {
  beforeEach(() => vi.resetAllMocks());

  it("creates and selects one assistant conversation on the first neutral send", async () => {
    const accepted = { ...channel, data_source_id: "ds.primary-1" };
    vi.mocked(createUserChannel).mockResolvedValueOnce(accepted);
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({ has_more: false, html: { bundle: channelBundle() } });
    vi.mocked(fetchChannelTranscript).mockResolvedValue(transcript());
    vi.mocked(sendChannelMessage).mockResolvedValueOnce(acceptedTurn("exact first message", accepted));
    const fixture = createHost();
    fixture.dataSourceID.mockReturnValue("ds.primary-1");
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));

    const result = await coordinator.createAndSubmit("exact first message");

    expect(result.kind).toBe("submitted");
    expect(createUserChannel).toHaveBeenCalledWith({
      id: channel.id,
      name: channel.name,
      tags: ["user-channel"],
      workspace_root: channel.workspace_root,
      data_source_id: "ds.primary-1",
      mode: "assistant",
    });
    expect(sendChannelMessage).toHaveBeenCalledWith(channel.id, "exact first message");
    expect(coordinator.selectedID()).toBe(channel.id);
    expect(fixture.channelSelect.value).toBe(channel.id);
  });

  it("omits workspace authority when the neutral send has no open project", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce(channel);
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({ has_more: false, html: { bundle: channelBundle() } });
    vi.mocked(fetchChannelTranscript).mockResolvedValue(transcript());
    vi.mocked(sendChannelMessage).mockResolvedValueOnce(acceptedTurn("hello"));
    const fixture = createHost();
    fixture.workspaceRoot.mockReturnValue(null);
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));

    await coordinator.createAndSubmit("hello");

    expect(createUserChannel).toHaveBeenCalledWith({
      id: channel.id,
      name: channel.name,
      tags: ["user-channel"],
      mode: "assistant",
    });
  });

  it("preserves neutral authority and the prompt path after a pre-201 create failure", async () => {
    vi.mocked(createUserChannel).mockRejectedValueOnce(new Error("database unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));

    await expect(coordinator.createAndSubmit("retry me")).rejects.toThrow("database unavailable");

    expect(coordinator.selectedID()).toBe("");
    expect(sendChannelMessage).not.toHaveBeenCalled();
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith("database unavailable", "error");
  });

  it("passes exact roleplay settings through the single neutral creation flow", async () => {
    const roleplay = {
      ...channel,
      mode: "roleplay" as const,
      roleplay_viewpoint_character_id: "rpc_0123456789abcdef0123456789abcdef",
    };
    vi.mocked(createUserChannel).mockResolvedValueOnce(roleplay);
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({ has_more: false, html: { bundle: channelBundle("roleplay") } });
    vi.mocked(fetchChannelTranscript).mockResolvedValue(transcript());
    vi.mocked(sendChannelMessage).mockResolvedValueOnce(acceptedTurn("begin", roleplay));
    const fixture = createHost();
    fixture.creationContext.mockReturnValue({
      mode: "roleplay",
      roleplay_world_name: "Harbor Kingdom",
      roleplay_viewpoint_name: "Alice",
    });
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));

    await coordinator.createAndSubmit("begin");

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

  it("retains post-201 authority and blocks duplicate neutral creation after reconciliation fails", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce(channel);
    vi.mocked(fetchChannelOptionsPage).mockRejectedValueOnce(new Error("options unavailable"));
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: channel.id, name: channel.name }));

    const pendingMessage = `Conversation ${channel.id} was created but could not be reconciled. ` +
      `Reload and select it before creating another.`;
    await expect(coordinator.createAndSubmit("first")).rejects.toThrow(pendingMessage);
    await expect(coordinator.createAndSubmit("second")).rejects.toThrow(pendingMessage);

    expect(createUserChannel).toHaveBeenCalledOnce();
    expect(sendChannelMessage).not.toHaveBeenCalled();
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith(pendingMessage, "error");
    expect(fixture.host.addEvent).toHaveBeenCalledWith(
      "channel_create_blocked_pending_reconciliation",
      { channel_id: channel.id },
    );
  });
});
