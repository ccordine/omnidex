import { beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel, fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import type { ChannelCreationContext } from "./channel_api";
import { fetchChannelOptionsPage, fetchNeutralChatTranscript } from "./chat_component_api";
import { ChatChannelCoordinator, type ChatChannelHost } from "./chat_channel_coordinator";
import { ChatChannelTransitionGate } from "./chat_channel_transition_gate";
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

const first: UserChannel = {
  id: "chat-42", scope: "user", name: "First conversation", tags: ["user-channel"],
  project_id: 42, workspace_root: "/workspace/project", mode: "assistant",
  created_at: "2026-08-19T02:00:00Z", updated_at: "2026-08-19T02:00:00Z",
};
const second: UserChannel = { ...first, id: "chat-43", name: "Second conversation" };

function optionsBundle(...channels: UserChannel[]): string {
  const options = channels.map((channel) =>
    `<option value="${channel.id}" data-channel-mode="${channel.mode}">${channel.name}</option>`,
  ).join("");
  return `<template data-recyclr-target="channel-options" data-recyclr-location="innerHTML">` +
    `<option value="" disabled selected>New conversation</option>${options}</template>`;
}

function transcript(channelID: string) {
  return {
    channel_id: channelID,
    has_more: false,
    html: { bundle: '<template data-recyclr-target="channel-transcript-messages">transcript</template>' },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => { resolve = accept; });
  return { promise, resolve };
}

function createHost(initial: UserChannel[] = []) {
  const channelSelect = document.createElement("select");
  const parsed = new DOMParser().parseFromString(optionsBundle(...initial), "text/html");
  channelSelect.innerHTML = parsed.querySelector("template")?.innerHTML ?? "";
  const waitForJob = vi.fn(async (_id: number) => undefined);
  const synchronizeRoleplay = vi.fn(async (_id: string, _mode: "assistant" | "roleplay") => undefined);
  const refreshRoleplay = vi.fn(async () => undefined);
  const renderComponentBundle = vi.fn(async (bundle: string) => {
    const fragment = new DOMParser().parseFromString(bundle, "text/html");
    const template = fragment.querySelector<HTMLTemplateElement>('template[data-recyclr-target="channel-options"]');
    if (template) channelSelect.innerHTML = template.innerHTML;
  });
  const host: ChatChannelHost = {
    hasNetworkURL: () => false,
    networkURL: () => document.createElement("a"),
    hasTransport: () => false,
    transport: () => document.createElement("div"),
    hasChannelSelect: () => true,
    channelSelect: () => channelSelect,
    queueEnabled: () => true,
    setQueueEnabled: vi.fn(),
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    renderComponentBundle,
    renderTranscriptBundle: vi.fn(async () => undefined),
    workspaceRoot: () => first.workspace_root,
    newChannelDataSourceID: () => undefined,
    newChannelCreationContext: vi.fn<() => ChannelCreationContext>(() => ({ mode: "assistant" })),
    setActivityLabel: vi.fn(),
    renderProgressActivity: vi.fn(),
    waitForJob,
    synchronizeRoleplay,
    roleplayConfigured: () => true,
    refreshRoleplay,
  };
  return { host, channelSelect, waitForJob, synchronizeRoleplay, refreshRoleplay };
}

function acceptedTurn(channel: UserChannel, prompt: string) {
  return {
    channel,
    user_message: { id: 91, channel_id: channel.id, role: "user" as const, content: prompt, created_at: channel.created_at },
    job: { id: 73, instruction: prompt, pipeline: "chat" as const, status: "pending" as const },
  };
}

function selectEvent(select: HTMLSelectElement): Event {
  return { currentTarget: select } as unknown as Event;
}

describe("ChatChannelCoordinator transition serialization", () => {
  beforeEach(() => vi.resetAllMocks());

  it("releases channel navigation after server acceptance while completion stays realtime", async () => {
    const created = deferred<UserChannel>();
    const completed = deferred<void>();
    const prompt = "  exact neutral prompt\n  ";
    vi.mocked(createUserChannel).mockReturnValueOnce(created.promise);
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: optionsBundle(first, second) },
    });
    vi.mocked(fetchChannelTranscript).mockImplementation(async (id) => transcript(id));
    vi.mocked(sendChannelMessage).mockResolvedValueOnce(acceptedTurn(first, prompt));
    const fixture = createHost([second]);
    fixture.waitForJob.mockReturnValueOnce(completed.promise);
    const coordinator = new ChatChannelCoordinator(fixture.host, () => ({ id: first.id, name: first.name }));

    const sending = coordinator.createAndSubmit(prompt);
    await vi.waitFor(() => expect(createUserChannel).toHaveBeenCalledOnce());
    created.resolve(first);
    const result = await sending;
    expect(result.kind).toBe("submitted");
    expect(fixture.channelSelect.disabled).toBe(false);

    fixture.channelSelect.value = second.id;
    await coordinator.select(selectEvent(fixture.channelSelect));
    expect(coordinator.selectedID()).toBe(second.id);
    expect(sendChannelMessage).toHaveBeenCalledWith(first.id, prompt);

    completed.resolve();
    if (result.kind === "submitted") await coordinator.reconcileTurn(result.turn);
    expect(fixture.refreshRoleplay).not.toHaveBeenCalled();
  });

  it("does not let a selection retarget an accepted existing-channel turn", async () => {
    const completed = deferred<void>();
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({
      has_more: false,
      html: { bundle: optionsBundle(first, second) },
    });
    vi.mocked(fetchChannelTranscript).mockImplementation(async (id) => transcript(id));
    vi.mocked(sendChannelMessage).mockResolvedValueOnce(acceptedTurn(first, "exact"));
    const fixture = createHost();
    fixture.waitForJob.mockReturnValueOnce(completed.promise);
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    fixture.channelSelect.value = first.id;
    await coordinator.select(selectEvent(fixture.channelSelect));

    const receipt = await coordinator.submit("exact");
    fixture.channelSelect.value = second.id;
    await coordinator.select(selectEvent(fixture.channelSelect));
    expect(coordinator.selectedID()).toBe(second.id);

    completed.resolve();
    await coordinator.reconcileTurn(receipt);
    expect(sendChannelMessage).toHaveBeenCalledWith(first.id, "exact");
    expect(fixture.refreshRoleplay).not.toHaveBeenCalled();
  });

  it("serializes a server-rendered neutral reset with channel selection", async () => {
    const neutral = deferred<{ has_more: boolean; html: { bundle: string } }>();
    vi.mocked(fetchChannelOptionsPage).mockResolvedValueOnce({ has_more: false, html: { bundle: optionsBundle(first) } });
    vi.mocked(fetchChannelTranscript).mockImplementation(async (id) => transcript(id));
    vi.mocked(fetchNeutralChatTranscript).mockReturnValueOnce(neutral.promise);
    const fixture = createHost();
    const coordinator = new ChatChannelCoordinator(fixture.host);
    await coordinator.loadChannels();
    fixture.channelSelect.value = first.id;
    await coordinator.select(selectEvent(fixture.channelSelect));

    const resetting = coordinator.beginNewConversation();
    expect(fixture.channelSelect.disabled).toBe(true);
    neutral.resolve({ has_more: false, html: { bundle: "neutral" } });
    await resetting;
    expect(coordinator.selectedID()).toBe("");
    expect(fixture.channelSelect.disabled).toBe(false);
  });

  it("restores a preexisting fail-closed disabled select", async () => {
    const select = document.createElement("select");
    select.disabled = true;
    const gate = new ChatChannelTransitionGate({ hasChannelSelect: () => true, channelSelect: () => select });
    await gate.run(async () => undefined);
    expect(select.disabled).toBe(true);
  });
});
