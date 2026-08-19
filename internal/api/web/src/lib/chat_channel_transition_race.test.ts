import { beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel, fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import type { ChannelCreationContext } from "./channel_api";
import { fetchChannelOptionsPage } from "./chat_component_api";
import {
  ChatChannelCoordinator,
  NEW_CONVERSATION_OPTION_VALUE,
  type ChatChannelHost,
} from "./chat_channel_coordinator";
import { ChatChannelTransitionGate } from "./chat_channel_transition_gate";
import type { UserChannel } from "./types";

vi.mock("./channel_api", () => ({
  createUserChannel: vi.fn(),
  fetchChannelTranscript: vi.fn(),
  sendChannelMessage: vi.fn(),
}));
vi.mock("./chat_component_api", () => ({ fetchChannelOptionsPage: vi.fn() }));

const first: UserChannel = {
  id: "chat-42",
  scope: "user",
  name: "First conversation",
  tags: ["user-channel"],
  project_id: 42,
  workspace_root: "/workspace/project",
  mode: "assistant",
  created_at: "2026-08-19T02:00:00Z",
  updated_at: "2026-08-19T02:00:00Z",
};
const second: UserChannel = { ...first, id: "chat-43", name: "Second conversation" };

function optionsBundle(...channels: UserChannel[]): string {
  const options = channels.map((channel) =>
    `<option value="${channel.id}" data-channel-mode="${channel.mode}">${channel.name}</option>`,
  ).join("");
  return `<template data-recyclr-target="channel-options" data-recyclr-location="innerHTML">` +
    `<option value="" disabled selected>Choose a conversation</option>` +
    `<option value="${NEW_CONVERSATION_OPTION_VALUE}">+ New conversation…</option>${options}</template>`;
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
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((accept, decline) => { resolve = accept; reject = decline; });
  return { promise, resolve, reject };
}

function createHost(initial: UserChannel[] = []) {
  const channelSelect = document.createElement("select");
  const parsed = new DOMParser().parseFromString(optionsBundle(...initial), "text/html");
  channelSelect.innerHTML = parsed.querySelector("template")?.innerHTML ?? "";
  const waitForJob = vi.fn(async (_id: number) => undefined);
  const synchronizeRoleplay = vi.fn(async (_id: string, _mode: "assistant" | "roleplay") => undefined);
  const refreshRoleplay = vi.fn(async () => undefined);
  const renderComponentBundle = vi.fn(async (bundle: string) => {
    const documentFragment = new DOMParser().parseFromString(bundle, "text/html");
    const template = documentFragment.querySelector<HTMLTemplateElement>('template[data-recyclr-target="channel-options"]');
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
    setBusy: vi.fn(),
    waitForJob,
    synchronizeRoleplay,
    roleplayConfigured: () => true,
    refreshRoleplay,
  };
  return { host, channelSelect, waitForJob, synchronizeRoleplay, refreshRoleplay };
}

function selectEvent(select: HTMLSelectElement): Event {
  return { currentTarget: select } as unknown as Event;
}

function acceptedTurn(channel: UserChannel, prompt: string) {
  return {
    channel,
    user_message: { id: 91, channel_id: channel.id, role: "user" as const, content: prompt, created_at: channel.created_at },
    job: { id: 73, instruction: prompt, pipeline: "chat" as const, status: "pending" as const },
  };
}

describe("ChatChannelCoordinator transition serialization", () => {
  beforeEach(() => vi.resetAllMocks());

  it("serializes two New events and keeps the canonical select disabled through both", async () => {
    const firstCreate = deferred<UserChannel>();
    const secondCreate = deferred<UserChannel>();
    vi.mocked(createUserChannel)
      .mockReturnValueOnce(firstCreate.promise)
      .mockReturnValueOnce(secondCreate.promise);
    vi.mocked(fetchChannelOptionsPage)
      .mockResolvedValueOnce({ has_more: false, html: { bundle: optionsBundle(first) } })
      .mockResolvedValueOnce({ has_more: false, html: { bundle: optionsBundle(second) } });
    vi.mocked(fetchChannelTranscript).mockImplementation(async (id) => transcript(id));
    const fixture = createHost();
    const identityFactory = vi.fn()
      .mockReturnValueOnce({ id: first.id, name: first.name })
      .mockReturnValueOnce({ id: second.id, name: second.name });
    const coordinator = new ChatChannelCoordinator(fixture.host, identityFactory);
    fixture.channelSelect.value = NEW_CONVERSATION_OPTION_VALUE;

    const firstTransition = coordinator.select(selectEvent(fixture.channelSelect));
    const secondTransition = coordinator.select(selectEvent(fixture.channelSelect));
    await vi.waitFor(() => expect(createUserChannel).toHaveBeenCalledTimes(1));
    expect(fixture.channelSelect.disabled).toBe(true);

    firstCreate.resolve(first);
    await vi.waitFor(() => expect(createUserChannel).toHaveBeenCalledTimes(2));
    expect(vi.mocked(fetchChannelTranscript).mock.invocationCallOrder[0])
      .toBeLessThan(vi.mocked(createUserChannel).mock.invocationCallOrder[1]);
    expect(fixture.channelSelect.disabled).toBe(true);

    secondCreate.resolve(second);
    await Promise.all([firstTransition, secondTransition]);
    expect(identityFactory).toHaveBeenCalledTimes(2);
    expect(coordinator.selectedID()).toBe(second.id);
    expect(fixture.channelSelect.disabled).toBe(false);
  });

  it("holds neutral create and exact send together before a queued selection", async () => {
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
    const coordinator = new ChatChannelCoordinator(
      fixture.host,
      () => ({ id: first.id, name: first.name }),
    );

    const sending = coordinator.createAndSubmit(prompt);
    await vi.waitFor(() => expect(createUserChannel).toHaveBeenCalledOnce());
    fixture.channelSelect.value = second.id;
    const selecting = coordinator.select(selectEvent(fixture.channelSelect));
    created.resolve(first);
    await vi.waitFor(() => expect(fixture.waitForJob).toHaveBeenCalledWith(73));

    expect(sendChannelMessage).toHaveBeenCalledWith(first.id, prompt);
    expect(coordinator.selectedID()).toBe(first.id);
    expect(vi.mocked(fetchChannelTranscript).mock.calls.some(([id]) => id === second.id)).toBe(false);
    expect(fixture.channelSelect.disabled).toBe(true);

    completed.resolve();
    await expect(sending).resolves.toBe("submitted");
    await selecting;
    expect(coordinator.selectedID()).toBe(second.id);
    expect(fixture.refreshRoleplay.mock.invocationCallOrder[0])
      .toBeLessThan(fixture.synchronizeRoleplay.mock.invocationCallOrder.at(-1) ?? 0);
    expect(fixture.channelSelect.disabled).toBe(false);
  });

  it("does not let a queued selection retarget an existing-channel turn", async () => {
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

    const sending = coordinator.submit("exact");
    await vi.waitFor(() => expect(fixture.waitForJob).toHaveBeenCalledOnce());
    fixture.channelSelect.value = second.id;
    const selecting = coordinator.select(selectEvent(fixture.channelSelect));
    expect(vi.mocked(fetchChannelTranscript).mock.calls.some(([id]) => id === second.id)).toBe(false);

    completed.resolve();
    await sending;
    await selecting;
    expect(sendChannelMessage).toHaveBeenCalledWith(first.id, "exact");
    expect(coordinator.selectedID()).toBe(second.id);
  });

  it("restores a preexisting fail-closed disabled select", async () => {
    const select = document.createElement("select");
    select.disabled = true;
    const gate = new ChatChannelTransitionGate({ hasChannelSelect: () => true, channelSelect: () => select });

    await gate.run(async () => undefined);

    expect(select.disabled).toBe(true);
  });
});
