import { describe, expect, it, vi } from "vitest";
import ChatController from "./chat_controller";

type SubmitHarness = {
  hasInputTarget: boolean;
  inputTarget: { value: string };
  busy: boolean;
  activityLabel: string;
  setBusy: ReturnType<typeof vi.fn>;
  renderProgressActivity: ReturnType<typeof vi.fn>;
  submitChannel: ReturnType<typeof vi.fn>;
  activatePanel: ReturnType<typeof vi.fn>;
  addEvent: ReturnType<typeof vi.fn>;
  setStatus: ReturnType<typeof vi.fn>;
};

function harness(value: string): SubmitHarness {
  return {
    hasInputTarget: true,
    inputTarget: { value },
    busy: false,
	activityLabel: "",
    setBusy: vi.fn(),
    renderProgressActivity: vi.fn(),
    submitChannel: vi.fn(async () => undefined),
    activatePanel: vi.fn(async () => undefined),
    addEvent: vi.fn(),
    setStatus: vi.fn(),
  };
}

const submit = ChatController.prototype.submit as unknown as (
  this: SubmitHarness,
  event: { preventDefault(): void },
) => Promise<void>;

describe("ChatController exact authority", () => {
  it("passes the exact composer value only to the channel coordinator", async () => {
    const exact = "  preserve leading space\npreserve trailing tab\t ";
    const controller = harness(exact);

    await submit.call(controller, { preventDefault: vi.fn() });

	expect(controller.submitChannel).toHaveBeenCalledWith(exact);
    expect(controller.inputTarget.value).toBe("");
  });

  it("rejects blank composer input without dispatching", async () => {
    const controller = harness(" \n\t ");

    await submit.call(controller, { preventDefault: vi.fn() });

	expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe(" \n\t ");
  });

  it("does not invent or clear channel transcript state before server completion", async () => {
    let resolveSubmission!: () => void;
    const pending = new Promise<void>((resolve) => { resolveSubmission = resolve; });
	const controller = harness("exact channel prompt");
    controller.submitChannel.mockReturnValueOnce(pending);

    const result = submit.call(controller, { preventDefault: vi.fn() });
    await vi.waitFor(() => expect(controller.submitChannel).toHaveBeenCalledWith("exact channel prompt"));

    expect(controller.inputTarget.value).toBe("exact channel prompt");
    expect(controller.setBusy).toHaveBeenCalledWith(true);

    resolveSubmission();
    await result;
    expect(controller.inputTarget.value).toBe("");
  });

  it("preserves the channel composer and adds no synthetic transcript on failure", async () => {
	const controller = harness("retry this exact prompt");
    controller.submitChannel.mockRejectedValueOnce(new Error("channel job failed"));

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("retry this exact prompt");
    expect(controller.setStatus).toHaveBeenLastCalledWith("failed", "error");
    expect(controller.setBusy).toHaveBeenLastCalledWith(false);
  });
});
