import { describe, expect, it, vi } from "vitest";
import ChatController from "./chat_controller";

function harness(mode: string, channelMode: "assistant" | "roleplay" = "assistant") {
  const select = document.createElement("select");
  select.innerHTML = '<option value="conversation">Conversation</option><option value="agents">Agents</option>';
  select.value = mode;
  const submit = vi.fn(async () => ({ jobID: 42 }));
  const channel = {
    hasSelection: () => true,
    selectedMode: () => channelMode,
    submit,
    select: vi.fn(async () => undefined),
    agentTurnState: vi.fn<() => { status: string; waitingFor?: "planning" | "execution" } | undefined>(() => undefined),
  };
  const controller = Object.assign(Object.create(ChatController.prototype), {
    hasCodingModeTarget: true,
    codingModeTarget: select,
    channel,
    busy: false,
    setBusy: vi.fn(),
    setTurnActive: vi.fn(),
    setStatus: vi.fn(),
    renderProgressActivity: vi.fn(),
    synchronizeSelectedSlashCommands: vi.fn(async () => undefined),
  }) as ChatController;
  return { controller, submit, channel };
}

describe("chat coding mode selection", () => {
  it("consumes the selected mode on each new job while keeping exact prompt bytes", async () => {
    const { controller, submit } = harness("agents");
    const prompt = "  precise task\n";
    await controller.submitChannel(prompt);
    expect(submit).toHaveBeenCalledWith(prompt, undefined, "agents");
    controller.codingModeTarget.value = "conversation";
    await controller.submitChannel(prompt);
    expect(submit).toHaveBeenLastCalledWith(prompt, undefined);
  });

  it("retains the draft and submits nothing when Agents is chosen for roleplay", async () => {
    const { controller, submit } = harness("agents", "roleplay");
    await expect(controller.submitChannel("exact draft")).rejects.toThrow("assistant conversation");
    expect(submit).not.toHaveBeenCalled();
  });

  it("keeps the coding mode fixed while allowing the planning conversation to continue", async () => {
    const { controller, channel } = harness("conversation");
    channel.agentTurnState.mockReturnValue({ status: "waiting_input", waitingFor: "planning" });

    await controller.selectChannel(new Event("change"));

    expect(controller.codingModeTarget.value).toBe("agents");
    expect(controller.codingModeTarget.disabled).toBe(true);
    expect(controller.setTurnActive).toHaveBeenLastCalledWith(false);
    expect(controller.setStatus).toHaveBeenLastCalledWith("Continue planning", "ready");
    channel.agentTurnState.mockReturnValue(undefined);
    await controller.selectChannel(new Event("change"));
    expect(controller.codingModeTarget.value).toBe("conversation");
    expect(controller.codingModeTarget.disabled).toBe(false);
  });
});
