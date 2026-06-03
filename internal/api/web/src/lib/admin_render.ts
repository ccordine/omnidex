import { escapeHTML, formatDateTime } from "./dom";
import type { MindStats, OllamaModelInfo, APISecretField, NetworkSettings } from "./admin_api";

const ADMIN_TABS = [
  { id: "overview", label: "Overview" },
  { id: "ai", label: "Models & agents" },
  { id: "datasources", label: "Data sources" },
  { id: "health", label: "Health" },
  { id: "advanced", label: "Advanced" },
] as const;

export type AdminTab = (typeof ADMIN_TABS)[number]["id"];

export function renderAdminTabNav(activeTab: AdminTab): string {
  return ADMIN_TABS.map((tab) => {
    const active = tab.id === activeTab;
    const classes = active
      ? "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
      : "border-white/10 text-zinc-400 hover:border-cyan-300/30 hover:text-zinc-200";
    return `<button type="button" data-action="admin#showTab" data-admin-tab="${tab.id}" class="rounded-md border px-3 py-2 text-sm font-medium transition ${classes}">${escapeHTML(tab.label)}</button>`;
  }).join("");
}

export function isAdminTab(value: string | null | undefined): value is AdminTab {
  return ADMIN_TABS.some((tab) => tab.id === value);
}

function adminSection(title: string, description: string, body: string): string {
  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/50 p-5">
      <div class="mb-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">${escapeHTML(title)}</h3>
        ${description ? `<p class="mt-1 text-xs leading-5 text-zinc-500">${escapeHTML(description)}</p>` : ""}
      </div>
      ${body}
    </section>
  `;
}

export function renderAdminSection(title: string, description: string, body: string): string {
  return adminSection(title, description, body);
}

export function renderAdminTabPanel(activeTab: AdminTab): string {
  switch (activeTab) {
    case "ai":
      return `
        <div data-admin-tab-panel="ai" class="mx-auto max-w-5xl space-y-4">
          ${adminSection("API keys", "Stored in the database. Leave a field blank to keep the current value. Environment variables are used only when no database value is set.", `<div data-admin-target="apiSecrets" class="mt-4">Loading...</div>`)}
          ${adminSection("Workspace agent defaults", "Global execution agent settings. Project and card overrides take precedence.", `<div data-admin-target="globalAgents" class="mt-4">Loading...</div>`)}
          ${adminSection(
            "Ollama models",
            "Pull, inspect, and remove local models used by the stack.",
            `<form data-action="submit->admin#pullModel" class="mt-4 flex flex-wrap gap-2">
              <input data-admin-target="pullModel" placeholder="llama3.2:latest" class="min-w-[220px] flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none" />
              <button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Pull model</button>
            </form>
            <div data-admin-target="ollamaModels" class="scrollbar mt-4 max-h-[420px] overflow-y-auto">Loading...</div>`,
          )}
          ${adminSection("Global model defaults", "Default Ollama models and generation settings for the workspace.", `<div data-admin-target="globalModels" class="mt-4">Loading...</div>`)}
        </div>
      `;
    case "datasources":
      return `<div data-admin-tab-panel="datasources" class="mx-auto max-w-6xl space-y-4"><div data-admin-target="dataSourcesPanel" class="space-y-4">Loading...</div></div>`;
    case "health":
      return `
        <div data-admin-tab-panel="health" class="mx-auto max-w-6xl space-y-4">
          ${adminSection("Core health", "Live /healthz payload from the running core service.", `<pre data-chat-target="statusOutput" data-recyclr-sink="status-output" class="scrollbar mt-4 max-h-[360px] overflow-y-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-zinc-900/60 p-4 text-sm leading-6 text-zinc-200">Loading...</pre>`)}
          <div class="grid gap-4 lg:grid-cols-2">
            ${adminSection("Research stack", "Ollama, embeddings, and web search readiness for research jobs.", `<div data-chat-target="researchStatusOutput" data-recyclr-sink="research-status-output" class="mt-4 text-sm text-zinc-400">Loading...</div>`)}
            ${adminSection("Host bridge", "In-app folder browser and terminal bridge. Run omni host service install or omni host serve.", `<div data-chat-target="hostBridgeStatusOutput" data-recyclr-sink="host-bridge-status-output" class="mt-4 text-sm text-zinc-400">Loading...</div>`)}
          </div>
        </div>
      `;
    case "advanced":
      return `
        <div data-admin-tab-panel="advanced" class="mx-auto max-w-5xl space-y-4">
          <section class="rounded-xl border border-rose-300/20 bg-rose-400/5 p-5">
            <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-rose-200">Destructive maintenance</h3>
            <p class="mt-1 text-xs text-rose-200/70">Reset the database schema when running with a repository. This cannot be undone.</p>
            <button data-action="chat#migrateFresh" type="button" class="mt-4 rounded-md border border-rose-300/30 bg-rose-400/10 px-4 py-2 text-sm font-semibold text-rose-100 transition hover:bg-rose-400/20">Migrate fresh</button>
          </section>
          <section class="rounded-xl border border-dashed border-zinc-700/80 bg-zinc-950/30 p-5">
            <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Debug tools</h3>
            <p class="mt-1 text-xs text-zinc-500">Low-level LLM wrappers for testing providers and prompts. Not part of the agent pipeline.</p>
            <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,320px)_minmax(0,1fr)]">
              <form data-action="submit->chat#runPersona" class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
                <h4 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-400">Persona lab</h4>
                <label class="mt-3 block text-xs text-zinc-500">Mode</label>
                <select data-chat-target="personaMode" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm outline-none">
                  <option value="instruct">instruct</option>
                  <option value="reasoning">reasoning</option>
                  <option value="roleplay">roleplay</option>
                  <option value="narrate">narrate</option>
                </select>
                <label class="mt-3 block text-xs text-zinc-500">Model override</label>
                <input data-chat-target="personaModel" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm outline-none" placeholder="optional" />
                <label class="mt-3 block text-xs text-zinc-500">System/context</label>
                <textarea data-chat-target="personaSystem" rows="3" class="scrollbar mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm outline-none" placeholder="Persona constraints or scene/system context"></textarea>
                <label class="mt-3 block text-xs text-zinc-500">Prompt</label>
                <textarea data-chat-target="personaPrompt" rows="4" class="scrollbar mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm outline-none" placeholder="Run a direct LLM tool call..."></textarea>
                <button type="submit" class="mt-3 w-full rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200">Run persona</button>
              </form>
              <pre data-chat-target="personaOutput" data-recyclr-sink="persona-output" class="scrollbar max-h-80 min-h-[12rem] overflow-y-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-zinc-950/50 p-3 text-xs leading-5 text-zinc-200">No run yet.</pre>
            </div>
          </section>
        </div>
      `;
    case "overview":
    default:
      return `
        <div data-admin-tab-panel="overview" class="mx-auto max-w-5xl space-y-4">
          ${adminSection("Network access", "LAN URL for phones, tablets, and other devices on your network.", `<div data-admin-target="networkAccess" class="mt-4">Loading...</div>`)}
          ${adminSection("Mind overview", "Counts for durable memory, candidates, jobs, and telemetry.", `<div data-admin-target="mindStats" class="mt-4">Loading...</div>`)}
          ${adminSection(
            "Document ingest",
            "Upload PDFs, DOCX, markdown, and text files. Default staging uses memory candidates so nothing enters durable memory until you approve it.",
            `<form data-action="submit->admin#uploadDocuments" class="mt-4 space-y-3">
              <input data-admin-target="ingestFiles" type="file" multiple accept=".pdf,.docx,.txt,.md,.markdown,.json,.yaml,.yml,.csv,.log,.srt,.vtt" class="block w-full text-sm text-zinc-300 file:mr-3 file:rounded-md file:border-0 file:bg-cyan-300 file:px-3 file:py-2 file:text-sm file:font-semibold file:text-zinc-950" />
              <div class="grid gap-3 md:grid-cols-2">
                <label class="block text-xs text-zinc-500">
                  Staging
                  <select data-admin-target="ingestStage" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
                    <option value="candidate" selected>Candidate review (recommended)</option>
                    <option value="durable">Durable memory (immediate)</option>
                  </select>
                </label>
                <label class="block text-xs text-zinc-500">
                  Extra tags
                  <input data-admin-target="ingestTags" placeholder="research,manual" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none" />
                </label>
              </div>
              <button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Upload and study</button>
            </form>`,
          )}
        </div>
      `;
  }
}

export function renderNetworkSettings(settings: NetworkSettings): string {
  const sourceLabel =
    settings.source === "database"
      ? "Saved in database"
      : settings.source === "environment"
        ? "From CORE_URL env"
        : "Default";
  const requestHint = settings.request_url
    ? `<p class="mt-2 text-xs text-zinc-500">This browser session: <span class="font-mono text-zinc-300">${escapeHTML(settings.request_url)}</span></p>`
    : "";
  return `
    <p class="text-sm text-zinc-400">Use this URL on iPad, phone, or other devices on your LAN — not localhost.</p>
    <div class="mt-3 rounded-md border border-cyan-300/20 bg-cyan-300/5 px-3 py-2">
      <a href="${escapeHTML(settings.core_url)}" target="_blank" rel="noopener noreferrer" class="font-mono text-sm text-cyan-200 hover:text-cyan-100">${escapeHTML(settings.core_url)}</a>
      <div class="mt-1 text-[11px] uppercase tracking-wide text-zinc-500">${escapeHTML(sourceLabel)} · listen ${escapeHTML(settings.listen_addr || "n/a")}</div>
    </div>
    ${requestHint}
    <form data-action="submit->admin#saveNetwork" class="mt-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_120px_auto]">
      <label class="block">
        <span class="text-xs text-zinc-500">Host / IP</span>
        <input data-admin-field="networkHost" value="${escapeHTML(settings.host)}" placeholder="192.168.1.102" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
      </label>
      <label class="block">
        <span class="text-xs text-zinc-500">Port</span>
        <input data-admin-field="networkPort" type="number" min="1" max="65535" value="${settings.port}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
      </label>
      <div class="self-end">
        <button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save URL</button>
      </div>
    </form>
  `;
}

export function renderMindStats(stats: MindStats | null): string {
  if (!stats) return `<p class="text-sm text-zinc-500">Mind stats unavailable.</p>`;
  const rows = [
    ["Memory chunks", stats.memory_chunks],
    ["Memory candidates", stats.memory_candidates],
    ["Pending review", stats.candidate_pending],
    ["Jobs", stats.jobs],
    ["Telemetry events", stats.telemetry_events],
  ];
  return `
    <div class="grid gap-2 sm:grid-cols-2">
      ${rows
        .map(
          ([label, value]) => `
        <div class="rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2">
          <div class="text-[11px] uppercase tracking-wide text-zinc-500">${escapeHTML(String(label))}</div>
          <div class="mt-1 font-mono text-lg text-cyan-200">${value}</div>
        </div>
      `,
        )
        .join("")}
    </div>
  `;
}

export function renderOllamaModels(endpoint: string, models: OllamaModelInfo[]): string {
  if (!models.length) {
    return `<p class="text-sm text-zinc-500">No models installed at ${escapeHTML(endpoint)}.</p>`;
  }
  return `
    <p class="mb-3 font-mono text-xs text-zinc-500">${escapeHTML(endpoint)}</p>
    <div class="space-y-2">
      ${models
        .map((model) => {
          const sizeGB = model.size > 0 ? `${(model.size / (1024 * 1024 * 1024)).toFixed(2)} GB` : "unknown size";
          return `
            <article class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2">
              <div class="min-w-0">
                <div class="font-mono text-sm text-zinc-100">${escapeHTML(model.name)}</div>
                <div class="text-[11px] text-zinc-500">${escapeHTML(sizeGB)} · ${escapeHTML(formatDateTime(model.modified_at))}</div>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                ${model.configured ? `<span class="rounded-full border border-cyan-300/30 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-cyan-200">In config</span>` : ""}
                <button type="button" data-action="admin#deleteOllamaModel" data-model-name="${escapeHTML(model.name)}" class="rounded-md border border-rose-300/30 px-2 py-1 text-xs text-rose-200 hover:bg-rose-400/10">Remove</button>
              </div>
            </article>
          `;
        })
        .join("")}
    </div>
  `;
}

export function renderGlobalModelSettings(
  fields: Array<{ key: string; label: string; description: string; value: string; options?: string[] }>,
  envFile: string,
): string {
  const rows = fields
    .map((field) => {
      const control = field.options?.length
        ? `<select data-admin-field="model_${escapeHTML(field.key)}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40">
            <option value="">Default</option>
            ${field.options.map((option) => `<option value="${escapeHTML(option)}"${field.value === option ? " selected" : ""}>${escapeHTML(option)}</option>`).join("")}
          </select>`
        : `<input data-admin-field="model_${escapeHTML(field.key)}" value="${escapeHTML(field.value)}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />`;
      return `
      <label class="block">
        <span class="text-xs text-zinc-500">${escapeHTML(field.label)}</span>
        ${control}
        <span class="mt-1 block text-[11px] text-zinc-600">${escapeHTML(field.description)}</span>
      </label>
    `;
    })
    .join("");
  return `
    <p class="mb-3 font-mono text-xs text-zinc-500">Env file: ${escapeHTML(envFile)}</p>
    <div class="grid gap-4 lg:grid-cols-2">${rows}</div>
    <button type="button" data-action="admin#saveGlobalModels" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save global model settings</button>
  `;
}

function secretSourceLabel(source: APISecretField["source"], hint: string): string {
  if (source === "database") return hint ? `Stored ${hint}` : "Stored";
  if (source === "environment") return hint ? `From env ${hint}` : "From environment";
  return "Not configured";
}

export function renderAPISecretsSettings(fields: APISecretField[]): string {
  const rows = fields
    .map((field) => {
      const status = secretSourceLabel(field.source, field.hint);
      const statusClass =
        field.source === "database"
          ? "border-cyan-300/30 bg-cyan-300/10 text-cyan-200"
          : field.source === "environment"
            ? "border-amber-300/30 bg-amber-300/10 text-amber-200"
            : "border-white/10 bg-zinc-900/60 text-zinc-500";
      return `
      <div class="rounded-md border border-white/10 bg-zinc-900/50 p-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-sm font-medium text-zinc-100">${escapeHTML(field.label)}</span>
          <span class="rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${statusClass}">${escapeHTML(status)}</span>
        </div>
        <input
          type="password"
          autocomplete="off"
          data-admin-field="secret_${escapeHTML(field.key)}"
          placeholder="Enter new key to save to database"
          class="mt-3 w-full rounded-md border border-white/10 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40"
        />
        <div class="mt-2 flex flex-wrap items-center justify-between gap-2">
          <span class="text-[11px] leading-5 text-zinc-600">${escapeHTML(field.description)}</span>
          ${
            field.source === "database"
              ? `<button type="button" data-action="admin#clearSecret" data-secret-key="${escapeHTML(field.key)}" class="rounded-md border border-rose-300/30 px-2 py-1 text-[11px] text-rose-200 hover:bg-rose-400/10">Clear stored</button>`
              : ""
          }
        </div>
      </div>
    `;
    })
    .join("");
  return `
    <div class="grid gap-4 lg:grid-cols-2">${rows}</div>
    <button type="button" data-action="admin#saveAPISecrets" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save API keys</button>
  `;
}
