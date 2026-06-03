import { Controller } from "@hotwired/stimulus";
import { createRecyclrGX, createRecyclrRealtimeStream } from "../lib/recyclr";
import { cssEscape } from "../lib/dom";
import { scheduleDomUpdate } from "../lib/main_thread";
import { showToast, type ToastTone } from "../lib/toast";

export default class GxController extends Controller {
  gx: ReturnType<typeof createRecyclrGX> | null = null;
  private stream: ReturnType<typeof createRecyclrRealtimeStream> | null = null;
  private metricsGlanceHandler: ((event: Event) => void) | null = null;
  private pendingBundleHTML: string | null = null;

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
      const html = String(message.html ?? "").trim();
      if (html) {
        this.queueRenderBundle(html);
      }
      queueMicrotask(() => this.dispatchRealtimeMessage(message));
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

  private queueRenderBundle(html: string): void {
    this.pendingBundleHTML = html;
    scheduleDomUpdate(() => {
      const pending = this.pendingBundleHTML;
      this.pendingBundleHTML = null;
      if (pending) this.renderBundleNow(pending);
    });
  }

  private dispatchRealtimeMessage(message: Record<string, unknown>): void {
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
    if (message.eventName === "chat-component-update") {
      document.dispatchEvent(new CustomEvent("omni:chat-component-update", { detail: message }));
    }
    if (message.eventName === "scrum-board-refresh") {
      const html = String(message.html ?? "").trim();
      document.dispatchEvent(
        new CustomEvent("omni:scrum-refresh", {
          detail: { project_id: message.projectID, skip_poll: html.length > 0 },
        }),
      );
    }
  }

  renderBundle(html: string): void {
    this.queueRenderBundle(html);
  }

  private renderBundleNow(html: string): void {
    const doc = new DOMParser().parseFromString(String(html || ""), "text/html");
    const events = [...doc.querySelectorAll("[data-recyclr-target]")].map((node) => {
      const target = (node as HTMLElement).dataset.recyclrTarget || "";
      let selection = node.innerHTML;
      if (!selection && node instanceof HTMLTemplateElement) {
        selection = [...node.content.childNodes].map((child) => {
          if (child instanceof Element) return child.outerHTML;
          return child.textContent ?? "";
        }).join("");
      }
      return {
        selector: `[data-recyclr-sink="${cssEscape(target)}"]`,
        location: (node as HTMLElement).dataset.recyclrLocation || "innerHTML",
        selection,
      };
    });
    if (events.length > 0 && this.gx) {
      this.gx.render(events);
      this.element.dispatchEvent(new CustomEvent("omni:recycled", { detail: { events: events.length } }));
      document.dispatchEvent(new CustomEvent("omni:recycled", { detail: { events } }));
      return;
    }
    for (const event of events) {
      const sink = document.querySelector(event.selector);
      if (!sink) continue;
      if (event.location === "beforeend") {
        sink.insertAdjacentHTML("beforeend", event.selection);
      } else if (event.location === "afterbegin") {
        sink.insertAdjacentHTML("afterbegin", event.selection);
      } else if (event.location === "outerHTML" && sink instanceof HTMLElement) {
        sink.outerHTML = event.selection;
      } else {
        sink.innerHTML = event.selection;
      }
    }
  }
}
