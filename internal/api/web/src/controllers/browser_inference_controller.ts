import { Controller } from "@hotwired/stimulus";
import { BrowserContextRelevanceRuntime } from "../lib/browser_context_relevance_runtime";

export default class BrowserInferenceController extends Controller<HTMLElement> {
  private runtime: BrowserContextRelevanceRuntime | null = null;

  connect(): void {
    if (this.element !== document.body) {
      throw new Error("Browser inference must be mounted once on <body>.");
    }
    if (this.runtime) throw new Error("Duplicate browser inference runtime attempted.");
    const runtime = new BrowserContextRelevanceRuntime();
    this.runtime = runtime;
    void runtime.start().catch((error) => {
      runtime.stop();
      console.error("Browser context relevance runtime failed", error);
      document.dispatchEvent(new CustomEvent("omni:browser-inference-status", {
        detail: {
          station: "context_relevance",
          state: "error",
          message: error instanceof Error ? error.message : String(error),
        },
      }));
    });
  }

  disconnect(): void {
    this.runtime?.stop();
    this.runtime = null;
  }
}
