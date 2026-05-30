import { escapeHTML } from "./dom";
import type { AgentFieldDefinition } from "./agent_config_types";

const SOURCE_LABELS: Record<string, string> = {
  instance: "This run",
  card: "Card override",
  project: "Project override",
  workspace: "Workspace default",
  env: "Environment fallback",
};

const SYSTEM_LABELS: Record<string, string> = {
  omnidex: "Omnidex (local stack)",
  cursor: "Cursor SDK",
  codex: "Codex SDK",
};

const PRE_ALPHA_AGENTS = new Set(["omnidex"]);

/** Colorful pre-alpha flag for Omnidex agent options. */
export function renderPreAlphaBadge(): string {
  return `<span class="inline-flex shrink-0 items-center gap-1 rounded-full border border-fuchsia-400/45 bg-gradient-to-r from-fuchsia-500/25 via-amber-400/20 to-orange-400/25 px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em] text-fuchsia-100 shadow-[0_0_12px_rgba(232,121,249,.25)]" title="Omnidex is in pre-alpha"><span aria-hidden="true" class="text-[11px] leading-none">🚩</span>Pre-alpha</span>`;
}

function renderAgentSystemPicker(
  field: AgentFieldDefinition,
  override: string,
  inherited: string,
  fieldPrefix: "projects" | "scrum",
): string {
  const options = field.options ?? ["omnidex", "cursor", "codex"];
  const selected = override || "";
  const inheritLabel = SYSTEM_LABELS[inherited] ?? inherited;
  const fieldAttr = `data-${fieldPrefix}-field="agent_${escapeHTML(field.key)}"`;

  const row = (value: string, labelHTML: string, badge = "") => {
    const active = selected === value;
    return `
      <label class="flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 transition ${active ? "border-cyan-300/40 bg-cyan-300/10" : "border-white/10 bg-zinc-900/50 hover:border-white/20"}">
        <input type="radio" ${fieldAttr} name="agent_${fieldPrefix}_${escapeHTML(field.key)}" value="${escapeHTML(value)}" class="mt-0.5 border-white/20 bg-zinc-900 text-cyan-300 focus:ring-cyan-300/40"${active ? " checked" : ""} />
        <span class="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-sm text-zinc-200">${labelHTML}${badge}</span>
      </label>
    `;
  };

  const inheritBadge = PRE_ALPHA_AGENTS.has(inherited) ? renderPreAlphaBadge() : "";
  const rows = [
    row("", `Inherit (<span class="text-zinc-400">${escapeHTML(inheritLabel)}</span>)`, inheritBadge),
    ...options.map((option) =>
      row(
        option,
        escapeHTML(SYSTEM_LABELS[option] ?? option),
        PRE_ALPHA_AGENTS.has(option) ? renderPreAlphaBadge() : "",
      ),
    ),
  ].join("");

  return `
    <fieldset class="block">
      <legend class="text-xs text-zinc-500">${escapeHTML(field.label)}</legend>
      <div class="mt-2 grid gap-2">${rows}</div>
      <span class="mt-2 block text-[11px] leading-5 text-zinc-600">${escapeHTML(field.description)}</span>
    </fieldset>
  `;
}

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
  const systemBadge = PRE_ALPHA_AGENTS.has(resolvedSystem) ? ` ${renderPreAlphaBadge()}` : "";
  const rows = fields
    .map((field) => {
      const override = overrides[field.key] ?? "";
      const inherited = field.value;
      if (field.key === "agent_system") {
        return renderAgentSystemPicker(field, override, inherited, fieldPrefix);
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
          <p class="mt-1 text-xs text-zinc-500">${scopeLabel} override for who runs work. Priority: this run → card → project → workspace → environment.</p>
        </div>
        <div class="space-y-1 text-right">
          <span class="block rounded-full border border-white/10 bg-zinc-900/80 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">Effective: ${escapeHTML(sourceLabel)}</span>
          <span class="flex flex-wrap items-center justify-end gap-2 font-mono text-[11px] text-cyan-200">${escapeHTML(systemLabel)}${systemBadge}</span>
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
    if (input instanceof HTMLInputElement && input.type === "radio") {
      const key = input.dataset[`${prefix}Field`]?.replace(/^agent_/, "") ?? "";
      if (key && input.checked) {
        const value = input.value.trim();
        if (value) out[key] = value;
      }
      continue;
    }
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
    if (input instanceof HTMLInputElement && input.type === "radio") {
      const key = input.dataset[`${prefix}Field`]?.replace(/^agent_/, "") ?? "";
      if (key === "agent_system") {
        input.checked = input.value === "";
      }
      continue;
    }
    if (input instanceof HTMLInputElement && input.type === "checkbox") {
      input.checked = false;
    } else {
      (input as HTMLSelectElement).value = "";
    }
  }
}

export function renderGlobalAgentSettings(fields: AgentFieldDefinition[]): string {
  const rows = fields
    .map((field) => {
      if (field.key === "agent_system") {
        const options = field.options ?? ["omnidex", "cursor", "codex"];
        const selected = field.value ?? "";
        const optionRows = options
          .map((option) => {
            const active = selected === option;
            return `
              <label class="flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 transition ${active ? "border-cyan-300/40 bg-cyan-300/10" : "border-white/10 bg-zinc-900/50 hover:border-white/20"}">
                <input type="radio" data-admin-field="agent_${field.key}" name="admin_agent_system" value="${escapeHTML(option)}" class="mt-0.5 border-white/20 bg-zinc-900 text-cyan-300 focus:ring-cyan-300/40"${active ? " checked" : ""} />
                <span class="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-sm text-zinc-200">${escapeHTML(SYSTEM_LABELS[option] ?? option)}${PRE_ALPHA_AGENTS.has(option) ? renderPreAlphaBadge() : ""}</span>
              </label>
            `;
          })
          .join("");
        return `
          <fieldset class="block">
            <legend class="text-xs text-zinc-500">${escapeHTML(field.label)}</legend>
            <div class="mt-2 grid gap-2">${optionRows}</div>
          </fieldset>
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
    <p class="mb-3 text-xs text-zinc-500">Workspace defaults stored in the database. Lower layers only apply when a higher layer does not set a value. Priority: this run → card → project → workspace → environment.</p>
    <div class="grid gap-4">${rows}</div>
    <button type="button" data-action="admin#saveGlobalAgents" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save workspace agent settings</button>
  `;
}
