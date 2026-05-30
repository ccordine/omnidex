import { escapeHTML, formatDateTime } from "./dom";
import type { DataSource } from "./admin_api";
import type { DataSourceChannel, DataSourceChannelMessage, DataSourceChannelPayload, QueryEvidence } from "./data_api";

export type DataPanelState = {
  sources: DataSource[];
  selectedSourceId: string | null;
  channels: DataSourceChannel[];
  selectedChannelId: string | null;
  messages: DataSourceChannelMessage[];
  pendingJobId: number | null;
  status: string;
};

export function emptyDataPanelState(): DataPanelState {
  return {
    sources: [],
    selectedSourceId: null,
    channels: [],
    selectedChannelId: null,
    messages: [],
    pendingJobId: null,
    status: "Open to load",
  };
}

function renderSourceList(state: DataPanelState): string {
  if (!state.sources.length) {
    return `<p class="px-3 py-4 text-xs text-zinc-500">No databases configured. Add connections in Admin → Data sources.</p>`;
  }
  return state.sources
    .map((source) => {
      const active = source.id === state.selectedSourceId;
      return `
        <button type="button" data-action="data#selectSource" data-source-id="${escapeHTML(source.id)}" class="block w-full border-b border-white/5 px-3 py-2 text-left ${active ? "bg-cyan-300/10" : "hover:bg-white/5"}">
          <div class="text-sm font-medium text-zinc-100">${escapeHTML(source.name)}</div>
          <div class="mt-0.5 font-mono text-[10px] text-zinc-500">${escapeHTML(source.use_dsn ? "DSN" : `${source.host}/${source.database_name}`)}</div>
        </button>
      `;
    })
    .join("");
}

function renderChannelList(state: DataPanelState): string {
  if (!state.selectedSourceId) {
    return `<p class="px-3 py-4 text-xs text-zinc-500">Select a database.</p>`;
  }
  const items = state.channels
    .map((channel) => {
      const active = channel.id === state.selectedChannelId;
      return `
        <button type="button" data-action="data#selectChannel" data-channel-id="${escapeHTML(channel.id)}" class="block w-full rounded-md border px-2 py-2 text-left ${active ? "border-cyan-300/40 bg-cyan-300/10" : "border-white/10 hover:border-cyan-300/20"}">
          <div class="truncate text-xs font-medium text-zinc-100">${escapeHTML(channel.name)}</div>
          <div class="mt-0.5 text-[10px] text-zinc-600">${escapeHTML(formatDateTime(channel.updated_at))}</div>
        </button>
      `;
    })
    .join("");
  return `
    <div class="space-y-2">${items || `<p class="text-xs text-zinc-500">No conversations yet.</p>`}</div>
    <button type="button" data-action="data#createChannel" class="mt-3 w-full rounded-md border border-dashed border-cyan-300/30 px-2 py-2 text-xs font-semibold text-cyan-100 hover:bg-cyan-300/10">+ New channel</button>
  `;
}

function renderMessageTable(rows: Array<Record<string, unknown>>, columns: string[]): string {
  if (!columns.length) return "";
  const head = columns.map((col) => `<th class="px-2 py-1 text-left font-mono text-[10px] uppercase text-zinc-500">${escapeHTML(col)}</th>`).join("");
  const body = rows
    .slice(0, 12)
    .map((row) => {
      const cells = columns
        .map((col) => `<td class="px-2 py-1 font-mono text-[11px] text-zinc-300">${escapeHTML(String(row[col] ?? ""))}</td>`)
        .join("");
      return `<tr class="border-t border-white/5">${cells}</tr>`;
    })
    .join("");
  return `
    <div class="scrollbar mt-2 max-h-40 overflow-auto rounded border border-white/10">
      <table class="min-w-full"><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table>
    </div>
  `;
}

function renderEvidenceBlock(query: DataSourceChannelPayload["query"], evidence?: QueryEvidence): string {
  const ev = evidence ?? query?.evidence;
  if (!ev || (!ev.steps?.length && !ev.citations?.length)) return "";
  const steps =
    ev.steps
      ?.map(
        (step) =>
          `<li class="text-[11px] text-zinc-400"><span class="font-mono text-cyan-200/90">q${step.step}</span>${step.purpose ? ` · ${escapeHTML(step.purpose)}` : ""} · ${step.row_count} rows</li>`,
      )
      .join("") ?? "";
  const citations =
    ev.citations
      ?.slice(0, 6)
      .map((fact) => `<li class="font-mono text-[10px] text-zinc-500">${escapeHTML(fact)}</li>`)
      .join("") ?? "";
  return `
    <details class="mt-2 rounded border border-emerald-300/20 bg-emerald-300/5 px-2 py-2">
      <summary class="cursor-pointer text-[10px] font-semibold uppercase tracking-[.18em] text-emerald-200/90">Evidence · ${escapeHTML(ev.confidence || "medium")} · ${ev.step_count} quer${ev.step_count === 1 ? "y" : "ies"}</summary>
      ${steps ? `<ul class="mt-2 space-y-1">${steps}</ul>` : ""}
      ${citations ? `<ul class="mt-2 space-y-0.5">${citations}</ul>` : ""}
    </details>
  `;
}

function renderMessage(message: DataSourceChannelMessage): string {
  const isUser = message.role === "user";
  const bubble = isUser ? "border-cyan-300/20 bg-cyan-300/10 text-cyan-50" : "border-white/10 bg-zinc-900/70 text-zinc-100";
  const align = isUser ? "justify-end" : "justify-start";
  const payload = message.payload;
  const query = payload?.query;
  const chartSpec = payload?.chart ? JSON.stringify(payload.chart) : "";
  const questionLine = !isUser && query?.question ? `<div class="text-[11px] text-zinc-500">Q: ${escapeHTML(query.question)}</div>` : "";
  const chartBlock =
    payload?.chart && payload.chart.series?.length
      ? `<div data-d3-chart data-chart-spec="${escapeHTML(chartSpec)}" class="mt-3 min-h-[240px] rounded-md border border-white/10 bg-zinc-950/80 p-2"></div>`
      : "";
  const sqlBlock = query?.sql
    ? `<pre class="scrollbar mt-2 max-h-28 overflow-auto rounded border border-white/10 bg-zinc-950/80 p-2 font-mono text-[10px] text-zinc-400">${escapeHTML(query.sql)}</pre>`
    : "";
  const tableBlock =
    query?.columns?.length && query.rows?.length ? renderMessageTable(query.rows, query.columns) : "";
  const evidenceBlock = !isUser ? renderEvidenceBlock(query, payload?.evidence) : "";
  return `
    <article class="flex ${align}">
      <div class="max-w-[92%] rounded-xl border ${bubble} px-3 py-2">
        ${questionLine}
        <div class="whitespace-pre-wrap text-sm leading-6">${escapeHTML(message.content || query?.answer || "")}</div>
        ${evidenceBlock}
        ${sqlBlock}
        ${tableBlock}
        ${chartBlock}
        <div class="mt-1 text-[10px] text-zinc-600">${escapeHTML(formatDateTime(message.created_at))}${message.job_id ? ` · job #${message.job_id}` : ""}</div>
      </div>
    </article>
  `;
}

function renderMessages(state: DataPanelState): string {
  if (!state.selectedChannelId) {
    return `<p class="text-sm text-zinc-500">Select or create a channel to start asking questions about this database.</p>`;
  }
  if (!state.messages.length && !state.pendingJobId) {
    return `<p class="text-sm text-zinc-500">Ask in plain language — answers are evidence-backed with SQL, data tables, and D3 charts.</p>`;
  }
  const pending =
    state.pendingJobId != null
      ? `<article class="flex justify-start"><div class="rounded-xl border border-white/10 bg-zinc-900/70 px-3 py-2 text-sm text-cyan-200">Running job #${state.pendingJobId}…</div></article>`
      : "";
  return `${state.messages.map(renderMessage).join("")}${pending}`;
}

export function renderDataPanel(state: DataPanelState): string {
  const selectedSource = state.sources.find((s) => s.id === state.selectedSourceId);
  const selectedChannel = state.channels.find((c) => c.id === state.selectedChannelId);
  return `
    <div class="grid h-full min-h-0 gap-0 lg:grid-cols-[220px_220px_minmax(0,1fr)]">
      <aside class="border-b border-white/10 lg:border-b-0 lg:border-r">
        <div class="border-b border-white/10 px-3 py-2 text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Databases</div>
        <div data-data-target="sourceList" class="scrollbar max-h-[220px] overflow-y-auto lg:max-h-none lg:min-h-[320px]">${renderSourceList(state)}</div>
      </aside>
      <aside class="border-b border-white/10 lg:border-b-0 lg:border-r">
        <div class="border-b border-white/10 px-3 py-2 text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Channels</div>
        <div data-data-target="channelList" class="scrollbar p-3 lg:min-h-[320px]">${renderChannelList(state)}</div>
      </aside>
      <section class="flex min-h-0 flex-col">
        <div class="border-b border-white/10 px-4 py-3">
          <h3 class="text-sm font-semibold text-zinc-100">${escapeHTML(selectedChannel?.name || selectedSource?.name || "Data chat")}</h3>
          <p class="text-xs text-zinc-500">${escapeHTML(state.status)}</p>
        </div>
        <div data-data-target="messageList" class="scrollbar flex-1 space-y-3 overflow-y-auto p-4">${renderMessages(state)}</div>
        <form data-action="submit->data#sendMessage" class="border-t border-white/10 p-4">
          <div class="flex flex-wrap gap-2">
            <input data-data-target="promptInput" placeholder="How many appointments are scheduled tomorrow?" class="min-w-[220px] flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" ${state.selectedChannelId ? "" : "disabled"} />
            <button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200" ${state.selectedChannelId ? "" : "disabled"}>Send</button>
          </div>
        </form>
      </section>
    </div>
  `;
}
