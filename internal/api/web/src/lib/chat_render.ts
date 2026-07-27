import { escapeHTML, formatTime } from "./dom";

export type ChatRenderMessage = {
  id?: string;
  role: string;
  content: string;
  at?: string;
  created_at?: string;
  status?: string;
  operation_id?: string;
};

export function chatMessageTimestamp(message: ChatRenderMessage): string {
  return message.at || message.created_at || new Date().toISOString();
}

export function renderChatMessage(message: ChatRenderMessage): string {
  const role = (message.role || "system").toLowerCase();
  const at = chatMessageTimestamp(message);
  const bodyClass =
    role === "thinking"
      ? "message-body text-sm italic text-zinc-400"
      : role === "tool"
        ? "message-body font-mono text-xs text-emerald-100/90"
        : role === "status"
          ? "message-body text-xs uppercase tracking-wide text-amber-100/90"
          : role === "error"
            ? "message-body text-sm text-rose-200"
            : "message-body text-zinc-100";
  const roleLabel =
    role === "tool" ? "tool" : role === "thinking" ? "thinking" : role === "status" ? "status" : role;
  return `
    <article class="message-grid message-${escapeHTML(role)}">
      <div class="message-shell">
        <div class="message-meta">
          <span>${escapeHTML(roleLabel)}</span>
          <time>${formatTime(at)}</time>
        </div>
        <div class="${bodyClass}">${escapeHTML(message.content)}</div>
      </div>
    </article>
  `;
}

export function renderPendingChatMessage(label = "Working…"): string {
  return `
    <article class="message-grid message-assistant message-pending" data-chat-working-message aria-live="polite">
      <div class="message-shell border border-cyan-300/20 bg-cyan-300/5">
        <div class="message-meta">
          <span>assistant</span>
          <time>${formatTime(new Date().toISOString())}</time>
        </div>
        <div class="message-body flex items-center gap-2 text-sm text-cyan-100">
          <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span>
          <span>${escapeHTML(label)}</span>
        </div>
      </div>
    </article>`;
}

function chatMessageDomAttrs(message: ChatRenderMessage): string {
  const id = (message.id || "").trim();
  if (!id) return "";
  return ` data-chat-message-id="${escapeHTML(id)}" data-recyclr-sink="chat-message-${escapeHTML(id)}"`;
}

export function renderChatMessages(messages: ChatRenderMessage[], options?: { pending?: boolean; pendingLabel?: string }): string {
  const html = messages.map((message) => renderChatMessage(message)).join("");
  if (!options?.pending) return html;
  return html + renderPendingChatMessage(options.pendingLabel);
}
