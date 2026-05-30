import { escapeHTML } from "./dom";
import { renderChatMessages } from "./chat_render";
import type { ProjectPlanningCardDraft, ProjectPlanningChatConfig, ProjectPlanningSuggestion } from "./project_chat_api";
import type { ScrumChatMessage } from "./scrum_types";

function tabPanelClass(tab: string, activeTab: string): string {
  return tab === activeTab ? "" : " hidden";
}

function suggestionTone(level?: string): string {
  switch ((level || "info").toLowerCase()) {
    case "warn":
      return "border-amber-300/30 bg-amber-300/10 text-amber-100";
    case "tip":
      return "border-emerald-300/30 bg-emerald-300/10 text-emerald-100";
    default:
      return "border-cyan-300/25 bg-cyan-300/5 text-cyan-100";
  }
}

export function renderProjectPlanningSuggestions(suggestions: ProjectPlanningSuggestion[]): string {
  if (!suggestions.length) return "";
  return suggestions
    .map(
      (item) => `
    <div class="rounded-md border px-3 py-2 text-xs ${suggestionTone(item.level)}">${escapeHTML(item.text)}</div>
  `,
    )
    .join("");
}

export function renderProjectPlanningCardDrafts(drafts: ProjectPlanningCardDraft[]): string {
  if (!drafts.length) return "";
  return drafts
    .map(
      (draft, index) => `
    <article class="rounded-md border border-violet-300/25 bg-violet-300/5 p-3">
      <div class="flex flex-wrap items-start justify-between gap-2">
        <div class="min-w-0">
          <h4 class="text-sm font-semibold text-violet-100">${escapeHTML(draft.title)}</h4>
          ${draft.description ? `<p class="mt-1 text-xs leading-5 text-zinc-400">${escapeHTML(draft.description)}</p>` : ""}
          <p class="mt-2 text-[10px] uppercase tracking-wide text-zinc-500">${escapeHTML(draft.column || "backlog")}</p>
        </div>
        <button
          type="button"
          data-action="project-chat#createDraftCard"
          data-draft-index="${index}"
          class="shrink-0 rounded-md border border-violet-300/30 px-2.5 py-1 text-[11px] text-violet-100 hover:bg-violet-300/10"
        >Add card</button>
      </div>
      ${
        draft.checklist?.length
          ? `<ul class="mt-2 space-y-1">${draft.checklist.map((item) => `<li class="text-[11px] text-zinc-400">• ${escapeHTML(item)}</li>`).join("")}</ul>`
          : ""
      }
    </article>
  `,
    )
    .join("");
}

export function renderProjectPlanningChatMessages(
  messages: ScrumChatMessage[],
  options?: { pending?: boolean; pendingLabel?: string },
): string {
  if (!messages.length && !options?.pending) {
    return `<p class="px-4 py-8 text-center text-sm text-zinc-500">Discuss the project, scan the board, draft cards, or run research. This assistant plans — it does not build.</p>`;
  }
  return renderChatMessages(messages, options);
}

export function renderProjectChatShell(
  projectName: string,
  config: ProjectPlanningChatConfig,
  modelOptions: string[],
  activeTab = "scrum",
): string {
  const resolvedDefault = config.model || "";
  const reasoningMode = config.reasoning_mode || "instant";
  const modelSelect = [
    `<option value="">Auto (${reasoningMode === "thinking" ? "thinking" : "instant"})</option>`,
    ...modelOptions.map((name) => {
      const selected = resolvedDefault === name ? " selected" : "";
      return `<option value="${escapeHTML(name)}"${selected}>${escapeHTML(name)}</option>`;
    }),
  ].join("");

  return `
    <div data-project-tab-panel="chat" class="flex min-h-[520px] flex-col gap-3${tabPanelClass("chat", activeTab)}">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <h3 class="text-sm font-semibold text-zinc-100">Project chat</h3>
          <p class="mt-1 text-xs text-zinc-500">Planning assistant for <span class="text-zinc-300">${escapeHTML(projectName)}</span> — backlog, ideas, research, and card drafts. Not a build agent.</p>
        </div>
        <span data-project-chat-target="status" class="text-xs text-zinc-500">Ready</span>
      </div>

      <div class="flex flex-wrap items-center gap-2 rounded-xl border border-white/10 bg-zinc-950/60 p-3">
        <label class="flex min-w-[180px] flex-1 items-center gap-2 text-xs text-zinc-400">
          <span class="shrink-0 uppercase tracking-wide">Model</span>
          <select data-project-chat-target="modelSelect" data-action="change->project-chat#changeModel" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-2 py-1.5 font-mono text-[11px] text-zinc-100 outline-none focus:border-cyan-300/40">
            ${modelSelect}
          </select>
        </label>
        <div class="flex rounded-md border border-white/10 p-0.5 text-[11px]">
          <button type="button" data-action="project-chat#setReasoningMode" data-reasoning-mode="instant" class="rounded px-2.5 py-1.5 ${reasoningMode === "instant" ? "bg-cyan-300/15 text-cyan-100" : "text-zinc-400 hover:text-zinc-200"}">Instant</button>
          <button type="button" data-action="project-chat#setReasoningMode" data-reasoning-mode="thinking" class="rounded px-2.5 py-1.5 ${reasoningMode === "thinking" ? "bg-violet-300/15 text-violet-100" : "text-zinc-400 hover:text-zinc-200"}">Thinking</button>
        </div>
        <button type="button" data-action="project-chat#scanBoard" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Scan board</button>
        <button type="button" data-action="project-chat#runResearch" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Research</button>
      </div>

      <div class="grid min-h-0 flex-1 gap-3 lg:grid-cols-[minmax(0,1fr)_280px]">
        <section class="flex min-h-[420px] min-w-0 flex-col overflow-hidden rounded-xl border border-white/10 bg-zinc-950/60">
          <div data-project-chat-target="messages" class="scrollbar min-h-0 flex-1 overflow-y-auto p-3 md:p-4">
            ${renderProjectPlanningChatMessages([])}
          </div>
          <form data-action="submit->project-chat#sendMessage" class="border-t border-white/10 bg-zinc-950/70 p-3 backdrop-blur-xl">
            <div class="rounded-md border border-white/10 bg-zinc-900/90 p-2">
              <textarea
                data-project-chat-target="input"
                rows="2"
                placeholder="Talk about the project… /plan /research /scan /cards"
                class="scrollbar max-h-32 min-h-[3.25rem] w-full resize-none bg-transparent text-sm leading-5 text-zinc-100 outline-none placeholder:text-zinc-500"
              ></textarea>
              <div class="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-white/10 pt-2">
                <p class="text-[10px] text-zinc-500">Productivity AI · reads board, map, and memory</p>
                <button type="submit" class="rounded-md bg-cyan-300 px-4 py-1.5 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Send</button>
              </div>
            </div>
          </form>
        </section>

        <aside class="flex min-h-0 flex-col gap-3">
          <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-3">
            <h4 class="text-[11px] font-semibold uppercase tracking-[.16em] text-zinc-500">Suggestions</h4>
            <div data-project-chat-target="suggestions" class="scrollbar mt-2 max-h-40 space-y-2 overflow-y-auto">
              <p class="text-xs text-zinc-600">Tips from the planner appear here.</p>
            </div>
          </section>
          <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-3">
            <h4 class="text-[11px] font-semibold uppercase tracking-[.16em] text-zinc-500">Card drafts</h4>
            <div data-project-chat-target="drafts" class="scrollbar mt-2 max-h-64 space-y-2 overflow-y-auto">
              <p class="text-xs text-zinc-600">Draft cards suggested by the planner.</p>
            </div>
          </section>
        </aside>
      </div>
    </div>
  `;
}
