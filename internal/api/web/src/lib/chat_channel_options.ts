import { fetchChannelOptionsPage } from "./chat_component_api";

const MAX_AUTOMATIC_CHANNEL_PAGES = 100;
const CANONICAL_CHANNEL_ID = /^[a-z0-9][a-z0-9_.:-]{0,95}$/;

export interface ChatChannelOptionsHost {
  channelSelect(): HTMLSelectElement;
  renderComponentBundle(bundle: string): Promise<void>;
}

export class ChatChannelOptions {
  constructor(private readonly host: ChatChannelOptionsHost) {}

  isCanonicalID(id: string): boolean {
    return CANONICAL_CHANNEL_ID.test(id);
  }

  option(id: string): HTMLOptionElement | null {
    if (!id) return null;
    return [...this.host.channelSelect().options].find((option) => option.value === id) ?? null;
  }

  mode(id: string): "assistant" | "roleplay" {
    const option = this.option(id);
    if (!option || (option.dataset.channelMode !== "assistant" && option.dataset.channelMode !== "roleplay")) {
      throw new Error(`Selected channel ${JSON.stringify(id)} has no exact server-owned mode.`);
    }
    return option.dataset.channelMode;
  }

  async loadAll(mode?: "assistant" | "roleplay"): Promise<void> {
    let offset = 0;
    const bundles: string[] = [];
    for (let pageCount = 0; pageCount < MAX_AUTOMATIC_CHANNEL_PAGES; pageCount += 1) {
      const page = mode === undefined
        ? await fetchChannelOptionsPage(offset)
        : await fetchChannelOptionsPage(offset, 20, mode);
      bundles.push(page.html.bundle);
      if (page.next_offset === undefined) {
        for (const bundle of bundles) await this.host.renderComponentBundle(bundle);
        this.validate(mode);
        return;
      }
      if (page.next_offset <= offset) {
        throw new Error("The server channel page cursor did not advance.");
      }
      offset = page.next_offset;
    }
    throw new Error(`Channel pagination exceeded ${MAX_AUTOMATIC_CHANNEL_PAGES} server pages.`);
  }

  private validate(mode?: "assistant" | "roleplay"): void {
    const options = [...this.host.channelSelect().options];
    const neutral = options.filter((option) => option.value === "");
    const neutralLabel = mode === "roleplay" ? "Select a world" : "New conversation";
    if (neutral.length !== 1 || !neutral[0].disabled || neutral[0].textContent !== neutralLabel) {
      throw new Error("Channel options lack the exact neutral conversation control.");
    }
    for (const option of options) {
      if (option === neutral[0]) continue;
      if (!CANONICAL_CHANNEL_ID.test(option.value) || option.disabled) {
        throw new Error(`Channel options contain an invalid server conversation ${JSON.stringify(option.value)}.`);
      }
      if (mode !== undefined && option.dataset.channelMode !== mode) {
        throw new Error(`Channel options contain a conversation outside the ${mode} scope.`);
      }
    }
  }
}
