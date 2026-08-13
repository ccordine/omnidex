import { jsonRequest, readJSON } from "./api";
import { fetchChatMemoryPage } from "./chat_component_api";
import { toastError, toastFromError, toastOk } from "./feedback";

export interface ChatMemoryHost {
  queueEnabled(): boolean;
  hasMemoryList(): boolean;
  memoryKind(): HTMLSelectElement;
  memoryKindFilter(): HTMLSelectElement;
  memoryTags(): HTMLInputElement;
  memoryContent(): HTMLTextAreaElement;
  renderComponentBundle(bundle: string): Promise<void>;
  loadTimeline(options?: { quiet?: boolean; strict?: boolean }): Promise<void>;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
}

export class ChatMemoryCoordinator {
  constructor(private readonly host: ChatMemoryHost) {}

  async load(): Promise<void> {
    if (!this.host.queueEnabled()) throw new Error("Memory components require repository mode.");
    const page = await fetchChatMemoryPage("all", this.host.memoryKindFilter().value.trim());
    await this.host.renderComponentBundle(page.html.bundle);
    this.host.addEvent("memory_loaded", {
      memory_has_more: page.memory?.has_more ?? false,
      candidate_has_more: page.candidates?.has_more ?? false,
    });
  }

  async loadMore(event: Event): Promise<void> {
    const button = event.currentTarget as HTMLButtonElement;
    const section = button.dataset.pageSection;
    const offset = Number(button.dataset.nextOffset ?? "");
    if ((section !== "memory" && section !== "candidates") || !Number.isSafeInteger(offset) || offset < 1) {
      throw new Error("The server-rendered memory page cursor is invalid.");
    }
    await withButtonFeedback(button, "Loading memory…", async () => {
      const page = await fetchChatMemoryPage(section, this.host.memoryKindFilter().value.trim(), offset);
      await this.host.renderComponentBundle(page.html.bundle);
    });
  }

  async deleteMemory(event: Event): Promise<void> {
    event.preventDefault();
    const id = positiveDataID(event, "memoryId", "memory");
    if (!window.confirm(`Delete memory #${id}?`)) return;
    await readJSON(await fetch(`/v1/memory/${id}`, { method: "DELETE" }));
    await this.load();
    this.host.addEvent("memory_deleted", { id });
  }

  async deleteCandidate(event: Event): Promise<void> {
    event.preventDefault();
    const id = positiveDataID(event, "candidateId", "candidate");
    if (!window.confirm(`Delete candidate #${id}?`)) return;
    await readJSON(await fetch(`/v1/memory-candidates/${id}`, { method: "DELETE" }));
    await this.load();
    this.host.addEvent("memory_candidate_deleted", { id });
  }

  async loadGlobalActivity(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> {
    if (!this.host.queueEnabled()) return;
    try {
      await this.host.loadTimeline({ quiet: options.quiet, strict: true });
      if (this.host.hasMemoryList()) await this.load();
    } catch (error) {
      if (!options.quiet) this.host.addEvent("global_activity_failed", { error: errorMessage(error) });
      if (options.strict) throw error;
    }
  }

  async promote(event: Event): Promise<void> {
    const target = event.currentTarget as HTMLElement;
    const id = positiveDataID(event, "candidateId", "candidate");
    const tier = target.dataset.tier;
    const authority = target.dataset.authority;
    if (tier !== "approved" && tier !== "durable") throw new Error("Memory promotion tier is invalid.");
    if (!isPromotionAuthority(authority)) throw new Error("Memory promotion authority is invalid.");
    try {
      await readJSON(await fetch(`/v1/memory-candidates/${id}/promote`, jsonRequest({ tier, authority })));
      await this.load();
      this.host.addEvent("memory_promoted", { id, tier, authority });
      toastOk("Memory promoted");
    } catch (error) {
      toastFromError(error);
    }
  }

  async reject(event: Event): Promise<void> {
    const id = positiveDataID(event, "candidateId", "candidate");
    try {
      await readJSON(await fetch(`/v1/memory-candidates/${id}/reject`, jsonRequest({})));
      await this.load();
      this.host.addEvent("memory_rejected", { id });
      toastOk("Memory candidate rejected");
    } catch (error) {
      toastFromError(error);
    }
  }

  async add(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.host.queueEnabled()) {
      toastError("Memory requires repository mode");
      throw new Error("Memory requires repository mode.");
    }
    const content = this.host.memoryContent().value.trim();
    if (!content) {
      toastError("Memory content is required");
      return;
    }
    const tags = this.host.memoryTags().value.split(",").map((tag) => tag.trim()).filter(Boolean);
    try {
      await readJSON(await fetch("/v1/memory", jsonRequest({
        source: "omni-web-ui", kind: this.host.memoryKind().value, content, tags,
      })));
      this.host.memoryContent().value = "";
      this.host.memoryTags().value = "";
      await this.load();
      this.host.addEvent("memory_added", { kind: this.host.memoryKind().value, tags: tags.join(",") || "none" });
      toastOk("Memory saved");
    } catch (error) {
      toastFromError(error);
    }
  }
}

function positiveDataID(event: Event, key: "memoryId" | "candidateId", source: string): number {
  const raw = (event.currentTarget as HTMLElement).dataset[key];
  const id = Number(raw ?? "");
  if (!Number.isSafeInteger(id) || id < 1 || String(id) !== raw) {
    throw new Error(`The server-rendered ${source} identity is invalid.`);
  }
  return id;
}

function isPromotionAuthority(value: string | undefined): value is "current_generation" | "historical_generation" | "global" {
  return value === "current_generation" || value === "historical_generation" || value === "global";
}

async function withButtonFeedback(
  button: HTMLButtonElement,
  label: string,
  operation: () => Promise<void>,
): Promise<void> {
  const original = button.textContent;
  button.disabled = true;
  button.setAttribute("aria-busy", "true");
  button.textContent = label;
  try {
    await operation();
  } catch (error) {
    button.disabled = false;
    button.setAttribute("aria-busy", "false");
    button.textContent = original;
    throw error;
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
