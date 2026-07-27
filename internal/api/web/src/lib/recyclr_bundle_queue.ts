import { cssEscape } from "./dom";
import { scheduleDomUpdate } from "./main_thread";

export type RecyclrRenderEvent = {
  selector: string;
  location: "innerHTML" | "outerHTML" | "beforebegin" | "afterbegin" | "beforeend" | "afterend";
  selection: string;
};

type PendingBundle = {
  html: string;
  resolve: () => void;
  reject: (error: unknown) => void;
};

type ScheduleRender = (callback: () => void) => Promise<void>;

const validLocations = new Set<RecyclrRenderEvent["location"]>([
  "innerHTML",
  "outerHTML",
  "beforebegin",
  "afterbegin",
  "beforeend",
  "afterend",
]);

export function parseRecyclrBundle(html: string): RecyclrRenderEvent[] {
  const documentFragment = new DOMParser().parseFromString(html, "text/html");
  const targets = [...documentFragment.querySelectorAll<HTMLElement>("[data-recyclr-target]")];
  if (targets.length === 0) {
    throw new Error("Server bundle does not contain a Recyclr target.");
  }
  return targets.map((node) => {
    const target = node.dataset.recyclrTarget?.trim() ?? "";
    if (!target) throw new Error("Server bundle contains an empty Recyclr target.");
    const location = (node.dataset.recyclrLocation?.trim() || "innerHTML") as RecyclrRenderEvent["location"];
    if (!validLocations.has(location)) {
      throw new Error(`Server bundle contains unsupported Recyclr location ${JSON.stringify(location)}.`);
    }
    return {
      selector: `[data-recyclr-sink="${cssEscape(target)}"]`,
      location,
      selection: node.innerHTML,
    };
  });
}

/** Coalesces all server bundles arriving in one frame without discarding any bundle. */
export class RecyclrBundleQueue {
  private pending: PendingBundle[] = [];
  private scheduled = false;

  constructor(
    private readonly render: (events: RecyclrRenderEvent[]) => void,
    private readonly schedule: ScheduleRender = scheduleDomUpdate,
  ) {}

  enqueue(html: string): Promise<void> {
    if (!html.trim()) return Promise.reject(new Error("Cannot render an empty Recyclr bundle."));
    const result = new Promise<void>((resolve, reject) => {
      this.pending.push({ html, resolve, reject });
    });
    this.scheduleFlush();
    return result;
  }

  private scheduleFlush(): void {
    if (this.scheduled) return;
    this.scheduled = true;
    void this.schedule(() => this.flush()).catch((error) => this.rejectPending(error));
  }

  private flush(): void {
    this.scheduled = false;
    const batch = this.pending.splice(0);
    try {
      const events = batch.flatMap((bundle) => parseRecyclrBundle(bundle.html));
      this.render(events);
      batch.forEach((bundle) => bundle.resolve());
    } catch (error) {
      batch.forEach((bundle) => bundle.reject(error));
    }
    if (this.pending.length > 0) this.scheduleFlush();
  }

  private rejectPending(error: unknown): void {
    this.scheduled = false;
    const batch = this.pending.splice(0);
    batch.forEach((bundle) => bundle.reject(error));
  }
}
