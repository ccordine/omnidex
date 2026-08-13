import { describe, expect, it, vi } from "vitest";
import type { OmniPanel } from "../lib/panel_routing";
import { ChatRuntimeController } from "./chat_runtime_controller";
import { ChatViewController } from "./chat_view_controller";

type PanelRuntime = {
  loadPanelData(panel: OmniPanel): void;
};

class TranscriptViewController extends ChatViewController {
  setup(): void {
    this.initializeViewState();
  }

  renderTranscript(bundle: string, preserveScroll: boolean): Promise<void> {
    return this.renderTranscriptBundle(bundle, preserveScroll);
  }
}

describe("server-authoritative chat transcript", () => {
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

  it("reloads the selected server transcript whenever the chat panel is rendered", () => {
    const loadTranscript = vi.fn(async () => undefined);
    const controller = Object.create(ChatRuntimeController.prototype) as ChatRuntimeController;
    Object.assign(controller, {
      channel: { hasSelection: () => true, selectedID: () => "chat-42", loadTranscript },
      jobs: { load: vi.fn() },
      memory: { load: vi.fn() },
      system: { loadMetrics: vi.fn() },
      addEvent: vi.fn(),
    });

    (controller as unknown as PanelRuntime).loadPanelData("chat");

    expect(loadTranscript).toHaveBeenCalledWith("chat-42");
  });
});
