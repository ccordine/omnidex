import { Controller } from "@hotwired/stimulus";
import { readJSON } from "../lib/api";
import { buildRecyclrBundle } from "../lib/recyclr";
import type GxController from "./gx_controller";

type ChatSnapshot = {
  html?: string;
  cursor?: string;
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
  private pollTimer: number | null = null;
  private recycledHandler = () => this.afterMessagesChanged();

  connect(): void {
    document.addEventListener("omni:recycled", this.recycledHandler);
    this.scrollToBottom(true);
    if (this.messagesElement().querySelector("[data-chat-component-working-message]")) {
      this.startPolling();
    }
  }

  disconnect(): void {
    document.removeEventListener("omni:recycled", this.recycledHandler);
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
  }

  private renderIntoMessages(html: string): void {
    const host = (window as Window & { omniRecyclr?: GxController }).omniRecyclr ?? null;
    const componentID = this.idValue || this.element.dataset.chatComponentId || "";
    const target = `chat-${componentID}-messages`;
    if (host?.renderBundle && componentID) {
      host.renderBundle(buildRecyclrBundle(target, html));
      return;
    }
    this.messagesElement().innerHTML = html;
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
    if (this.isPinnedToBottom()) this.scrollToBottom();
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
