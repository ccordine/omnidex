import { fetchSlashCommandComponent, type SlashCommandComponent } from "./chat_slash_palette_api";

const CANONICAL_CHANNEL_ID = /^[a-z0-9][a-z0-9_.:-]{0,95}$/;
const CANONICAL_COMMAND_PREFIX = /^\/[a-z][a-z0-9-]{0,31}$/;
const TYPED_COMMAND_PREFIX = /^\/[a-z0-9-]*$/;
const COMMAND_KINDS = new Set(["interaction", "give", "take", "research"]);

export interface ChatSlashPaletteHost {
  input(): HTMLTextAreaElement;
  palette(): HTMLElement;
  options(): HTMLElement;
  renderComponentBundle(bundle: string): Promise<void>;
}

export class ChatSlashPaletteCoordinator {
  private channelID = "";
  private generation = 0;
  private readyGeneration = -1;
  private activeIndex = -1;
  private renderGate: Promise<void> = Promise.resolve();

  constructor(private readonly host: ChatSlashPaletteHost) {}

  async activate(channelID: string): Promise<void> {
    const generation = ++this.generation;
    this.readyGeneration = -1;
    this.channelID = channelID;
    this.dismiss();
    if (!channelID) return;
    if (!CANONICAL_CHANNEL_ID.test(channelID)) {
      throw new Error("Slash-command activation requires a canonical channel id.");
    }
    const component = await fetchSlashCommandComponent(channelID);
    await this.applyComponent(component, channelID, generation);
  }

  async refresh(): Promise<void> {
    await this.activate(this.channelID);
  }

  inputChanged(): void {
    const prefix = typedPrefix(this.host.input());
    if (prefix === null || !this.channelID || !this.hasCurrentList()) {
      this.dismiss();
      return;
    }
    const visible = this.filterOptions(prefix);
    this.setNoMatchVisible(visible.length === 0);
    this.open();
    this.selectIndex(visible.length > 0 ? 0 : -1, visible);
  }

  keydown(event: KeyboardEvent): void {
    if (event.isComposing) return;
    if (!this.isOpen()) return;
    if (event.key === "Escape") {
      event.preventDefault();
      this.dismiss();
      return;
    }
    if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) return;
    const visible = this.visibleOptions();
    if (visible.length === 0) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const direction = event.key === "ArrowDown" ? 1 : -1;
      const next = this.activeIndex < 0
        ? (direction > 0 ? 0 : visible.length - 1)
        : (this.activeIndex + direction + visible.length) % visible.length;
      this.selectIndex(next, visible);
      return;
    }
    if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      this.insert(visible[this.activeIndex >= 0 ? this.activeIndex : 0]);
    }
  }

  choose(event: Event): void {
    const option = event.currentTarget;
    if (!(option instanceof HTMLButtonElement) || !this.isOpen() ||
      !this.allOptions().includes(option) || option.hidden) {
      throw new Error("Slash-command choice is not a visible server-rendered option.");
    }
    event.preventDefault();
    this.insert(option);
  }

  dismiss(): void {
    this.activeIndex = -1;
    for (const option of this.allOptions()) option.setAttribute("aria-selected", "false");
    this.host.palette().classList.add("hidden");
    const input = this.host.input();
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
  }

  private async applyComponent(
    component: SlashCommandComponent,
    channelID: string,
    generation: number,
  ): Promise<void> {
    const predecessor = this.renderGate;
    let release!: () => void;
    this.renderGate = new Promise<void>((resolve) => { release = resolve; });
    await predecessor;
    try {
      if (!this.isCurrent(channelID, generation)) return;
      await this.host.renderComponentBundle(component.html.bundle);
      if (!this.isCurrent(channelID, generation)) return;
      this.validateRenderedList(component);
      this.readyGeneration = generation;
      this.inputChanged();
    } finally {
      release();
    }
  }

  private validateRenderedList(component: SlashCommandComponent): void {
    const lists = [...this.host.options().querySelectorAll<HTMLElement>("[data-slash-command-list]")];
    if (lists.length !== 1 || lists[0].getAttribute("role") !== "listbox" ||
      lists[0].dataset.slashCommandChannelId !== component.channel_id) {
      throw new Error("Slash-command bundle lacks its exact channel-scoped listbox.");
    }
    const options = this.allOptions();
    if (options.length !== component.command_count) {
      throw new Error("Slash-command bundle count differs from its typed response.");
    }
    const ids = new Set<string>();
    for (const [expectedOrder, option] of options.entries()) {
      const prefix = option.dataset.slashCommandPrefix ?? "";
      const key = option.dataset.slashCommandKey ?? "";
      const order = canonicalInteger(option.dataset.slashCommandOrder);
      const cursor = canonicalInteger(option.dataset.slashCommandCursor);
      const command = option.dataset.slashCommand ?? "";
      if (option.getAttribute("role") !== "option" || option.dataset.action !== "chat#chooseSlashCommand" ||
        !option.id || ids.has(option.id) || order !== expectedOrder || !CANONICAL_COMMAND_PREFIX.test(prefix) ||
        prefix !== `/${key}` || !COMMAND_KINDS.has(option.dataset.slashCommandKind ?? "") || !command ||
        !isUTF16Boundary(command, cursor)) {
        throw new Error("Slash-command bundle contains an invalid server-rendered option.");
      }
      ids.add(option.id);
    }
    this.noMatch();
  }

  private filterOptions(prefix: string): HTMLButtonElement[] {
    const visible: HTMLButtonElement[] = [];
    for (const option of this.allOptions()) {
      const matches = (option.dataset.slashCommandPrefix ?? "").startsWith(prefix);
      option.hidden = !matches;
      option.setAttribute("aria-hidden", String(!matches));
      option.setAttribute("aria-selected", "false");
      if (matches) visible.push(option);
    }
    return visible;
  }

  private insert(option: HTMLButtonElement | undefined): void {
    if (!option) throw new Error("Slash-command selection has no active option.");
    const command = option.dataset.slashCommand ?? "";
    const cursor = canonicalInteger(option.dataset.slashCommandCursor);
    if (!command || !isUTF16Boundary(command, cursor)) throw new Error("Slash-command insertion is invalid.");
    const input = this.host.input();
    input.value = command;
    input.setSelectionRange(cursor, cursor);
    this.dismiss();
    input.focus();
  }

  private selectIndex(index: number, visible: HTMLButtonElement[]): void {
    this.activeIndex = index;
    visible.forEach((option, current) => option.setAttribute("aria-selected", String(current === index)));
    const active = index >= 0 ? visible[index] : undefined;
    if (!active) {
      this.host.input().removeAttribute("aria-activedescendant");
      return;
    }
    this.host.input().setAttribute("aria-activedescendant", active.id);
    if (typeof active.scrollIntoView === "function") active.scrollIntoView({ block: "nearest" });
  }

  private open(): void {
    this.host.palette().classList.remove("hidden");
    this.host.input().setAttribute("aria-expanded", "true");
  }

  private isOpen(): boolean {
    return !this.host.palette().classList.contains("hidden");
  }

  private hasCurrentList(): boolean {
    if (this.readyGeneration !== this.generation) return false;
    const list = this.host.options().querySelector<HTMLElement>("[data-slash-command-list]");
    return list?.dataset.slashCommandChannelId === this.channelID;
  }

  private allOptions(): HTMLButtonElement[] {
    return [...this.host.options().querySelectorAll<HTMLButtonElement>("button[data-chat-slash-option]")];
  }

  private noMatch(): HTMLElement {
    const matches = [...this.host.options().querySelectorAll<HTMLElement>("[data-slash-command-no-match]")];
    if (matches.length !== 1) throw new Error("Slash-command bundle lacks its server-rendered no-match row.");
    return matches[0];
  }

  private setNoMatchVisible(visible: boolean): void {
    const row = this.noMatch();
    row.hidden = !visible;
    row.setAttribute("aria-hidden", String(!visible));
  }

  private visibleOptions(): HTMLButtonElement[] {
    return this.allOptions().filter((option) => !option.hidden);
  }

  private isCurrent(channelID: string, generation: number): boolean {
    return this.channelID === channelID && this.generation === generation;
  }
}

function typedPrefix(input: HTMLTextAreaElement): string | null {
  if (input.selectionStart !== input.selectionEnd || input.selectionEnd !== input.value.length ||
    !TYPED_COMMAND_PREFIX.test(input.value)) return null;
  return input.value;
}

function canonicalInteger(value: string | undefined): number {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value)) {
    throw new Error("Slash-command option integer is not canonical.");
  }
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed)) throw new Error("Slash-command option integer exceeds its bound.");
  return parsed;
}

function isUTF16Boundary(value: string, index: number): boolean {
  if (index < 0 || index > value.length) return false;
  if (index === 0 || index === value.length) return true;
  const preceding = value.charCodeAt(index - 1);
  const following = value.charCodeAt(index);
  return !(preceding >= 0xd800 && preceding <= 0xdbff && following >= 0xdc00 && following <= 0xdfff);
}
