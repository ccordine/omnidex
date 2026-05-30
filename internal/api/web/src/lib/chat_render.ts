import { escapeHTML, formatTime } from "./dom";
import { isScrumChannelNoiseContent } from "./channel_noise";
import { parseChannelActivity, renderChannelActivityMessage } from "./channel_activity_render";
import { renderChannelComposer, type ChannelComposerOptions } from "./channel_render";
import type { ScrumChatMessage } from "./scrum_types";

export type ChatRenderMessage = {
  role: string;
  content: string;
  at?: string;
  created_at?: string;
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
    <article class="message-grid message-assistant message-pending" aria-live="polite">
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

export function renderChatMessages(messages: ChatRenderMessage[], options?: { pending?: boolean; pendingLabel?: string }): string {
  const html = messages.map((message) => renderChatMessage(message)).join("");
  if (!options?.pending) return html;
  return html + renderPendingChatMessage(options.pendingLabel);
}

export function renderChannelChatMessage(message: ChatRenderMessage): string {
  const role = (message.role || "system").toLowerCase();
  const at = chatMessageTimestamp(message);
  const activity = role === "tool" ? parseChannelActivity(message.content || "") : null;
  if (activity) {
    return renderChannelActivityMessage(activity, at);
  }
  const content = escapeHTML(message.content || "");
  if (role === "system") {
    return `
      <article class="message-grid message-system">
        <div class="message-shell">
          <div class="message-body text-center text-[11px] leading-5 text-zinc-500">${content}</div>
        </div>
      </article>`;
  }
  if (role === "thinking") {
    return `
      <article class="message-grid message-thinking">
        <div class="message-shell">
          <div class="message-meta"><span>thinking</span><time>${formatTime(at)}</time></div>
          <div class="message-body whitespace-pre-wrap text-xs italic leading-5 text-zinc-500">${content}</div>
        </div>
      </article>`;
  }
  if (role === "user") {
    return `
      <article class="message-grid message-user">
        <div class="message-shell">
          <div class="message-meta"><span>you</span><time>${formatTime(at)}</time></div>
          <div class="message-body whitespace-pre-wrap text-sm leading-6 text-cyan-50">${content}</div>
        </div>
      </article>`;
  }
  if (role === "error") {
    return `
      <article class="message-grid message-error">
        <div class="message-shell">
          <div class="message-meta"><span>error</span><time>${formatTime(at)}</time></div>
          <div class="message-body whitespace-pre-wrap text-sm leading-6 text-rose-200">${content}</div>
        </div>
      </article>`;
  }
  return `
    <article class="message-grid message-assistant">
      <div class="message-shell">
        <div class="message-meta"><span>agent</span><time>${formatTime(at)}</time></div>
        <div class="message-body channel-agent-reply whitespace-pre-wrap text-[0.9375rem] leading-7 text-zinc-50">${content}</div>
      </div>
    </article>`;
}

/** Channel tab: newest at the visual bottom; DOM order matches flex-col-reverse (pending → newest → oldest). */
export function renderChannelChatMessages(messages: ChatRenderMessage[], options?: { pending?: boolean; pendingLabel?: string }): string {
  const reversed = [...messages].reverse().map((message) => renderChannelChatMessage(message)).join("");
  const pending = options?.pending ? renderPendingChatMessage(options.pendingLabel) : "";
  return `${pending}${reversed}<div data-scrum-channel-anchor class="h-px w-full shrink-0" aria-hidden="true"></div>`;
}

export function renderChatComposer(options: ChannelComposerOptions): string {
  return renderChannelComposer(options);
}

export function scrumMessagesToChat(messages: ScrumChatMessage[] = []): ChatRenderMessage[] {
  return [...messages]
    .sort((left, right) => chatMessageTimestamp(left).localeCompare(chatMessageTimestamp(right)))
    .filter((message) => {
      const role = (message.role || "").toLowerCase();
      const content = (message.content || "").trim();
      if (!content) return false;
      if (content.startsWith("[[context-sync:")) return false;
      if (content.startsWith("[[agent-stream-len:")) return false;
      if (role === "status") return false;
      if (isScrumChannelNoiseContent(role, content)) return false;
      if (role === "tool") {
        if (parseChannelActivity(content)) return true;
        return false;
      }
      if (role === "assistant") {
        const lower = content.toLowerCase();
        if (lower.includes("tool_call") || lower.includes("function_call")) return false;
        if (content.startsWith("{") && content.includes('"type"')) return false;
      }
      return true;
    })
    .map((message) => ({
      role: message.role,
      content: message.content,
      created_at: message.created_at,
    }));
}
