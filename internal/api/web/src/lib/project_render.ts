import { escapeHTML, formatDateTime, statusPillClass } from "./dom";
import { renderModelConfigSection } from "./model_config_render";
import { renderAgentConfigSection } from "./agent_config_render";
import { renderProjectScrumShell } from "./scrum_render";
import { renderProjectTerminalShell } from "./terminal_render";
import { renderProjectScreenShell } from "./screen_render";
import { renderProjectChatShell } from "./project_chat_render";
import { renderProjectGitSection } from "./project_git_render";
import type { ModelFieldDefinition } from "./model_config_types";
import type { AgentFieldDefinition } from "./agent_config_types";
import type { BrowseResponse } from "./project_types";
import type { ProjectRecord, ProjectMapSummary, ProjectGitStatus, RecipeCatalogItem } from "./project_types";
import { AUTO_PLAY_WORK_COLUMNS, COLUMN_LABELS, DEFAULT_AUTO_WORK_COLUMNS, type ScrumAutoWorkConfig, type ScrumCreateTicketConfig } from "./scrum_types";

const PROJECT_TABS = [
  { id: "scrum", label: "Scrum" },
  { id: "chat", label: "Project chat" },
  { id: "terminal", label: "Terminal" },
  { id: "screen", label: "Screen" },
  { id: "settings", label: "Settings" },
  { id: "map", label: "Codebase map" },
  { id: "git", label: "Git" },
  { id: "recipe", label: "Recipe" },
] as const;

function renderProjectTabNav(activeTab: string): string {
  return PROJECT_TABS.map((tab) => {
    const active = tab.id === activeTab;
    const classes = active
      ? "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
      : "border-white/10 text-zinc-400 hover:border-cyan-300/30 hover:text-zinc-200";
    return `<button type="button" data-action="projects#showTab" data-project-tab="${tab.id}" class="rounded-md border px-3 py-2 text-sm font-medium transition ${classes}">${escapeHTML(tab.label)}</button>`;
  }).join("");
}

function scrumAutoReviewFromProject(project: ProjectRecord): { enabled: boolean; bounce_column: string } {
  const raw = project.settings?.scrum_auto_review;
  if (!raw || typeof raw !== "object") {
    return { enabled: false, bounce_column: "assigned" };
  }
  const cfg = raw as Record<string, unknown>;
  const bounce = typeof cfg.bounce_column === "string" && cfg.bounce_column.trim() ? cfg.bounce_column.trim() : "assigned";
  return { enabled: Boolean(cfg.enabled), bounce_column: bounce };
}

function scrumAutoWorkFromProject(project: ProjectRecord): ScrumAutoWorkConfig {
  const raw = project.settings?.scrum_auto_work;
  if (!raw || typeof raw !== "object") {
    return { enabled: false, source_columns: [...DEFAULT_AUTO_WORK_COLUMNS] };
  }
  const cfg = raw as Record<string, unknown>;
  const sourceColumns = Array.isArray(cfg.source_columns)
    ? cfg.source_columns.filter((item): item is string => typeof item === "string" && item.trim().length > 0)
    : [...DEFAULT_AUTO_WORK_COLUMNS];
  return {
    enabled: Boolean(cfg.enabled),
    source_columns: sourceColumns.length ? sourceColumns : [...DEFAULT_AUTO_WORK_COLUMNS],
  };
}

function scrumCreateTicketFromProject(project: ProjectRecord): ScrumCreateTicketConfig {
  const raw = project.settings?.scrum_create_ticket;
  if (!raw || typeof raw !== "object") {
    return { enabled: false, column: "backlog" };
  }
  const cfg = raw as Record<string, unknown>;
  const column = typeof cfg.column === "string" && ["backlog", "ready", "assigned"].includes(cfg.column)
    ? cfg.column
    : "backlog";
  return { enabled: Boolean(cfg.enabled), column };
}

function renderScrumAutomationSettings(project: ProjectRecord): string {
  const autoReview = scrumAutoReviewFromProject(project);
  const autoWork = scrumAutoWorkFromProject(project);
  const createTicket = scrumCreateTicketFromProject(project);
  const sourceColumns = new Set((autoWork.source_columns?.length ? autoWork.source_columns : [...DEFAULT_AUTO_WORK_COLUMNS]).map((col) => col.trim()).filter(Boolean));
  const bounceOptions = ["assigned", "in_progress", "ready"]
    .map((column) => {
      const selected = autoReview.bounce_column === column ? " selected" : "";
      const label = column === "in_progress" ? "In Progress" : column.charAt(0).toUpperCase() + column.slice(1);
      return `<option value="${escapeHTML(column)}"${selected}>${escapeHTML(label)}</option>`;
    })
    .join("");
  const createColumnOptions = ["backlog", "ready", "assigned"]
    .map((column) => {
      const selected = createTicket.column === column ? " selected" : "";
      return `<option value="${escapeHTML(column)}"${selected}>${escapeHTML(COLUMN_LABELS[column] ?? column)}</option>`;
    })
    .join("");
  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Scrum automation</h3>
          <p class="mt-1 max-w-2xl text-xs leading-relaxed text-zinc-500">
            When a card first lands in Review, queue an independent agent pass over the changes — checklist, test criteria, and recipe guide included.
            Cards that fail bounce back for another implementation pass.
          </p>
        </div>
      </div>
      <div class="mt-4 grid gap-4 lg:grid-cols-2">
        <label class="flex cursor-pointer items-start gap-3 rounded-lg border border-white/10 bg-zinc-900/50 px-3 py-3 transition hover:border-cyan-300/30">
          <input type="checkbox" data-projects-field="autoWorkEnabled" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${autoWork.enabled ? " checked" : ""} />
          <span>
            <span class="block text-sm font-medium text-zinc-100">Auto-work queue</span>
            <span class="mt-1 block text-xs text-zinc-500">Pull cards from selected columns and start play automatically.</span>
          </span>
        </label>
        <label class="flex cursor-pointer items-start gap-3 rounded-lg border border-white/10 bg-zinc-900/50 px-3 py-3 transition hover:border-cyan-300/30">
          <input type="checkbox" data-projects-field="autoReviewEnabled" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${autoReview.enabled ? " checked" : ""} />
          <span>
            <span class="block text-sm font-medium text-zinc-100">Auto-review on Review</span>
            <span class="mt-1 block text-xs text-zinc-500">Fresh agent eyes verify the work before humans see it.</span>
          </span>
        </label>
        <label class="flex cursor-pointer items-start gap-3 rounded-lg border border-white/10 bg-zinc-900/50 px-3 py-3 transition hover:border-cyan-300/30">
          <input type="checkbox" data-projects-field="createTicketEnabled" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${createTicket.enabled ? " checked" : ""} />
          <span>
            <span class="block text-sm font-medium text-zinc-100">Generate ticket on card create</span>
            <span class="mt-1 block text-xs text-zinc-500">Queue a planning-mode card ticket job from the new card title and description.</span>
          </span>
        </label>
        <label class="block">
          <span class="text-xs text-zinc-500">Create new ticket cards in</span>
          <select data-projects-field="createTicketColumn" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">
            ${createColumnOptions}
          </select>
        </label>
        <fieldset class="lg:col-span-2">
          <legend class="text-xs text-zinc-500">Auto-work source columns</legend>
          <div class="mt-2 flex flex-wrap gap-2">
            ${AUTO_PLAY_WORK_COLUMNS.map((column) => {
              const checked = sourceColumns.has(column) ? " checked" : "";
              return `
                <label class="flex cursor-pointer items-center gap-2 rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2 text-xs text-zinc-300 transition hover:border-cyan-300/30">
                  <input type="checkbox" data-projects-field="autoWorkColumn" data-auto-work-column="${escapeHTML(column)}" class="rounded border-white/20 bg-zinc-900 text-cyan-300"${checked} />
                  <span>${escapeHTML(COLUMN_LABELS[column] ?? column)}</span>
                </label>
              `;
            }).join("")}
          </div>
        </fieldset>
        <label class="block">
          <span class="text-xs text-zinc-500">Bounce failed reviews to</span>
          <select data-projects-field="autoReviewBounce" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">
            ${bounceOptions}
          </select>
        </label>
      </div>
      <div class="mt-4">
        <button type="button" data-action="projects#saveScrumAutomation" data-project-id="${project.id}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save scrum automation</button>
      </div>
    </section>
  `;
}

export function renderProjectList(projects: ProjectRecord[]): string {
  if (!projects.length) {
    return `<div class="rounded-xl border border-dashed border-white/10 p-8 text-sm text-zinc-500">No projects yet. Create one by choosing a working directory on your machine.</div>`;
  }
  return projects
    .map((project) => {
      const autoWork = scrumAutoWorkFromProject(project);
      const controlAction = autoWork.enabled ? "projects#pauseAutoWork" : "projects#startAutoWork";
      const controlTitle = autoWork.enabled ? "Pause auto-work" : "Start auto-work";
      const controlLabel = `${controlTitle} for ${project.name}`;
      const controlClasses = autoWork.enabled
        ? "grid h-9 w-9 shrink-0 place-items-center rounded-md border border-amber-300/30 bg-amber-300/10 text-sm font-semibold text-amber-100 transition hover:bg-amber-300/20"
        : "grid h-9 w-9 shrink-0 place-items-center rounded-md border border-cyan-300/30 bg-cyan-300/10 text-sm font-semibold text-cyan-100 transition hover:bg-cyan-300/20";
      const controlGlyph = autoWork.enabled ? "⏸" : "▶";
      return `
        <div class="rounded-xl border border-white/10 bg-zinc-950/60 p-4 transition hover:border-cyan-300/30 hover:bg-cyan-300/5">
          <div class="flex items-start gap-3">
            <button
              type="button"
              data-action="projects#openProject"
              data-project-id="${project.id}"
              class="min-w-0 flex-1 text-left"
            >
              <h3 class="truncate text-base font-semibold text-zinc-100">${escapeHTML(project.name)}</h3>
              <p class="mt-1 truncate font-mono text-xs text-zinc-500">${escapeHTML(project.location)}</p>
            </button>
            <div class="shrink-0 text-right text-[11px] text-zinc-500">
              <div>${escapeHTML(formatDateTime(project.updated_at))}</div>
              <div class="mt-1">${project.card_count ?? 0} cards · ${project.job_count ?? 0} jobs</div>
            </div>
            <button
              type="button"
              data-action="${controlAction}"
              data-project-id="${project.id}"
              class="${controlClasses}"
              title="${escapeHTML(controlTitle)}"
              aria-label="${escapeHTML(controlLabel)}"
            >${controlGlyph}</button>
          </div>
          ${
            project.project_state
              ? `<div class="mt-3"><span class="${statusPillClass(project.project_state)}">${escapeHTML(project.project_state)}</span></div>`
              : ""
          }
        </div>
      `;
    })
    .join("");
}

export function renderProjectDetail(
  project: ProjectRecord,
  recipes: RecipeCatalogItem[],
  modelFields: ModelFieldDefinition[] = [],
  resolvedModelSource = "env",
  agentFields: AgentFieldDefinition[] = [],
  resolvedAgentSource = "env",
  resolvedAgentSystem = "omnidex",
  projectMap: ProjectMapSummary | null = null,
  projectGit: ProjectGitStatus | null = null,
  activeTab = "scrum",
): string {
  const recipeOptions = recipes
    .map((recipe) => {
      const selected = recipe.id === project.recipe_id ? " selected" : "";
      return `<option value="${escapeHTML(recipe.id)}"${selected}>${escapeHTML(recipe.id)} — ${escapeHTML(recipe.description)}</option>`;
    })
    .join("");

  const recipeJSON = JSON.stringify(project.recipe ?? {}, null, 2);

  const settingsTab = `
    <div data-project-tab-panel="settings" class="scrollbar space-y-4">
      <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Project</h3>
        <div class="mt-4 grid gap-4 lg:grid-cols-2">
          <label class="block">
            <span class="text-xs text-zinc-500">Name</span>
            <input data-projects-field="name" value="${escapeHTML(project.name)}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
          </label>
          <label class="block">
            <span class="text-xs text-zinc-500">Detected state</span>
            <input readonly value="${escapeHTML(project.project_state || "unknown")}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2 text-sm text-zinc-400" />
          </label>
          <label class="block lg:col-span-2">
            <span class="text-xs text-zinc-500">Working directory</span>
            <div class="mt-1 flex gap-2">
              <input data-projects-field="location" value="${escapeHTML(project.location)}" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />
              <button type="button" data-action="projects#browseForEdit" data-project-id="${project.id}" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Choose folder…</button>
            </div>
          </label>
          <label class="block lg:col-span-2">
            <span class="text-xs text-zinc-500">Description</span>
            <textarea data-projects-field="description" rows="3" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">${escapeHTML(project.description || "")}</textarea>
          </label>
        </div>
        <div class="mt-4 flex flex-wrap gap-2">
          <button type="button" data-action="projects#saveProject" data-project-id="${project.id}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save project</button>
          <button type="button" data-action="projects#rescanProject" data-project-id="${project.id}" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Detect stack</button>
          <button type="button" data-action="projects#deleteProject" data-project-id="${project.id}" class="rounded-md border border-rose-400/30 px-4 py-2 text-sm text-rose-300 hover:bg-rose-400/10">Delete</button>
        </div>
      </section>

      ${
        modelFields.length
          ? renderModelConfigSection(modelFields, project.model_config ?? {}, resolvedModelSource, "project", String(project.id))
          : ""
      }

      ${
        agentFields.length
          ? renderAgentConfigSection(
              agentFields,
              project.agent_config ?? {},
              resolvedAgentSource,
              resolvedAgentSystem,
              "project",
              String(project.id),
            )
          : ""
      }

      ${renderScrumAutomationSettings(project)}
    </div>
  `;

  const mapTab = `<div data-project-tab-panel="map" class="scrollbar space-y-4">${renderProjectMapSection(project.id, projectMap)}</div>`;

  const gitTab = `<div data-project-tab-panel="git" class="scrollbar space-y-4">${renderProjectGitSection(project.id, projectGit)}</div>`;

  const recipeTab = `
    <div data-project-tab-panel="recipe" class="scrollbar space-y-4">
      <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex recipe</h3>
            <p class="mt-1 text-xs text-zinc-500">Catalog recipe plus per-project overrides stored in the database.</p>
          </div>
          <select data-projects-field="recipeId" class="max-w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
            <option value="">No catalog recipe</option>
            ${recipeOptions}
          </select>
        </div>
        <textarea data-projects-target="recipeEditor" data-projects-field="recipeJson" rows="18" class="scrollbar mt-4 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs leading-5 text-zinc-100 outline-none focus:border-cyan-300/40">${escapeHTML(recipeJSON)}</textarea>
        <div class="mt-3 flex flex-wrap gap-2">
          <button type="button" data-action="projects#loadCatalogRecipe" data-project-id="${project.id}" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Load catalog template</button>
          <button type="button" data-action="projects#saveRecipe" data-project-id="${project.id}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save recipe</button>
        </div>
      </section>
    </div>
  `;

  const activePanel = (() => {
    switch (activeTab) {
      case "chat":
        return renderProjectChatShell(project.name, { reasoning_mode: "instant" }, []);
      case "terminal":
        return renderProjectTerminalShell(project.location);
      case "screen":
        return renderProjectScreenShell(project.location);
      case "settings":
        return settingsTab;
      case "map":
        return mapTab;
      case "git":
        return gitTab;
      case "recipe":
        return recipeTab;
      case "scrum":
      default:
        return renderProjectScrumShell(project.location);
    }
  })();

  return `
    <div data-controller="terminal screen project-chat" class="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden">
      <div class="flex shrink-0 flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <button type="button" data-action="projects#backToList" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10">← All projects</button>
          <h3 class="mt-3 truncate text-2xl font-semibold tracking-tight text-zinc-100">${escapeHTML(project.name)}</h3>
          <p class="mt-1 truncate font-mono text-xs text-zinc-500">${escapeHTML(project.location)}</p>
        </div>
        <div class="flex shrink-0 flex-wrap gap-2">
          <button
            type="button"
            data-action="projects#openDebuggerModal"
            data-project-id="${project.id}"
            class="rounded-md border border-amber-300/30 bg-amber-300/10 px-4 py-2 text-sm font-semibold text-amber-100 hover:border-amber-200/50 hover:bg-amber-300/15"
          >Analyze</button>
        </div>
      </div>

      <nav class="flex shrink-0 flex-wrap gap-2" aria-label="Project sections">${renderProjectTabNav(activeTab)}</nav>

      <div class="project-tab-stack">
      ${activePanel}
      </div>
    </div>
  `;
}

export function renderProjectMapSection(projectID: number, map: ProjectMapSummary | null): string {
  if (!map) {
    return `
      <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Codebase map</h3>
            <p class="mt-1 text-xs text-zinc-500">What Omnidex knows about this project directory for routing and planning.</p>
          </div>
          <button type="button" data-action="projects#scanProjectMap" data-project-id="${projectID}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Scan project directory</button>
        </div>
        <p class="mt-4 text-sm text-zinc-500">No map loaded yet.</p>
      </section>
    `;
  }

  const statusBadge = map.exists
    ? `<span class="rounded-full border border-cyan-300/30 bg-cyan-300/10 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-cyan-200">Live</span>`
    : `<span class="rounded-full border border-amber-300/30 bg-amber-300/10 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-amber-200">Unavailable</span>`;

  const stats = [
    ["Files", map.file_count],
    ["Modules", map.module_count],
    ["Scan", map.scan_truncated ? "Limited" : "Complete"],
  ];

  const languageRows = (map.languages ?? [])
    .slice(0, 8)
    .map((lang) => `<div class="flex items-center justify-between gap-3 text-xs"><span class="text-zinc-300">${escapeHTML(lang.language)}</span><span class="font-mono text-zinc-500">${lang.files}</span></div>`)
    .join("");

  const moduleRows = (map.modules ?? [])
    .slice(0, 10)
    .map((mod) => {
      const files = (mod.important_files ?? []).slice(0, 4).map((file) => `<li class="font-mono text-[11px] text-zinc-500">${escapeHTML(file)}</li>`).join("");
      return `
        <article class="rounded-md border border-white/10 bg-zinc-900/50 p-3">
          <div class="font-mono text-xs text-cyan-200">${escapeHTML(mod.path || ".")}</div>
          ${mod.purpose ? `<p class="mt-2 text-xs leading-5 text-zinc-400">${escapeHTML(mod.purpose)}</p>` : ""}
          ${files ? `<ul class="mt-2 space-y-1">${files}</ul>` : ""}
        </article>
      `;
    })
    .join("");

  const entrypointRows = (map.entrypoints ?? [])
    .slice(0, 8)
    .map((entry) => `<li class="font-mono text-[11px] text-zinc-400">${escapeHTML(entry.path)}${entry.kind ? ` · ${escapeHTML(entry.kind)}` : ""}</li>`)
    .join("");

  const commandRows = (map.commands ?? [])
    .slice(0, 6)
    .map((cmd) => `<li class="text-xs text-zinc-400"><span class="font-mono text-zinc-200">${escapeHTML(cmd.name)}</span> · ${escapeHTML(cmd.command)}</li>`)
    .join("");

  const treePreview = map.tree_preview?.trim()
    ? `<pre class="scrollbar mt-3 max-h-56 overflow-auto whitespace-pre-wrap rounded-md border border-white/10 bg-black/40 p-3 font-mono text-[11px] leading-5 text-zinc-300">${escapeHTML(map.tree_preview)}</pre>`
    : `<p class="mt-3 text-sm text-zinc-500">No indexed files yet. Scan the project directory to build the map.</p>`;

  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Codebase map</h3>
            ${statusBadge}
          </div>
          <p class="mt-1 text-xs text-zinc-500">Current filesystem context for likely files, modules, tests, and verification commands. Nothing is persisted in the project.</p>
          ${map.root ? `<p class="mt-2 font-mono text-[11px] text-zinc-600">${escapeHTML(map.root)}</p>` : ""}
          ${map.generated_at ? `<p class="mt-1 text-[11px] text-zinc-600">Scanned ${escapeHTML(formatDateTime(map.generated_at))}</p>` : ""}
        </div>
        <button type="button" data-action="projects#scanProjectMap" data-project-id="${projectID}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Refresh current map</button>
      </div>

      <div class="mt-4 grid gap-3 sm:grid-cols-3">
        ${stats
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

      <div class="mt-5 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Indexed files</h4>
          ${treePreview}
        </div>
        <div class="space-y-4">
          ${languageRows ? `<div><h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Languages</h4><div class="mt-2 space-y-1">${languageRows}</div></div>` : ""}
          ${entrypointRows ? `<div><h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Entrypoints</h4><ul class="mt-2 space-y-1">${entrypointRows}</ul></div>` : ""}
          ${commandRows ? `<div><h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Commands</h4><ul class="mt-2 space-y-1">${commandRows}</ul></div>` : ""}
        </div>
      </div>

      ${moduleRows ? `<div class="mt-5"><h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Modules</h4><div class="mt-3 grid gap-3 lg:grid-cols-2">${moduleRows}</div></div>` : ""}
    </section>
  `;
}

export function renderBrowseModal(data: BrowseResponse, selectedPath: string, mode: string): string {
  const dirs = (data.entries ?? []).filter((entry) => entry.is_dir);
  const rows = dirs
    .map((entry) => {
      const selected = selectedPath === entry.path ? " border-cyan-300/40 bg-cyan-300/10" : " border-white/10";
      return `
        <div class="flex gap-2">
          <button
            type="button"
            data-action="projects#selectBrowseDir"
            data-path="${escapeHTML(entry.path)}"
            class="flex min-w-0 flex-1 items-center gap-3 rounded-md border px-3 py-2 text-left text-sm text-zinc-200 transition hover:border-cyan-300/30${selected}"
          >
            <span aria-hidden="true">📁</span>
            <span class="truncate">${escapeHTML(entry.name)}</span>
          </button>
          <button
            type="button"
            data-action="projects#enterBrowseDir"
            data-path="${escapeHTML(entry.path)}"
            title="Open folder"
            class="shrink-0 rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10"
          >Open</button>
        </div>
      `;
    })
    .join("");

  const currentSelected = selectedPath || data.path;

  return `
    <div class="border-b border-white/10 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-xl font-semibold text-zinc-100">Choose working directory</h2>
          <p class="mt-1 text-sm text-zinc-500">Browse folders on the host via the bridge. Select a folder, or create a new one below.</p>
          <p class="mt-2 font-mono text-xs text-zinc-500">${escapeHTML(data.path)}</p>
        </div>
        <button type="button" data-action="projects#closeBrowse" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10">Close</button>
      </div>
    </div>
    <div class="omni-modal-body scrollbar grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_300px]">
      <div data-projects-modal-feedback class="hidden lg:col-span-2 rounded-md border px-3 py-2 text-sm" role="status"></div>
      <div class="space-y-2">
        ${
          data.parent
            ? `<button type="button" data-action="projects#enterBrowseDir" data-path="${escapeHTML(data.parent)}" class="w-full rounded-md border border-white/10 px-3 py-2 text-left text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10">↑ Parent directory</button>`
            : ""
        }
        <div class="scrollbar space-y-2">${rows || `<p class="text-sm text-zinc-500">No subfolders here — you can use this directory or create a new folder.</p>`}</div>
      </div>
      <aside class="space-y-4">
        <div class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Selected directory</h3>
          <p class="mt-3 break-all font-mono text-xs text-zinc-300">${escapeHTML(currentSelected)}</p>
          <input type="hidden" data-browse-mode="${escapeHTML(mode)}" value="${escapeHTML(mode)}" />
          <button
            type="button"
            data-action="projects#confirmBrowse"
            data-path="${escapeHTML(currentSelected)}"
            class="mt-4 w-full rounded-md bg-cyan-300 px-3 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200"
          >Use this directory</button>
        </div>
        <div class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">New folder</h3>
          <p class="mt-2 text-xs leading-5 text-zinc-500">Create inside <span class="font-mono text-zinc-400">${escapeHTML(data.path)}</span></p>
          <input
            data-browse-field="newFolderName"
            type="text"
            placeholder="my-project"
            class="mt-3 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40"
          />
          <button
            type="button"
            data-action="projects#createBrowseFolder"
            class="mt-3 w-full rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10"
          >Create folder</button>
        </div>
      </aside>
    </div>
  `;
}

export function renderProjectCreateModal(recipes: RecipeCatalogItem[]): string {
  const recipeOptions = recipes
    .map((recipe) => `<option value="${escapeHTML(recipe.id)}">${escapeHTML(recipe.id)}</option>`)
    .join("");
  return `
    <div class="border-b border-white/10 p-4 md:p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="text-xs uppercase tracking-[.20em] text-cyan-200/80">Projects</p>
          <h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">New project</h2>
          <p class="mt-1 text-sm text-zinc-500">Browse and choose a working directory on the host via the bridge.</p>
        </div>
        <button type="button" data-action="projects#closeCreateModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Cancel</button>
      </div>
    </div>
    <form data-action="submit->projects#submitCreate" class="omni-modal-body scrollbar space-y-4 p-4 md:p-5">
      <div data-projects-modal-feedback class="hidden rounded-md border px-3 py-2 text-sm" role="status"></div>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Working directory</span>
        <div class="mt-2 flex gap-2">
          <input data-projects-field="selectedPath" type="text" readonly placeholder="Browse to choose a directory…" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100" />
          <button data-action="projects#openBrowse" type="button" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-100 hover:border-cyan-300/40 hover:bg-cyan-300/10">Choose folder…</button>
        </div>
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Name</span>
        <input data-projects-field="createName" type="text" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" placeholder="Project name" />
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Catalog recipe</span>
        <select data-projects-field="createRecipe" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
          <option value="">No catalog recipe</option>
          ${recipeOptions}
        </select>
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Description</span>
        <textarea data-projects-field="createDesc" rows="3" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
      </label>
      <div class="flex justify-end gap-2 border-t border-white/10 pt-4">
        <button type="button" data-action="projects#closeCreateModal" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300">Cancel</button>
        <button type="submit" data-projects-create-submit class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200 disabled:cursor-not-allowed disabled:opacity-60">Create project</button>
      </div>
    </form>
  `;
}
