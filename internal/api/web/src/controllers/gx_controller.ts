import { Controller } from "@hotwired/stimulus";
import { createRecyclrGX } from "../lib/recyclr";
import { cssEscape } from "../lib/dom";

export default class GxController extends Controller {
  gx: ReturnType<typeof createRecyclrGX> | null = null;

  connect(): void {
    if (this.gx) return;
    this.gx = createRecyclrGX();
    (window as Window & { omniRecyclr?: GxController }).omniRecyclr = this;
  }

  renderBundle(html: string): void {
    const doc = new DOMParser().parseFromString(String(html || ""), "text/html");
    const events = [...doc.querySelectorAll("[data-recyclr-target]")].map((node) => ({
      selector: `[data-recyclr-sink="${cssEscape((node as HTMLElement).dataset.recyclrTarget || "")}"]`,
      location: "innerHTML",
      selection: node.innerHTML,
    }));
    if (events.length > 0 && this.gx) {
      this.gx.render(events);
      this.element.dispatchEvent(new CustomEvent("omni:recycled", { detail: { events: events.length } }));
    }
  }
}
