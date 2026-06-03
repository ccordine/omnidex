import { Controller } from "@hotwired/stimulus";
import { readJSON } from "../lib/api";
import { closeModalShell, getModalElements, openModalShell } from "../lib/modal";

type ChatSnapshot = {
  html?: string;
  cursor?: string;
  before_cursor?: string;
  has_more?: boolean;
  busy?: boolean;
  card?: unknown;
};

type ChatSubmitResponse = ChatSnapshot & {
  operation_id?: string;
  action?: string;
  agent?: string;
  error?: string;
};

export default class ChatComponentController extends Controller<HTMLElement> {
  static targets = ["messages"];
  static values = {
    id: String,
    endpoint: String,
  };

  declare readonly hasMessagesTarget: boolean;
  declare readonly messagesTarget: HTMLElement;
  declare readonly idValue: string;
  declare readonly endpointValue: string;

  private cursor = "";
  private beforeCursor = "";
  private hasMore = true;
  private loadingOlder = false;
  private pollTimer: number | null = null;
  private recycledHandler = () => this.afterMessagesChanged();
  private scrollHandler = () => this.handleScroll();
  private detailClickHandler = (event: Event) => this.openMessageDetail(event);

  connect(): void {
    document.addEventListener("omni:recycled", this.recycledHandler);
    this.messagesElement().addEventListener("scroll", this.scrollHandler);
    this.messagesElement().addEventListener("click", this.detailClickHandler);
    this.restoreCachedMessages();
    this.scrollToBottom(true);
    if (this.messagesElement().querySelector("[data-chat-component-working-message]")) {
      this.startPolling();
    }
  }

  disconnect(): void {
    document.removeEventListener("omni:recycled", this.recycledHandler);
    this.messagesElement().removeEventListener("scroll", this.scrollHandler);
    this.messagesElement().removeEventListener("click", this.detailClickHandler);
    this.stopPolling();
  }

  composerKeydown(event: KeyboardEvent): void {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      const form = (event.currentTarget as HTMLElement | null)?.closest("form");
      if (form) void this.submitForm(form);
    }
  }

  send(event: Event): void {
    event.preventDefault();
    const form = (event.currentTarget as HTMLElement | null)?.closest("form") ?? (event.currentTarget as HTMLFormElement | null);
    if (form) void this.submitForm(form);
  }

  private async submitForm(form: HTMLFormElement): Promise<void> {
    const endpoint = this.endpointFrom(form);
    if (!endpoint) return;
    const input = form.querySelector<HTMLTextAreaElement | HTMLInputElement>('[data-scrum-field="chatMessage"], [name="message"]');
    const message = input?.value.trim() || "";
    if (!message) return;
    input!.value = "";
    this.appendTemporaryMessage("you", message);
    this.appendWorkingMessage("Sending...");
    this.scrollToBottom(true);
    this.setFormDisabled(form, true);
    try {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message, cursor: this.cursor }),
      });
      const payload = await readJSON<ChatSubmitResponse>(response);
      this.applySnapshot(payload);
      this.startPolling();
      document.dispatchEvent(new CustomEvent("omni:chat-component-submitted", { detail: payload }));
    } catch (error) {
      this.replaceWorkingMessage(error instanceof Error ? error.message : "Message failed");
    } finally {
      this.setFormDisabled(form, false);
      input?.focus();
    }
  }

  private async poll(): Promise<void> {
    const endpoint = this.endpointFrom();
    if (!endpoint) return;
    try {
      const url = new URL(endpoint, window.location.origin);
      if (this.cursor) url.searchParams.set("cursor", this.cursor);
      url.searchParams.set("limit", "5");
      const response = await fetch(url);
      const payload = await readJSON<ChatSnapshot>(response);
      this.applySnapshot(payload);
      if (payload.busy) {
        this.pollTimer = window.setTimeout(() => void this.poll(), 1500);
      } else {
        this.stopPolling();
      }
    } catch {
      this.pollTimer = window.setTimeout(() => void this.poll(), 3000);
    }
  }

  private startPolling(): void {
    if (this.pollTimer != null) return;
    this.pollTimer = window.setTimeout(() => void this.poll(), 1000);
  }

  private stopPolling(): void {
    if (this.pollTimer != null) window.clearTimeout(this.pollTimer);
    this.pollTimer = null;
  }

  private applySnapshot(payload: ChatSnapshot): void {
    const wasPinned = this.isPinnedToBottom();
    if (typeof payload.cursor === "string") this.cursor = payload.cursor;
    if (typeof payload.before_cursor === "string") this.beforeCursor = payload.before_cursor;
    if (typeof payload.has_more === "boolean") this.hasMore = payload.has_more;
    if (payload.html != null) {
      this.renderIntoMessages(payload.html);
    }
    if (payload.busy) {
      this.appendWorkingMessage("Agent working...");
      this.startPolling();
    } else {
      this.removeWorkingMessage();
    }
    if (wasPinned) this.scrollToBottom(true);
    this.cacheMessages();
  }

  private renderIntoMessages(html: string): void {
    this.mergeMessagesHTML(html, "append");
  }

  private mergeMessagesHTML(html: string, mode: "append" | "prepend"): void {
    const messages = this.messagesElement();
    const template = document.createElement("template");
    template.innerHTML = html;
    const incoming = Array.from(template.content.children) as HTMLElement[];
    const hasMessageIDs = incoming.some((node) => node.dataset.chatMessageId);
    if (!hasMessageIDs && messages.querySelectorAll("[data-chat-message-id]").length === 0) {
      messages.innerHTML = html;
      return;
    }
    messages.querySelectorAll("[data-chat-component-working-message], [data-scrum-channel-anchor]").forEach((node) => node.remove());
    const seen = new Set(Array.from(messages.querySelectorAll<HTMLElement>("[data-chat-message-id]")).map((node) => node.dataset.chatMessageId || ""));
    const fragment = document.createDocumentFragment();
    for (const node of incoming) {
      const id = node.dataset.chatMessageId || "";
      if (id && seen.has(id)) continue;
      if (id) seen.add(id);
      fragment.appendChild(node);
    }
    if (mode === "prepend") {
      messages.prepend(fragment);
    } else {
      messages.append(fragment);
    }
  }

  private async loadOlderMessages(): Promise<void> {
    if (this.loadingOlder || !this.hasMore) return;
    const endpoint = this.endpointFrom();
    const first = this.messagesElement().querySelector<HTMLElement>("[data-chat-message-id]");
    const before = first?.dataset.chatMessageId || this.beforeCursor;
    if (!endpoint || !before) return;
    this.loadingOlder = true;
    const messages = this.messagesElement();
    const previousHeight = messages.scrollHeight;
    try {
      const url = new URL(endpoint, window.location.origin);
      url.searchParams.set("before", before);
      url.searchParams.set("limit", "5");
      const payload = await readJSON<ChatSnapshot>(await fetch(url));
      if (typeof payload.before_cursor === "string") this.beforeCursor = payload.before_cursor;
      if (typeof payload.has_more === "boolean") this.hasMore = payload.has_more;
      if (payload.html) {
        this.mergeMessagesHTML(payload.html, "prepend");
        messages.scrollTop += messages.scrollHeight - previousHeight;
        this.cacheMessages();
      }
    } finally {
      this.loadingOlder = false;
    }
  }

  private handleScroll(): void {
    if (this.messagesElement().scrollTop <= 24) void this.loadOlderMessages();
  }

  private openMessageDetail(event: Event): void {
    const target = event.target as HTMLElement | null;
    if (!target) return;
    const card = target.closest<HTMLElement>("[data-chat-message-detail-card]");
    if (!card || !this.messagesElement().contains(card)) return;
    const explicit = target.closest("[data-chat-message-detail-open]");
    if (!explicit && target.closest("button,a,input,textarea,select")) return;
    const detail = card.querySelector<HTMLTemplateElement>("template[data-chat-message-detail]");
    if (!detail) return;
    event.preventDefault();
    const title = card.dataset.chatMessageDetailTitle || "Message details";
    const { panel } = getModalElements();
    if (!panel) return;
    panel.innerHTML = `
      <div class="flex max-h-[90vh] flex-col">
        <header class="flex shrink-0 items-center justify-between gap-3 border-b border-white/10 p-4 md:p-5">
          <div>
            <p class="text-xs uppercase tracking-[.18em] text-zinc-500">Channel message</p>
            <h2 class="mt-1 text-lg font-semibold text-zinc-100">${this.escape(title)}</h2>
          </div>
          <button type="button" data-chat-detail-close class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Close</button>
        </header>
        <div class="omni-modal-body scrollbar overflow-auto p-4 md:p-5">
          ${detail.innerHTML}
        </div>
      </div>`;
    panel.querySelector("[data-chat-detail-close]")?.addEventListener("click", () => closeModalShell(), { once: true });
    openModalShell({ wide: true });
  }

  private appendTemporaryMessage(role: string, content: string): void {
    const el = document.createElement("article");
    el.className = `message-grid message-${role}`;
    el.dataset.chatTemporary = "true";
    const label = role === "user" ? "you" : role;
    el.innerHTML = `<div class="message-shell"><div class="message-meta"><span>${this.escape(label)}</span><time>now</time></div><div class="message-body whitespace-pre-wrap text-sm leading-6 text-cyan-50">${this.escape(content)}</div></div>`;
    this.messagesElement().appendChild(el);
  }

  private appendWorkingMessage(label: string): void {
    const messages = this.messagesElement();
    if (messages.querySelector("[data-chat-component-working-message]")) return;
    const el = document.createElement("article");
    el.className = "message-grid message-assistant message-pending";
    el.dataset.chatComponentWorkingMessage = "true";
    el.setAttribute("aria-live", "polite");
    el.innerHTML = `<div class="message-shell border border-cyan-300/20 bg-cyan-300/5"><div class="message-meta"><span>assistant</span><time>now</time></div><div class="message-body flex items-center gap-2 text-sm text-cyan-100"><span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span><span>${this.escape(label)}</span></div></div>`;
    messages.appendChild(el);
  }

  private replaceWorkingMessage(label: string): void {
    const working = this.messagesElement().querySelector("[data-chat-component-working-message]");
    if (!working) return;
    working.outerHTML = `<article class="message-grid message-error"><div class="message-shell"><div class="message-meta"><span>error</span><time>now</time></div><div class="message-body whitespace-pre-wrap text-sm leading-6 text-rose-200">${this.escape(label)}</div></div></article>`;
  }

  private removeWorkingMessage(): void {
    this.messagesElement().querySelectorAll("[data-chat-component-working-message], [data-chat-temporary]").forEach((el) => el.remove());
  }

  private afterMessagesChanged(): void {
    this.restoreCachedMessages(true);
    if (this.isPinnedToBottom()) this.scrollToBottom();
    this.cacheMessages();
  }

  private scrollToBottom(force = false): void {
    const el = this.messagesElement();
    if (!force && !this.isPinnedToBottom()) return;
    requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight;
      requestAnimationFrame(() => {
        el.scrollTop = el.scrollHeight;
      });
    });
  }

  private isPinnedToBottom(): boolean {
    const el = this.messagesElement();
    return el.scrollHeight - el.clientHeight - el.scrollTop <= 80;
  }

  private messagesElement(): HTMLElement {
    return this.hasMessagesTarget ? this.messagesTarget : this.element;
  }

  private cacheKey(): string {
    const componentID = this.idValue || this.element.dataset.chatComponentId || "";
    return componentID ? `omni.chat-component.${componentID}.html.v1` : "";
  }

  private cacheMessages(): void {
    const key = this.cacheKey();
    if (!key) return;
    try {
      const copy = this.messagesElement().cloneNode(true) as HTMLElement;
      copy.querySelectorAll("[data-chat-component-working-message], [data-chat-temporary], [data-scrum-channel-anchor]").forEach((node) => node.remove());
      localStorage.setItem(key, copy.innerHTML);
    } catch {
      /* ignore storage quota/private mode */
    }
  }

  private restoreCachedMessages(mergeOnly = false): void {
    const key = this.cacheKey();
    if (!key) return;
    let html = "";
    try {
      html = localStorage.getItem(key) || "";
    } catch {
      return;
    }
    if (!html.trim()) return;
    if (!mergeOnly && this.messagesElement().querySelectorAll("[data-chat-message-id]").length === 0) {
      this.messagesElement().innerHTML = html;
      return;
    }
    this.mergeMessagesHTML(html, "prepend");
  }

  private endpointFrom(form?: HTMLFormElement): string {
    return form?.dataset.chatEndpoint || this.endpointValue || this.element.dataset.chatEndpoint || "";
  }

  private setFormDisabled(form: HTMLFormElement, disabled: boolean): void {
    form.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>("button,input,textarea,select").forEach((control) => {
      control.disabled = disabled;
    });
  }

  private escape(value: string): string {
    return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char] || char);
  }
}
