import { escapeHTML } from "./dom";

export type ChannelComposerOptions = {
  formAction: string;
  cardId?: string;
  placeholder?: string;
  inputTarget?: string;
  submitLabel?: string;
  disabled?: boolean;
  keydownAction?: string;
  inputName?: string;
  inputTargetAttr?: string;
  inputType?: "textarea" | "input";
};

export type ChannelSurfaceOptions = {
  eyebrow?: string;
  title: string;
  subtitle?: string;
  statusHtml?: string;
  actionsHtml?: string;
  badgeHtml?: string;
  messagesHtml: string;
  messagesAttrs?: string;
  messagesClass?: string;
  composerHtml?: string;
  shellClass?: string;
  headerClass?: string;
};

export function renderChannelComposer(options: ChannelComposerOptions): string {
  const cardAttr = options.cardId ? ` data-card-id="${escapeHTML(options.cardId)}"` : "";
  const inputTarget = options.inputTargetAttr
    ? ` ${options.inputTargetAttr}="${escapeHTML(options.inputTarget || "")}"`
    : options.inputTarget
      ? ` data-scrum-field="${escapeHTML(options.inputTarget)}"`
      : ' data-scrum-field="chatMessage"';
  const inputName = options.inputName ? ` name="${escapeHTML(options.inputName)}"` : "";
  const disabled = options.disabled ? " disabled" : "";
  const keydownAction = options.keydownAction ? ` keydown->${escapeHTML(options.keydownAction)}` : "";
  const placeholder = escapeHTML(options.placeholder || "Ask Omni to inspect, build, research, or explain...");
  const control =
    options.inputType === "input"
      ? `<input${inputTarget}${inputName}${disabled} placeholder="${placeholder}" class="min-w-[220px] flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500 disabled:opacity-60" />`
      : `<textarea${inputTarget}${inputName}${disabled} rows="2" placeholder="${placeholder}" class="scrollbar max-h-32 min-h-[3.25rem] w-full resize-none bg-transparent text-sm leading-5 text-zinc-100 outline-none placeholder:text-zinc-500 disabled:opacity-60"></textarea>`;
  const hint =
    options.inputType === "input"
      ? ""
      : `<div class="flex items-center gap-1.5 text-[10px] text-zinc-500">
          <span class="rounded border border-white/10 px-1.5 py-0.5">Enter newline</span>
          <span class="rounded border border-white/10 px-1.5 py-0.5">Cmd/Ctrl+Enter send</span>
        </div>`;
  const controlLayout = options.inputType === "input" ? "flex flex-wrap items-center gap-2" : "";
  return `
    <form data-action="${escapeHTML(options.formAction)}${keydownAction}"${cardAttr} class="border-t border-white/10 bg-zinc-950/70 p-3 backdrop-blur-xl md:px-4">
      <div class="rounded-md border border-white/10 bg-zinc-900/90 p-2">
        <div class="${controlLayout}">
          ${control}
          ${options.inputType === "input" ? renderComposerButton(options, disabled) : ""}
        </div>
        ${options.inputType === "input" ? "" : `
          <div class="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-2">
            ${hint}
            ${renderComposerButton(options, disabled)}
          </div>
        `}
      </div>
    </form>
  `;
}

function renderComposerButton(options: ChannelComposerOptions, disabled: string): string {
  return `
    <button type="submit"${disabled} class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200 disabled:cursor-not-allowed disabled:bg-zinc-700 disabled:text-zinc-400">
      ${escapeHTML(options.submitLabel || "Send")}
    </button>
  `;
}

export function renderChannelSurface(options: ChannelSurfaceOptions): string {
  const shellClass =
    options.shellClass || "flex min-h-[min(70vh,720px)] flex-col overflow-hidden rounded-lg border border-white/10 bg-zinc-950/50";
  const headerClass =
    options.headerClass || "flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-zinc-950/45 px-3 py-2 backdrop-blur-xl md:px-4";
  const messagesClass =
    options.messagesClass || "scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-3 py-3 md:px-4";
  const attrs = options.messagesAttrs ? ` ${options.messagesAttrs}` : "";
  return `
    <div class="${shellClass}">
      <header class="${headerClass}">
        <div class="min-w-0">
          ${options.eyebrow ? `<p class="text-[10px] uppercase tracking-[.18em] text-cyan-200/80">${escapeHTML(options.eyebrow)}</p>` : ""}
          <h3 class="truncate text-lg font-semibold tracking-tight text-zinc-100">${escapeHTML(options.title)}</h3>
          ${options.subtitle ? `<p class="text-xs text-zinc-500">${escapeHTML(options.subtitle)}</p>` : ""}
        </div>
        <div class="flex flex-wrap items-center gap-2">
          ${options.statusHtml || ""}
          ${options.actionsHtml || ""}
          ${options.badgeHtml || ""}
        </div>
      </header>
      <div${attrs} class="${messagesClass}">${options.messagesHtml}</div>
      ${options.composerHtml || ""}
    </div>
  `;
}
