import { describe, expect, it, vi } from "vitest";
import type { OmniPanel } from "../lib/panel_routing";
import { ChatRuntimeController } from "./chat_runtime_controller";
import { ChatViewController } from "./chat_view_controller";

type PanelRuntime = {
  loadPanelData(panel: OmniPanel): Promise<void>;
};

class TranscriptViewController extends ChatViewController {
  setup(): void {
    this.initializeViewState();
  }

  renderTranscript(bundle: string, preserveScroll: boolean): Promise<void> {
    return this.renderTranscriptBundle(bundle, preserveScroll);
  }

  setReplyTurn(active: boolean): void {
    this.setTurnActive(active);
  }

  setSubmitting(active: boolean): void {
    this.activityLabel = active ? "Sending…" : "";
    this.setBusy(active);
  }
}

describe("server-authoritative chat transcript", () => {
  it("shows the server-rendered typing indicator from submission through terminal turn state", () => {
    const controller = Object.create(TranscriptViewController.prototype) as TranscriptViewController;
    const messages = document.createElement("div");
    const typing = document.createElement("div");
    typing.classList.add("hidden");
    typing.setAttribute("aria-hidden", "true");
    Object.defineProperty(messages, "scrollHeight", { value: 240 });
    Object.defineProperties(controller, {
      hasLiveBadgeTarget: { value: false },
      hasInputTarget: { value: false },
      hasSendTarget: { value: false },
      hasSpinnerTarget: { value: false },
      hasTranscriptLoadingTarget: { value: false },
      hasMessagesTarget: { value: true },
      messagesTarget: { value: messages },
      hasTypingIndicatorTarget: { value: true },
      typingIndicatorTarget: { value: typing },
    });

    controller.setup();
    expect(typing.classList.contains("hidden")).toBe(true);

    controller.setSubmitting(true);
    expect(typing.classList.contains("hidden")).toBe(false);
    expect(typing.getAttribute("aria-hidden")).toBe("false");
    expect(messages.getAttribute("aria-busy")).toBe("true");

    controller.setReplyTurn(true);
    controller.setSubmitting(false);
    expect(typing.classList.contains("hidden")).toBe(false);
    expect(messages.getAttribute("aria-busy")).toBe("true");

    controller.setReplyTurn(false);
    expect(typing.classList.contains("hidden")).toBe(true);
    expect(typing.getAttribute("aria-hidden")).toBe("true");
    expect(messages.getAttribute("aria-busy")).toBe("false");
  });

  it("applies the exact server bundle through page Recyclr with visible loading feedback", async () => {
    const controller = Object.create(TranscriptViewController.prototype) as TranscriptViewController;
    const messages = document.createElement("div");
    const loading = document.createElement("div");
    loading.classList.add("hidden");
    messages.scrollTop = 20;
    let height = 100;
    Object.defineProperty(messages, "scrollHeight", { get: () => height });
    Object.defineProperties(controller, {
      hasMessagesTarget: { value: true },
      messagesTarget: { value: messages },
      hasTranscriptLoadingTarget: { value: true },
      transcriptLoadingTarget: { value: loading },
      hasTypingIndicatorTarget: { value: false },
    });
    let release!: () => void;
    const pending = new Promise<void>((resolve) => { release = resolve; });
    const renderBundle = vi.fn(async () => {
      height = 180;
      await pending;
    });
    controller.recyclrController = { renderBundle } as never;
    controller.setup();
    const exactBundle = '<template data-recyclr-target="channel-transcript-messages">server</template>';

    const rendering = controller.renderTranscript(exactBundle, true);

    expect(renderBundle).toHaveBeenCalledWith(exactBundle);
    expect(messages.getAttribute("aria-busy")).toBe("true");
    expect(loading.classList.contains("hidden")).toBe(false);
    release();
    await rendering;
    expect(messages.getAttribute("aria-busy")).toBe("false");
    expect(loading.classList.contains("hidden")).toBe(true);
    expect(messages.scrollTop).toBe(100);
  });

  it("reloads the chat surface whenever the chat panel is rendered", async () => {
    const loadChannels = vi.fn(async () => undefined);
    const controller = Object.create(ChatRuntimeController.prototype) as ChatRuntimeController;
    Object.assign(controller, {
      channel: { loadChannels },
      creation: { synchronize: vi.fn(), selectedMode: vi.fn(() => "assistant") },
      dataSources: { setCreationMode: vi.fn(), load: vi.fn(async () => undefined) },
      jobs: { load: vi.fn() },
      memory: { load: vi.fn() },
      system: { loadMetrics: vi.fn() },
      hasInputTarget: false,
    });

    await (controller as unknown as PanelRuntime).loadPanelData("chat");

    expect(loadChannels).toHaveBeenCalledOnce();
  });
});
