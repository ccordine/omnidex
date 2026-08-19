import { describe, expect, it, vi } from "vitest";
import ChatController from "./chat_controller";

const reportSubmitFailure = Reflect.get(
  ChatController.prototype,
  "reportSubmitFailure",
) as (this: SubmitHarness, error: unknown) => void;

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
  reportSubmitFailure(error: unknown): void;
  roleplay: { isConfigured(): boolean };
  slashPalette: { dismiss(): void };
  channel: {
    selectedID(): string;
    hasSelection: ReturnType<typeof vi.fn>;
    createAndSubmit: ReturnType<typeof vi.fn>;
    select: ReturnType<typeof vi.fn>;
  };
  synchronizeSlashCommands: ReturnType<typeof vi.fn>;
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
    reportSubmitFailure,
    roleplay: { isConfigured: () => true },
    slashPalette: { dismiss: vi.fn() },
    channel: {
      selectedID: () => "story-42",
      hasSelection: vi.fn(() => true),
      createAndSubmit: vi.fn(async () => "submitted" as const),
      select: vi.fn(async () => undefined),
    },
    synchronizeSlashCommands: vi.fn(async () => undefined),
  };
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
  it("passes the exact composer value only to the channel coordinator", async () => {
    const exact = "  preserve leading space\npreserve trailing tab\t ";
    const controller = harness(exact);

    await submit.call(controller, { preventDefault: vi.fn() });

	expect(controller.submitChannel).toHaveBeenCalledWith(exact);
    expect(controller.inputTarget.value).toBe("");
    expect(controller.synchronizeSlashCommands).toHaveBeenCalledWith("story-42");
  });

  it("rejects blank composer input without dispatching", async () => {
    const controller = harness(" \n\t ");

    await submit.call(controller, { preventDefault: vi.fn() });

	expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe(" \n\t ");
  });

  it("creates one typed channel while neutral, then sends the exact composer bytes", async () => {
    const exact = "  neutral request\nwith exact spacing  ";
    const controller = harness(exact);
    controller.channel.hasSelection.mockReturnValue(false);

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.channel.createAndSubmit).toHaveBeenCalledWith(exact);
    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe("");
  });

  it("clears an accepted neutral prompt before a slow command refresh can re-expose it", async () => {
    let releaseSlash!: () => void;
    const slash = new Promise<void>((resolve) => { releaseSlash = resolve; });
    const controller = harness("do not submit twice");
    controller.channel.hasSelection.mockReturnValue(false);
    controller.channel.createAndSubmit.mockImplementationOnce(async () => {
      controller.busy = false;
      return "submitted";
    });
    controller.synchronizeSlashCommands.mockReturnValueOnce(slash);

    const submitting = submit.call(controller, { preventDefault: vi.fn() });
    await vi.waitFor(() => expect(controller.synchronizeSlashCommands).toHaveBeenCalledOnce());
    expect(controller.inputTarget.value).toBe("");

    releaseSlash();
    await submitting;
  });

  it("blocks composer submission through channel selection and slash synchronization", async () => {
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
    expect(controller.channel.createAndSubmit).not.toHaveBeenCalled();
    expect(controller.busy).toBe(true);

    releaseSelection();
    await vi.waitFor(() => expect(controller.synchronizeSlashCommands).toHaveBeenCalledOnce());
    expect(controller.busy).toBe(true);
    releaseSlash();
    await selecting;
    expect(controller.busy).toBe(false);
    expect(controller.inputTarget.value).toBe("do not race this prompt");
  });

  it("preserves the neutral composer when typed channel creation cannot complete", async () => {
    const exact = "creation failure exact";
    const controller = harness(exact);
    controller.channel.hasSelection.mockReturnValue(false);
    controller.channel.createAndSubmit.mockResolvedValueOnce("creation_failed");

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe(exact);
    expect(controller.setBusy).toHaveBeenLastCalledWith(false);
  });

  it("reports a neutral transition error without clearing the exact composer", async () => {
    const controller = harness("retry exact neutral prompt");
    controller.channel.hasSelection.mockReturnValue(false);
    controller.channel.createAndSubmit.mockRejectedValueOnce(new Error("transition invariant failed"));

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.inputTarget.value).toBe("retry exact neutral prompt");
    expect(controller.addEvent).toHaveBeenCalledWith("request_failed", {
      error: "transition invariant failed",
    });
    expect(controller.setStatus).toHaveBeenLastCalledWith("failed", "error");
    expect(controller.setBusy).toHaveBeenLastCalledWith(false);
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

  it("rejects a turn while the selected roleplay simulation is unconfigured", async () => {
    const controller = harness("do not accept this turn");
    controller.roleplay.isConfigured = () => false;

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe("do not accept this turn");
    expect(controller.setStatus).toHaveBeenCalledWith("roleplay setup required", "error");
  });

  it("creates a neutral roleplay channel but preserves the turn until scene setup", async () => {
    const exact = "preserve this roleplay turn";
    const controller = harness(exact);
    controller.channel.hasSelection.mockReturnValue(false);
    controller.channel.createAndSubmit.mockResolvedValueOnce("roleplay_setup_required");

    await submit.call(controller, { preventDefault: vi.fn() });

    expect(controller.channel.createAndSubmit).toHaveBeenCalledWith(exact);
    expect(controller.submitChannel).not.toHaveBeenCalled();
    expect(controller.inputTarget.value).toBe(exact);
    expect(controller.setStatus).toHaveBeenCalledWith("roleplay setup required", "error");
    expect(controller.setBusy).toHaveBeenLastCalledWith(false);
  });

  it("leaves exact composer bytes for the canonical submit path on modified Enter", () => {
    const exact = "/ta";
    const controller = { submit: vi.fn(), inputTarget: { value: exact } };
    const event = new KeyboardEvent("keydown", { key: "Enter", metaKey: true, cancelable: true });

    ChatController.prototype.composerKeydown.call(controller as never, event);

    expect(event.defaultPrevented).toBe(true);
    expect(controller.submit).toHaveBeenCalledWith(event);
    expect(controller.inputTarget.value).toBe(exact);
  });

  it("runs slash coordination before canonical modified-Enter submission without replacing bytes", () => {
    const inputTarget = { value: "/ta" };
    const submitted: string[] = [];
    const controller = {
      inputTarget,
      slashPalette: { keydown: vi.fn() },
      submit: vi.fn(() => { submitted.push(inputTarget.value); }),
    };
    const event = new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true, cancelable: true });

    ChatController.prototype.slashCommandKeydown.call(controller as never, event);
    ChatController.prototype.composerKeydown.call(controller as never, event);

    expect(controller.slashPalette.keydown).toHaveBeenCalledWith(event);
    expect(submitted).toEqual(["/ta"]);
    expect(controller.inputTarget.value).toBe("/ta");
  });
});
