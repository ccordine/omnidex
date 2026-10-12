import { describe, expect, it, vi } from "vitest";
import ChatController from "./chat_controller";

function harness(mode: string, channelMode: "assistant" | "roleplay" = "assistant") {
  const select = document.createElement("select");
  select.innerHTML = '<option value="normal">Normal</option><option value="agents">Agents</option>';
  select.value = mode;
  const submit = vi.fn(async () => ({ jobID: 42 }));
  const channel = {
    hasSelection: () => true,
    selectedMode: () => channelMode,
    submit,
    select: vi.fn(async () => undefined),
    agentTurnState: vi.fn<() => { status: string; question?: string } | undefined>(() => undefined),
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
    controller.codingModeTarget.value = "normal";
    await controller.submitChannel(prompt);
    expect(submit).toHaveBeenLastCalledWith(prompt, undefined);
  });

  it("retains the draft and submits nothing when Agents is chosen for roleplay", async () => {
    const { controller, submit } = harness("agents", "roleplay");
    await expect(controller.submitChannel("exact draft")).rejects.toThrow("assistant conversation");
    expect(submit).not.toHaveBeenCalled();
  });

  it("shows the persisted mode and concrete question after selection, then restores the next-job preference", async () => {
    const { controller, channel } = harness("normal");
    const panel = document.createElement("div");
    panel.classList.add("hidden");
    const text = document.createElement("p");
    Object.assign(controller, { hasAgentQuestionsTarget: true, agentQuestionsTarget: panel, agentQuestionTextTarget: text });
    channel.agentTurnState.mockReturnValue({ status: "waiting_input", question: "Use <target> or keep the existing one?" });

    await controller.selectChannel(new Event("change"));

    expect(controller.codingModeTarget.value).toBe("agents");
    expect(controller.codingModeTarget.disabled).toBe(true);
    expect(panel.classList.contains("hidden")).toBe(false);
    expect(text.textContent).toBe("Use <target> or keep the existing one?");
    expect(text.querySelector("target")).toBeNull();
    channel.agentTurnState.mockReturnValue(undefined);
    await controller.selectChannel(new Event("change"));
    expect(controller.codingModeTarget.value).toBe("normal");
    expect(controller.codingModeTarget.disabled).toBe(false);
    expect(panel.classList.contains("hidden")).toBe(true);
  });
});
