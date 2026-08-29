import { fetchChatDataSourceOptionsPage } from "./chat_component_api";
import type { StatusTone } from "./types";

const MAX_AUTOMATIC_DATA_CONNECTION_PAGES = 100;

export interface ChatDataSourceHost {
  hasSelect(): boolean;
  select(): HTMLSelectElement;
  renderComponentBundle(bundle: string): Promise<void>;
  setStatus(text: string, mode: StatusTone): void;
  addEvent(type: string, details?: Record<string, unknown>): void;
}

export class ChatDataSourceCoordinator {
	private loaded = false;
	private creationMode: "assistant" | "roleplay" = "assistant";

  constructor(private readonly host: ChatDataSourceHost) {}

  async load(): Promise<void> {
    if (!this.host.hasSelect()) return;
    try {
      await this.loadAllOptions();
      this.host.select().value = "";
		this.loaded = true;
		this.applyAvailability();
    } catch (error) {
		this.loaded = false;
		this.applyAvailability();
      this.host.setStatus("data connections unavailable", "error");
      this.host.addEvent("chat_data_sources_load_failed", { error: errorMessage(error) });
    }
  }

	setCreationMode(mode: "assistant" | "roleplay"): void {
		this.creationMode = mode;
		if (!this.host.hasSelect()) return;
		if (mode === "roleplay") this.host.select().value = "";
		this.applyAvailability();
	}

  selectedForCreation(): string | undefined {
    if (!this.host.hasSelect()) return undefined;
    const select = this.host.select();
    const selected = select.value;
    if (!selected) return undefined;
    if (select.disabled) throw new Error("The data selector is unavailable.");
    if (!/^[a-z0-9][a-z0-9_.:-]{0,127}$/.test(selected)) {
      throw new Error("The selected data connection has an invalid canonical identity.");
    }
    const serverOption = [...select.options].find((option) => option.value === selected);
    if (!serverOption) {
      throw new Error("The selected data connection is absent from server-rendered options.");
    }
    return selected;
  }

  private async loadAllOptions(): Promise<void> {
    let offset = 0;
    for (let pageCount = 0; pageCount < MAX_AUTOMATIC_DATA_CONNECTION_PAGES; pageCount += 1) {
      const page = await fetchChatDataSourceOptionsPage(offset);
      await this.host.renderComponentBundle(page.html.bundle);
      if (page.next_offset === undefined) return;
      if (page.next_offset <= offset) {
        throw new Error("The server data-connection page cursor did not advance.");
      }
      offset = page.next_offset;
    }
    throw new Error(`Data connection pagination exceeded ${MAX_AUTOMATIC_DATA_CONNECTION_PAGES} server pages.`);
  }

	private applyAvailability(): void {
		if (!this.host.hasSelect()) return;
		this.host.select().disabled = !this.loaded || this.creationMode === "roleplay";
	}
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
