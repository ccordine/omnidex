import { beforeEach, describe, expect, it, vi } from "vitest";
import { sendChannelMessage } from "./channel_api";
import {
  ChatChannelTurnCoordinator,
  type ChatChannelTurnHost,
} from "./chat_channel_turn";

vi.mock("./channel_api", () => ({ sendChannelMessage: vi.fn() }));

function deferred() {
  let resolve!: () => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((accept, decline) => { resolve = accept; reject = decline; });
  return { promise, resolve, reject };
}

function createHost(completion: Promise<void>) {
  let selected = true;
  const host: ChatChannelTurnHost = {
    setStatus: vi.fn(),
    setActivityLabel: vi.fn(),
    renderProgressActivity: vi.fn(),
    addEvent: vi.fn(),
    loadTranscript: vi.fn(async () => undefined),
    waitForJob: vi.fn(() => completion),
    isSelected: () => selected,
    refreshRoleplay: vi.fn(async () => undefined),
  };
  return { host, setSelected: (value: boolean) => { selected = value; } };
}

describe("ChatChannelTurnCoordinator", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns after server acceptance while realtime completion remains pending", async () => {
    const pending = deferred();
    const acceptedTranscript = deferred();
    vi.mocked(sendChannelMessage).mockResolvedValueOnce({
      channel: {
        id: "chat-42", scope: "user", name: "Chat", tags: ["user-channel"], project_id: 1,
        workspace_root: "/workspace", mode: "assistant",
        created_at: "2026-08-19T12:00:00Z", updated_at: "2026-08-19T12:00:00Z",
      },
      user_message: {
        id: 91, channel_id: "chat-42", role: "user", content: "exact",
        created_at: "2026-08-19T12:00:00Z",
      },
      job: { id: 73, instruction: "exact", pipeline: "chat", status: "pending" },
    });
    const fixture = createHost(pending.promise);
    vi.mocked(fixture.host.loadTranscript)
      .mockReturnValueOnce(acceptedTranscript.promise)
      .mockResolvedValueOnce(undefined);
    const coordinator = new ChatChannelTurnCoordinator(fixture.host);

    const receipt = await coordinator.accept("chat-42", "exact");

    expect(receipt).toMatchObject({ channelID: "chat-42", jobID: 73 });
    expect(fixture.host.loadTranscript).toHaveBeenCalledWith("chat-42", 91);
    expect(fixture.host.waitForJob).toHaveBeenCalledWith(73);
    let reconciled = false;
    const reconciliation = coordinator.reconcile(receipt).then(() => { reconciled = true; });
    await Promise.resolve();
    expect(reconciled).toBe(false);
    pending.resolve();
    await Promise.resolve();
    expect(fixture.host.loadTranscript).toHaveBeenCalledTimes(1);
    acceptedTranscript.resolve();
    await reconciliation;
    expect(fixture.host.loadTranscript).toHaveBeenLastCalledWith("chat-42");
    expect(fixture.host.refreshRoleplay).toHaveBeenCalledOnce();
  });

  it("keeps an accepted turn active when its immediate transcript refresh fails", async () => {
    const pending = deferred();
    vi.mocked(sendChannelMessage).mockResolvedValueOnce({
      channel: {
        id: "chat-42", scope: "user", name: "Chat", tags: ["user-channel"], project_id: 1,
        workspace_root: "/workspace", mode: "assistant",
        created_at: "2026-08-19T12:00:00Z", updated_at: "2026-08-19T12:00:00Z",
      },
      user_message: {
        id: 91, channel_id: "chat-42", role: "user", content: "exact",
        created_at: "2026-08-19T12:00:00Z",
      },
      job: { id: 73, instruction: "exact", pipeline: "chat", status: "pending" },
    });
    const fixture = createHost(pending.promise);
    vi.mocked(fixture.host.loadTranscript)
      .mockRejectedValueOnce(new Error("transcript database unavailable"))
      .mockResolvedValueOnce(undefined);
    const coordinator = new ChatChannelTurnCoordinator(fixture.host);

    const receipt = await coordinator.accept("chat-42", "exact");
    await receipt.acceptedTranscript;

    expect(fixture.host.waitForJob).toHaveBeenCalledWith(73);
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith(
      "Message accepted as job #73, but its transcript refresh failed: transcript database unavailable",
      "error",
    );
    expect(fixture.host.addEvent).toHaveBeenCalledWith("channel_accepted_transcript_failed", {
      channel_id: "chat-42",
      message_id: 91,
      job_id: 73,
      error: "transcript database unavailable",
    });
    pending.resolve();
    await coordinator.reconcile(receipt);
    expect(fixture.host.loadTranscript).toHaveBeenLastCalledWith("chat-42");
  });

  it("does not overwrite another selected conversation when a background turn completes", async () => {
    const pending = deferred();
    vi.mocked(sendChannelMessage).mockResolvedValueOnce({
      channel: {
        id: "chat-42", scope: "user", name: "Chat", tags: ["user-channel"], project_id: 1,
        workspace_root: "/workspace", mode: "assistant",
        created_at: "2026-08-19T12:00:00Z", updated_at: "2026-08-19T12:00:00Z",
      },
      user_message: {
        id: 91, channel_id: "chat-42", role: "user", content: "exact",
        created_at: "2026-08-19T12:00:00Z",
      },
      job: { id: 73, instruction: "exact", pipeline: "chat", status: "pending" },
    });
    const fixture = createHost(pending.promise);
    const coordinator = new ChatChannelTurnCoordinator(fixture.host);
    const receipt = await coordinator.accept("chat-42", "exact");
    fixture.setSelected(false);

    pending.resolve();
    await coordinator.reconcile(receipt);

    expect(fixture.host.loadTranscript).toHaveBeenCalledTimes(1);
    expect(fixture.host.refreshRoleplay).not.toHaveBeenCalled();
  });

  it("refreshes the terminal transcript before surfacing a failed job", async () => {
    const failure = new Error("response station failed loudly");
    vi.mocked(sendChannelMessage).mockResolvedValueOnce({
      channel: {
        id: "chat-42", scope: "user", name: "Chat", tags: ["user-channel"], project_id: 1,
        workspace_root: "/workspace", mode: "assistant",
        created_at: "2026-08-19T12:00:00Z", updated_at: "2026-08-19T12:00:00Z",
      },
      user_message: {
        id: 91, channel_id: "chat-42", role: "user", content: "exact",
        created_at: "2026-08-19T12:00:00Z",
      },
      job: { id: 73, instruction: "exact", pipeline: "chat", status: "pending" },
    });
    const fixture = createHost(Promise.reject(failure));
    const coordinator = new ChatChannelTurnCoordinator(fixture.host);
    const receipt = await coordinator.accept("chat-42", "exact");

    await expect(coordinator.reconcile(receipt)).rejects.toBe(failure);
    expect(fixture.host.loadTranscript).toHaveBeenLastCalledWith("chat-42");
    expect(fixture.host.refreshRoleplay).toHaveBeenCalledOnce();
  });
});
