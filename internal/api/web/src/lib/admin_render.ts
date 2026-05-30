import { escapeHTML, formatDateTime } from "./dom";
import type { MindStats, OllamaModelInfo, APISecretField, NetworkSettings } from "./admin_api";

const ADMIN_TABS = [
  { id: "overview", label: "Overview" },
  { id: "ai", label: "Models & agents" },
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

export function adminTabPanelClass(tab: AdminTab, activeTab: AdminTab): string {
  return tab === activeTab ? "" : " hidden";
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
  fields: Array<{ key: string; label: string; description: string; value: string }>,
  envFile: string,
): string {
  const rows = fields
    .map(
      (field) => `
      <label class="block">
        <span class="text-xs text-zinc-500">${escapeHTML(field.label)}</span>
        <input data-admin-field="model_${escapeHTML(field.key)}" value="${escapeHTML(field.value)}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />
        <span class="mt-1 block text-[11px] text-zinc-600">${escapeHTML(field.description)}</span>
      </label>
    `,
    )
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
