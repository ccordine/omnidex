import { describe, expect, it, vi } from "vitest";
import type { ChatChannelTurnReceipt } from "../lib/chat_channel_turn";
import ChatController from "./chat_controller";

type SubmitHarness = {
  hasInputTarget: boolean;
  inputTarget: HTMLTextAreaElement;
  busy: boolean;
  activityLabel: string;
  setBusy: ReturnType<typeof vi.fn>;
  setTurnActive: ReturnType<typeof vi.fn>;
  isTurnActive: ReturnType<typeof vi.fn>;
  focusComposer: ReturnType<typeof vi.fn>;
  renderProgressActivity: ReturnType<typeof vi.fn>;
  submitChannel: ReturnType<typeof vi.fn>;
  activatePanel: ReturnType<typeof vi.fn>;
  addEvent: ReturnType<typeof vi.fn>;
  setStatus: ReturnType<typeof vi.fn>;
  synchronizeSlashCommands: ReturnType<typeof vi.fn>;
  roleplay: { isConfigured: () => boolean };
  slashPalette: { dismiss: ReturnType<typeof vi.fn> };
  channel: {
    selectedID: () => string;
    selectedMode: () => "assistant" | "roleplay";
    hasSelection: ReturnType<typeof vi.fn>;
    createAndSubmit: ReturnType<typeof vi.fn>;
    reconcileTurn: ReturnType<typeof vi.fn>;
    select: ReturnType<typeof vi.fn>;
  };
};

function receipt(completion: Promise<void> = Promise.resolve()): ChatChannelTurnReceipt {
  return { channelID: "story-42", jobID: 73, acceptedTranscript: Promise.resolve(), completion };
}

function harness(value: string): SubmitHarness {
  const input = document.createElement("textarea");
  input.value = value;
  const turn = receipt();
  return Object.assign(Object.create(ChatController.prototype), {
    hasInputTarget: true,
    inputTarget: input,
    busy: false,
    activityLabel: "",
    setBusy: vi.fn(),
    setTurnActive: vi.fn(),
    isTurnActive: vi.fn(() => false),
    focusComposer: vi.fn(),
    renderProgressActivity: vi.fn(),
    submitChannel: vi.fn(async () => turn),
    activatePanel: vi.fn(async () => undefined),
    addEvent: vi.fn(),
    setStatus: vi.fn(),
    roleplay: { isConfigured: () => true },
    slashPalette: { dismiss: vi.fn() },
    channel: {
      selectedID: () => "story-42",
      selectedMode: () => "roleplay",
      hasSelection: vi.fn(() => true),
      createAndSubmit: vi.fn(async () => ({ kind: "submitted" as const, turn })),
      reconcileTurn: vi.fn(async (accepted: ChatChannelTurnReceipt) => accepted.completion),
      select: vi.fn(async () => undefined),
    },
    synchronizeSlashCommands: vi.fn(async () => undefined),
  }) as SubmitHarness;
}

const submit = ChatController.prototype.submit as unknown as (
  this: SubmitHarness,
  event: { preventDefault(): void },
) => Promise<void>;
const selectChannel = ChatController.prototype.selectChannel as unknown as (
  this: SubmitHarness,
  event: Event,
) => Promise<void>;

describe("ChatController exact authority", () => {
  it("passes exact composer bytes to the selected channel and clears after acceptance", async () => {
    const exact = "  preserve leading space\npreserve trailing tab\t ";
    const controller = harness(exact);

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.submitChannel).toHaveBeenCalledWith(exact);
    expect(controller.inputTarget.value).toBe("");
    expect(controller.setTurnActive).toHaveBeenCalledWith(true);
    expect(controller.setBusy).toHaveBeenLastCalledWith(false);
    expect(controller.channel.reconcileTurn).toHaveBeenCalledOnce();
  });

  it("submits the selected user character and ordered message part as immutable roleplay authority", async () => {
    const exact = "I follow Mara closely and watch the floorboards.";
    const controller = harness(exact) as SubmitHarness & {
      hasRoleplayPersonaTarget: boolean;
      roleplayPersonaTarget: HTMLSelectElement;
    };
    const persona = document.createElement("select");
    persona.innerHTML = `
      <option value="narrator" data-persona-kind="narrator">Narrator</option>
      <option value="rpc_0123456789abcdef0123456789abcdef" data-persona-kind="character" selected>Gryph</option>
    `;
    Object.assign(controller, {
      hasRoleplayPersonaTarget: true,
      roleplayPersonaTarget: persona,
    });

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.submitChannel).toHaveBeenCalledWith(`[Message]\n${exact}`, {
      persona_kind: "character",
      character_id: "rpc_0123456789abcdef0123456789abcdef",
      contribution_kind: "dialogue",
      parts: [{ kind: "message", text: exact }],
    });
  });

  it("keeps the composer recoverable until server acceptance and then clears it", async () => {
    let accept!: (value: ChatChannelTurnReceipt) => void;
    const pending = new Promise<ChatChannelTurnReceipt>((resolve) => { accept = resolve; });
    const controller = harness("clear this now");
    controller.submitChannel.mockReturnValueOnce(pending);

    const submission = submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("clear this now");
    expect(controller.submitChannel).toHaveBeenCalledWith("clear this now");
    accept(receipt());
    await submission;
    expect(controller.inputTarget.value).toBe("");
  });

  it("rejects blank composer input without dispatching", async () => {
    const controller = harness(" \n\t ");
    await submit.call(controller, { preventDefault: vi.fn() });
    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe(" \n\t ");
  });

  it("creates one typed channel from a neutral first send", async () => {
    const exact = "  neutral request\nwith exact spacing  ";
    const controller = harness(exact);
    controller.channel.hasSelection.mockReturnValue(false);

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.channel.createAndSubmit).toHaveBeenCalledWith(exact);
    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe("");
  });

  it("clears an accepted prompt while realtime completion remains pending", async () => {
    let finish!: () => void;
    const completion = new Promise<void>((resolve) => { finish = resolve; });
    const accepted = receipt(completion);
    const controller = harness("accepted immediately");
    controller.submitChannel.mockResolvedValueOnce(accepted);
    controller.channel.reconcileTurn.mockImplementationOnce(async () => completion);

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("");
    expect(controller.setTurnActive).toHaveBeenCalledWith(true);
    expect(controller.focusComposer).toHaveBeenCalled();
    expect(controller.setTurnActive).not.toHaveBeenCalledWith(false);
    finish();
    await vi.waitFor(() => expect(controller.setTurnActive).toHaveBeenCalledWith(false));
  });

  it("blocks a second send while the current authoritative turn is active", async () => {
    const controller = harness("draft next message");
    controller.isTurnActive.mockReturnValue(true);

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.channel.createAndSubmit).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe("draft next message");
    expect(controller.setStatus).toHaveBeenCalledWith(
      "Wait for the current reply before sending another message.",
      "active",
    );
  });

  it("blocks submission through channel selection and command synchronization", async () => {
    let releaseSelection!: () => void;
    let releaseSlash!: () => void;
    const selection = new Promise<void>((resolve) => { releaseSelection = resolve; });
    const slash = new Promise<void>((resolve) => { releaseSlash = resolve; });
    const controller = harness("do not race this prompt");
    controller.setBusy.mockImplementation((value: boolean) => { controller.busy = value; });
    controller.channel.select.mockReturnValueOnce(selection);
    controller.synchronizeSlashCommands.mockReturnValueOnce(slash);

    const selecting = selectChannel.call(controller, new Event("change"));
    await vi.waitFor(() => expect(controller.channel.select).toHaveBeenCalledOnce());
    await submit.call(controller, { preventDefault: vi.fn() });
    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.busy).toBe(true);
    releaseSelection();
    await vi.waitFor(() => expect(controller.synchronizeSlashCommands).toHaveBeenCalledOnce());
    releaseSlash();
    await selecting;
    expect(controller.busy).toBe(false);
  });

  it("preserves the neutral composer and foregrounds a channel creation failure", async () => {
    document.body.innerHTML = `<div id="omni-toast-root"><div id="omni-toast" hidden></div></div>`;
    const controller = harness("creation failure exact");
    controller.channel.hasSelection.mockReturnValue(false);
    controller.channel.createAndSubmit.mockRejectedValueOnce(new Error("channel database unavailable"));

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("creation failure exact");
    expect(controller.setBusy).toHaveBeenLastCalledWith(false);
    expect(controller.addEvent).toHaveBeenCalledWith("request_failed", {
      error: "channel database unavailable",
    });
    expect(controller.setStatus).toHaveBeenLastCalledWith("channel database unavailable", "error");
    expect(document.getElementById("omni-toast")).toHaveTextContent("channel database unavailable");
    document.body.replaceChildren();
  });

  it("reports a pre-acceptance failure without clearing exact composer bytes", async () => {
    document.body.innerHTML = `<div id="omni-toast-root"><div id="omni-toast" hidden></div></div>`;
    const controller = harness("retry exact prompt");
    controller.submitChannel.mockRejectedValueOnce(new Error("transition invariant failed"));

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("retry exact prompt");
    expect(controller.addEvent).toHaveBeenCalledWith("request_failed", {
      error: "transition invariant failed",
    });
    expect(controller.setStatus).toHaveBeenLastCalledWith("transition invariant failed", "error");
    expect(document.getElementById("omni-toast")).toHaveTextContent("transition invariant failed");
    document.body.replaceChildren();
  });

  it("surfaces a post-acceptance turn failure in the foreground", async () => {
    document.body.innerHTML = `<div id="omni-toast-root"><div id="omni-toast" hidden></div></div>`;
    const controller = harness("accepted before completion failure");
    controller.channel.reconcileTurn.mockRejectedValueOnce(new Error("response round failed"));

    await submit.call(controller, { preventDefault: vi.fn() });

    await vi.waitFor(() => expect(document.getElementById("omni-toast")).toHaveTextContent("response round failed"));
    expect(controller.addEvent).toHaveBeenCalledWith("channel_turn_failed", {
      channel_id: "story-42",
      job_id: 73,
      error: "response round failed",
    });
    expect(controller.setStatus).toHaveBeenCalledWith("response round failed", "error");
    document.body.replaceChildren();
  });

  it("preserves a roleplay prompt until its simulation is configured", async () => {
    const controller = harness("preserve roleplay prompt");
    controller.roleplay.isConfigured = () => false;

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe("preserve roleplay prompt");
    expect(controller.setStatus).toHaveBeenCalledWith("roleplay setup required", "error");
  });

  it("preserves a neutral roleplay prompt after its channel requires setup", async () => {
    const controller = harness("preserve this roleplay turn");
    controller.channel.hasSelection.mockReturnValue(false);
    controller.channel.createAndSubmit.mockResolvedValueOnce({ kind: "roleplay_setup_required" });

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("preserve this roleplay turn");
    expect(controller.setStatus).toHaveBeenCalledWith("roleplay setup required", "error");
  });

  it("uses Enter to send and Shift+Enter for a newline", () => {
    const controller = { submit: vi.fn() };
    const send = new KeyboardEvent("keydown", { key: "Enter", cancelable: true });
    const newline = new KeyboardEvent("keydown", { key: "Enter", shiftKey: true, cancelable: true });

    ChatController.prototype.composerKeydown.call(controller as never, send);
    ChatController.prototype.composerKeydown.call(controller as never, newline);

    expect(send.defaultPrevented).toBe(true);
    expect(newline.defaultPrevented).toBe(false);
    expect(controller.submit).toHaveBeenCalledTimes(1);
    expect(controller.submit).toHaveBeenCalledWith(send);
  });
});
