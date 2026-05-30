import { escapeHTML } from "./dom";
import type { ModelFieldDefinition } from "./model_config_types";

const SOURCE_LABELS: Record<string, string> = {
  env: "Environment defaults",
  project: "Project overrides",
  card: "Card overrides",
};

export function renderModelConfigSection(
  fields: ModelFieldDefinition[],
  overrides: Record<string, string>,
  resolvedSource: string,
  scope: "project" | "card",
  entityId: string,
): string {
  const scopeLabel = scope === "project" ? "Project" : "Card";
  const fieldPrefix = scope === "project" ? "projects" : "scrum";
  const sourceLabel = SOURCE_LABELS[resolvedSource] ?? resolvedSource;
  const rows = fields
    .map((field) => {
      const override = overrides[field.key] ?? "";
      const inherited = field.value;
      const control = field.options?.length
        ? `<select
            data-${fieldPrefix}-field="model_${escapeHTML(field.key)}"
            class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40"
          >
            <option value="">Inherit${inherited ? ` (${escapeHTML(inherited)})` : ""}</option>
            ${field.options.map((option) => `<option value="${escapeHTML(option)}"${override === option ? " selected" : ""}>${escapeHTML(option)}</option>`).join("")}
          </select>`
        : `<input
            type="text"
            data-${fieldPrefix}-field="model_${escapeHTML(field.key)}"
            value="${escapeHTML(override)}"
            placeholder="${escapeHTML(inherited || "Inherit default")}"
            class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40"
          />`;
      return `
        <label class="block">
          <span class="text-xs text-zinc-500">${escapeHTML(field.label)}</span>
          ${control}
          <span class="mt-1 block text-[11px] leading-5 text-zinc-600">${escapeHTML(field.description)}</span>
        </label>
      `;
    })
    .join("");

  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex models</h3>
          <p class="mt-1 text-xs text-zinc-500">${scopeLabel}-level overrides. Empty fields inherit from ${scope === "card" ? "project, then environment" : "environment"}.</p>
        </div>
        <span class="rounded-full border border-white/10 bg-zinc-900/80 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">Effective: ${escapeHTML(sourceLabel)}</span>
      </div>
      <div class="mt-4 grid gap-4 lg:grid-cols-2">${rows}</div>
      <div class="mt-4 flex flex-wrap gap-2">
        <button
          type="button"
          data-action="${fieldPrefix}#saveModelConfig"
          data-${fieldPrefix === "projects" ? "project" : "card"}-id="${escapeHTML(entityId)}"
          class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200"
        >Save model settings</button>
        <button
          type="button"
          data-action="${fieldPrefix}#clearModelConfig"
          data-${fieldPrefix === "projects" ? "project" : "card"}-id="${escapeHTML(entityId)}"
          class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10"
        >Clear overrides</button>
      </div>
    </section>
  `;
}

export function collectModelFieldValues(root: ParentNode, scope: "project" | "card"): Record<string, string> {
  const prefix = scope === "project" ? "projects" : "scrum";
  const out: Record<string, string> = {};
  for (const input of root.querySelectorAll(`[data-${prefix}-field^="model_"]`)) {
    const element = input as HTMLInputElement | HTMLSelectElement;
    const key = element.dataset[`${prefix}Field`]?.replace(/^model_/, "") ?? "";
    const value = element.value.trim();
    if (key && value) out[key] = value;
  }
  return out;
}

export function clearModelFieldInputs(root: ParentNode, scope: "project" | "card"): void {
  const prefix = scope === "project" ? "projects" : "scrum";
  for (const input of root.querySelectorAll(`[data-${prefix}-field^="model_"]`)) {
    (input as HTMLInputElement | HTMLSelectElement).value = "";
  }
}
