import { Controller } from "@hotwired/stimulus";
import { createRecyclrGX, createRecyclrRealtimeStream } from "../lib/recyclr";
import { cssEscape } from "../lib/dom";
import { showToast, type ToastTone } from "../lib/toast";

export default class GxController extends Controller {
  gx: ReturnType<typeof createRecyclrGX> | null = null;
  private stream: ReturnType<typeof createRecyclrRealtimeStream> | null = null;
  private metricsGlanceHandler: ((event: Event) => void) | null = null;

  connect(): void {
    if (this.gx) return;
    this.gx = createRecyclrGX();
    (window as Window & { omniRecyclr?: GxController }).omniRecyclr = this;
    this.startRealtimeStream();
  }

  disconnect(): void {
    this.stream?.stop();
    this.stream = null;
    if (this.metricsGlanceHandler) {
      document.removeEventListener("omni:metrics-glance", this.metricsGlanceHandler);
      this.metricsGlanceHandler = null;
    }
  }

  private startRealtimeStream(): void {
    if (!this.gx || this.stream) return;
    this.stream = createRecyclrRealtimeStream(this.gx, (message) => {
      const toast = String(message.toast ?? "").trim();
      if (toast) {
        const tone = String(message.toastTone ?? "info").trim() as ToastTone;
        showToast(toast, tone === "error" || tone === "ok" || tone === "busy" ? tone : "info");
      }
      if (message.eventName === "metrics-glance") {
        document.dispatchEvent(new CustomEvent("omni:metrics-glance", { detail: message }));
      }
      if (message.eventName === "scrum-card-modal-refresh") {
        document.dispatchEvent(new CustomEvent("omni:scrum-card-modal-refresh", { detail: message }));
      }
    });
    this.stream?.start();
  }

  /** Push a URL into browser history when GX history is enabled (same behavior as Recyclr fetch navigations). */
  pushRoute(url: string): void {
    if (!this.gx?.history) return;
    try {
      history.pushState(null, document.title, url);
    } catch {
      /* ignore invalid URLs in exotic environments */
    }
  }

  renderBundle(html: string): void {
    const doc = new DOMParser().parseFromString(String(html || ""), "text/html");
    const events = [...doc.querySelectorAll("[data-recyclr-target]")].map((node) => {
      const target = (node as HTMLElement).dataset.recyclrTarget || "";
      let selection = node.innerHTML;
      if (!selection && node instanceof HTMLTemplateElement) {
        selection = node.content?.innerHTML ?? "";
      }
      return {
        selector: `[data-recyclr-sink="${cssEscape(target)}"]`,
        location: "innerHTML",
        selection,
      };
    });
    if (events.length > 0 && this.gx) {
      this.gx.render(events);
      this.element.dispatchEvent(new CustomEvent("omni:recycled", { detail: { events: events.length } }));
      return;
    }
    for (const event of events) {
      const sink = document.querySelector(event.selector);
      if (sink) sink.innerHTML = event.selection;
    }
  }
}
