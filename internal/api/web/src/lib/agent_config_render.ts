import { escapeHTML } from "./dom";
import type { AgentFieldDefinition } from "./agent_config_types";

const SOURCE_LABELS: Record<string, string> = {
  env: "Environment default",
  project: "Project override",
  card: "Card override",
};

const SYSTEM_LABELS: Record<string, string> = {
  omnidex: "Omnidex (local stack)",
  cursor: "Cursor SDK",
  codex: "Codex SDK",
};

export function renderAgentConfigSection(
  fields: AgentFieldDefinition[],
  overrides: Record<string, string>,
  resolvedSource: string,
  resolvedSystem: string,
  scope: "project" | "card",
  entityId: string,
): string {
  const scopeLabel = scope === "project" ? "Project" : "Card";
  const fieldPrefix = scope === "project" ? "projects" : "scrum";
  const sourceLabel = SOURCE_LABELS[resolvedSource] ?? resolvedSource;
  const systemLabel = SYSTEM_LABELS[resolvedSystem] ?? resolvedSystem;
  const rows = fields
    .map((field) => {
      const override = overrides[field.key] ?? "";
      const inherited = field.value;
      if (field.key === "agent_system") {
        const options = (field.options ?? ["omnidex", "cursor", "codex"])
          .map((option) => {
            const selected = (override || inherited) === option ? " selected" : "";
            return `<option value="${escapeHTML(option)}"${selected}>${escapeHTML(SYSTEM_LABELS[option] ?? option)}</option>`;
          })
          .join("");
        return `
          <label class="block">
            <span class="text-xs text-zinc-500">${escapeHTML(field.label)}</span>
            <select data-${fieldPrefix}-field="agent_${escapeHTML(field.key)}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">
              <option value="">Inherit (${escapeHTML(SYSTEM_LABELS[inherited] ?? inherited)})</option>
              ${options}
            </select>
            <span class="mt-1 block text-[11px] leading-5 text-zinc-600">${escapeHTML(field.description)}</span>
          </label>
        `;
      }
      if (field.key === "agent_strict") {
        const checked = override === "true" || (!override && inherited === "true") ? " checked" : "";
        return `
          <label class="flex items-start gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-3">
            <input type="checkbox" data-${fieldPrefix}-field="agent_${escapeHTML(field.key)}" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${checked} />
            <span>
              <span class="block text-sm text-zinc-200">${escapeHTML(field.label)}</span>
              <span class="mt-1 block text-[11px] leading-5 text-zinc-600">${escapeHTML(field.description)}</span>
            </span>
          </label>
        `;
      }
      return "";
    })
    .join("");

  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Execution agent</h3>
          <p class="mt-1 text-xs text-zinc-500">${scopeLabel} override for who runs work. Project context, files, and card details still apply.</p>
        </div>
        <div class="space-y-1 text-right">
          <span class="block rounded-full border border-white/10 bg-zinc-900/80 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">Effective: ${escapeHTML(sourceLabel)}</span>
          <span class="block font-mono text-[11px] text-cyan-200">${escapeHTML(systemLabel)}</span>
        </div>
      </div>
      <div class="mt-4 grid gap-4">${rows}</div>
      <div class="mt-4 flex flex-wrap gap-2">
        <button type="button" data-action="${fieldPrefix}#saveAgentConfig" data-${fieldPrefix === "projects" ? "project" : "card"}-id="${escapeHTML(entityId)}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save agent settings</button>
        <button type="button" data-action="${fieldPrefix}#clearAgentConfig" data-${fieldPrefix === "projects" ? "project" : "card"}-id="${escapeHTML(entityId)}" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10">Clear overrides</button>
      </div>
    </section>
  `;
}

export function collectAgentFieldValues(root: ParentNode, scope: "project" | "card"): Record<string, string> {
  const prefix = scope === "project" ? "projects" : "scrum";
  const out: Record<string, string> = {};
  for (const input of root.querySelectorAll(`[data-${prefix}-field^="agent_"]`)) {
    if (input instanceof HTMLInputElement && input.type === "checkbox") {
      const key = input.dataset[`${prefix}Field`]?.replace(/^agent_/, "") ?? "";
      if (key && input.checked) out[key] = "true";
      continue;
    }
    const element = input as HTMLSelectElement | HTMLInputElement;
    const key = element.dataset[`${prefix}Field`]?.replace(/^agent_/, "") ?? "";
    const value = element.value.trim();
    if (key && value) out[key] = value;
  }
  return out;
}

export function clearAgentFieldInputs(root: ParentNode, scope: "project" | "card"): void {
  const prefix = scope === "project" ? "projects" : "scrum";
  for (const input of root.querySelectorAll(`[data-${prefix}-field^="agent_"]`)) {
    if (input instanceof HTMLInputElement && input.type === "checkbox") {
      input.checked = false;
    } else {
      (input as HTMLSelectElement).value = "";
    }
  }
}

export function renderGlobalAgentSettings(fields: AgentFieldDefinition[], envFile: string): string {
  const rows = fields
    .map((field) => {
      if (field.key === "agent_system") {
        return `
          <label class="block">
            <span class="text-xs text-zinc-500">${field.label}</span>
            <select data-admin-field="agent_${field.key}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
              ${(field.options ?? []).map((option) => `<option value="${option}"${field.value === option ? " selected" : ""}>${SYSTEM_LABELS[option] ?? option}</option>`).join("")}
            </select>
          </label>
        `;
      }
      if (field.key === "agent_strict") {
        return `
          <label class="flex items-center gap-3">
            <input type="checkbox" data-admin-field="agent_${field.key}" class="rounded border-white/20 bg-zinc-900 text-cyan-300"${field.value === "true" ? " checked" : ""} />
            <span class="text-sm text-zinc-200">${field.label}</span>
          </label>
        `;
      }
      return "";
    })
    .join("");
  return `
    <p class="mb-3 font-mono text-xs text-zinc-500">Env file: ${escapeHTML(envFile)}</p>
    <div class="grid gap-4">${rows}</div>
    <button type="button" data-action="admin#saveGlobalAgents" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save global agent settings</button>
  `;
}
