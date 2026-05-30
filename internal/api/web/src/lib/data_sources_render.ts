import { escapeHTML, formatDateTime } from "./dom";
import type { DataSource, DataSourceCatalog, DataSourceQueryResult, DataSourceSchemaTable } from "./admin_api";

export type DataSourcesViewState = {
  sources: DataSource[];
  selectedId: string | null;
  editingId: string | null;
  schema: DataSourceSchemaTable[] | null;
  catalog: DataSourceCatalog | null;
  catalogReady: boolean;
  queryResult: DataSourceQueryResult | null;
  chartLabelCol: string;
  chartValueCol: string;
};

export function emptyDataSourcesViewState(sources: DataSource[] = []): DataSourcesViewState {
  return {
    sources,
    selectedId: sources[0]?.id ?? null,
    editingId: null,
    schema: null,
    catalog: null,
    catalogReady: false,
    queryResult: null,
    chartLabelCol: "",
    chartValueCol: "",
  };
}

function testStatusBadge(source: DataSource): string {
  const status = (source.last_test_status || "").toLowerCase();
  if (status === "ok") {
    return `<span class="rounded-full border border-emerald-300/30 bg-emerald-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-emerald-200">Connected</span>`;
  }
  if (status === "failed") {
    return `<span class="rounded-full border border-rose-300/30 bg-rose-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-rose-200">Failed</span>`;
  }
  return `<span class="rounded-full border border-zinc-600/50 bg-zinc-800/50 px-2 py-0.5 text-[10px] font-semibold uppercase text-zinc-400">Untested</span>`;
}

function renderSourceForm(source: Partial<DataSource> | null, editingId: string | null): string {
  const isEdit = Boolean(editingId);
  const useDSN = source?.use_dsn ?? false;
  return `
    <form data-action="submit->admin#saveDataSource" class="mt-4 grid gap-3">
      <input type="hidden" data-ds-field="id" value="${escapeHTML(editingId || "")}" />
      <label class="block">
        <span class="text-xs text-zinc-500">Name</span>
        <input data-ds-field="name" value="${escapeHTML(source?.name || "")}" placeholder="Production analytics" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" required />
      </label>
      <div class="grid gap-3 md:grid-cols-2">
        <label class="block">
          <span class="text-xs text-zinc-500">Database type</span>
          <select data-ds-field="driver" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
            <option value="postgres" ${(source?.driver || "postgres") === "postgres" ? "selected" : ""}>PostgreSQL</option>
          </select>
        </label>
        <label class="block">
          <span class="text-xs text-zinc-500">Domain</span>
          <select data-ds-field="domain" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
            ${["generic", "healthcare", "gaming", "analytics"]
              .map((domain) => `<option value="${domain}" ${(source?.domain || "generic") === domain ? "selected" : ""}>${domain}</option>`)
              .join("")}
          </select>
        </label>
      </div>
      <label class="block">
        <span class="text-xs text-zinc-500">Context prompt</span>
            <textarea data-ds-field="context_prompt" rows="3" placeholder="Outpatient EMR; staff ask about at-risk patients, appointment volume, and sentiment in portal feedback comments." class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">${escapeHTML(source?.context_prompt || "")}</textarea>
            <span class="mt-1 block text-[11px] text-zinc-600">Guides schema exploration and query planning. Patient comment text is analyzed for themes/sentiment — raw text is not shown to staff.</span>
      </label>
      <label class="block md:max-w-xs">
        <span class="text-xs text-zinc-500">Privacy mode</span>
        <select data-ds-field="privacy_mode" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
          ${["strict", "standard"]
            .map((mode) => `<option value="${mode}" ${(source?.privacy_mode || "strict") === mode ? "selected" : ""}>${mode}</option>`)
            .join("")}
        </select>
      </label>
      <label class="flex items-center gap-2 text-sm text-zinc-300">
        <input type="checkbox" data-ds-field="use_dsn" ${useDSN ? "checked" : ""} class="rounded border-white/20 bg-zinc-900" />
        Use connection string (DSN)
      </label>
      <div data-ds-panel="dsn" class="${useDSN ? "" : "hidden"} grid gap-3">
        <label class="block">
          <span class="text-xs text-zinc-500">PostgreSQL DSN</span>
          <input data-ds-field="dsn" value="" placeholder="${source?.password_set ? "Leave blank to keep current DSN" : "postgres://user:pass@host:5432/db?sslmode=prefer"}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />
          ${source?.password_hint ? `<span class="mt-1 block text-[11px] text-zinc-600">Current: ${escapeHTML(source.password_hint)}</span>` : ""}
        </label>
      </div>
      <div data-ds-panel="fields" class="${useDSN ? "hidden" : ""} grid gap-3 md:grid-cols-2">
        <label class="block md:col-span-2">
          <span class="text-xs text-zinc-500">Host</span>
          <input data-ds-field="host" value="${escapeHTML(source?.host || "")}" placeholder="localhost" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        </label>
        <label class="block">
          <span class="text-xs text-zinc-500">Port</span>
          <input data-ds-field="port" type="number" min="1" max="65535" value="${source?.port || 5432}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        </label>
        <label class="block">
          <span class="text-xs text-zinc-500">Database</span>
          <input data-ds-field="database_name" value="${escapeHTML(source?.database_name || "")}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        </label>
        <label class="block">
          <span class="text-xs text-zinc-500">Username</span>
          <input data-ds-field="username" value="${escapeHTML(source?.username || "")}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        </label>
        <label class="block">
          <span class="text-xs text-zinc-500">Password</span>
          <input data-ds-field="password" type="password" value="" placeholder="${source?.password_set ? "Leave blank to keep current password" : ""}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
          ${source?.password_hint ? `<span class="mt-1 block text-[11px] text-zinc-600">Current: ${escapeHTML(source.password_hint)}</span>` : ""}
        </label>
        <label class="block md:col-span-2">
          <span class="text-xs text-zinc-500">SSL mode</span>
          <select data-ds-field="ssl_mode" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
            ${["disable", "allow", "prefer", "require", "verify-ca", "verify-full"]
              .map((mode) => `<option value="${mode}" ${(source?.ssl_mode || "prefer") === mode ? "selected" : ""}>${mode}</option>`)
              .join("")}
          </select>
        </label>
      </div>
      <p class="text-xs text-zinc-500">Connections are read-only. Only SELECT / WITH queries are allowed in the query builder.</p>
      <div class="flex flex-wrap gap-2">
        <button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">${isEdit ? "Save changes" : "Add data source"}</button>
        ${isEdit ? `<button type="button" data-action="admin#cancelEditDataSource" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300 hover:border-cyan-300/30">Cancel</button>` : ""}
      </div>
    </form>
  `;
}

function renderSourceList(state: DataSourcesViewState): string {
  if (!state.sources.length) {
    return `<p class="text-sm text-zinc-500">No data sources yet. Add a read-only PostgreSQL connection below.</p>`;
  }
  return `
    <div class="space-y-2">
      ${state.sources
        .map((source) => {
          const selected = source.id === state.selectedId;
          const border = selected ? "border-cyan-300/40 bg-cyan-300/5" : "border-white/10 bg-zinc-900/50";
          const meta = source.last_test_at
            ? `Tested ${escapeHTML(formatDateTime(source.last_test_at))}`
            : "Not tested yet";
          return `
            <article class="rounded-md border ${border} px-3 py-3">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <button type="button" data-action="admin#selectDataSource" data-source-id="${escapeHTML(source.id)}" class="min-w-0 text-left">
                  <div class="font-medium text-zinc-100">${escapeHTML(source.name)}</div>
                  <div class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(source.use_dsn ? "DSN" : `${source.host}:${source.port}/${source.database_name}`)} · ${escapeHTML(source.username || "n/a")}</div>
                  <div class="mt-1 text-[11px] text-zinc-600">${meta}${source.last_test_message ? ` · ${escapeHTML(source.last_test_message)}` : ""}</div>
                </button>
                <div class="flex flex-wrap items-center gap-2">
                  ${testStatusBadge(source)}
                  <button type="button" data-action="admin#testDataSource" data-source-id="${escapeHTML(source.id)}" class="rounded-md border border-white/10 px-2 py-1 text-xs text-zinc-300 hover:border-cyan-300/30">Test</button>
                  <button type="button" data-action="admin#editDataSource" data-source-id="${escapeHTML(source.id)}" class="rounded-md border border-white/10 px-2 py-1 text-xs text-zinc-300 hover:border-cyan-300/30">Edit</button>
                  <button type="button" data-action="admin#deleteDataSource" data-source-id="${escapeHTML(source.id)}" class="rounded-md border border-rose-300/30 px-2 py-1 text-xs text-rose-200 hover:bg-rose-400/10">Remove</button>
                </div>
              </div>
            </article>
          `;
        })
        .join("")}
    </div>
  `;
}

function renderCatalogBrowser(catalog: DataSourceCatalog | null, ready: boolean): string {
  if (!catalog?.tables?.length) {
    return `<p class="text-sm text-zinc-500">${ready ? "Catalog is empty." : "No schema map yet. Run Explore to build a metadata-only catalog and reference memories."}</p>`;
  }
  const summary = catalog.summary ? `<p class="mb-3 text-xs leading-relaxed text-zinc-400">${escapeHTML(catalog.summary)}</p>` : "";
  return `
    ${summary}
    <div class="scrollbar max-h-[280px] space-y-2 overflow-y-auto">
      ${catalog.tables
        .slice(0, 40)
        .map((table) => {
          const cols = (table.columns || [])
            .slice(0, 8)
            .map((col) => {
              const sensitive = col.sensitive ? " text-rose-300/80" : " text-zinc-500";
              const hint = col.hint ? ` — ${escapeHTML(col.hint)}` : "";
              return `<li class="font-mono text-[10px]${sensitive}">${escapeHTML(col.name)} <span class="text-zinc-600">${escapeHTML(col.data_type)}</span>${hint}</li>`;
            })
            .join("");
          const purpose = table.purpose ? `<p class="mt-1 text-[11px] text-zinc-400">${escapeHTML(table.purpose)}</p>` : "";
          const rows = table.row_estimate ? `<span class="text-zinc-600">~${table.row_estimate.toLocaleString()} rows</span>` : "";
          return `
            <details class="rounded border border-white/10 bg-zinc-950/40 px-2 py-2">
              <summary class="cursor-pointer font-mono text-xs text-cyan-100">${escapeHTML(table.full_name)} ${rows}</summary>
              ${purpose}
              <ul class="mt-2 space-y-0.5">${cols}</ul>
            </details>
          `;
        })
        .join("")}
    </div>
  `;
}

function renderSchemaBrowser(schema: DataSourceSchemaTable[] | null): string {
  if (!schema) {
    return `<p class="text-sm text-zinc-500">Select a source and load schema to browse tables.</p>`;
  }
  if (!schema.length) {
    return `<p class="text-sm text-zinc-500">No tables found in public schema.</p>`;
  }
  return `
    <div class="scrollbar max-h-[280px] space-y-2 overflow-y-auto">
      ${schema
        .map((table) => {
          const fullName = `${table.schema}.${table.name}`;
          const cols = (table.columns || [])
            .map((col) => `<li class="font-mono text-[11px] text-zinc-400">${escapeHTML(col.name)} <span class="text-zinc-600">${escapeHTML(col.data_type)}${col.nullable ? "" : " NOT NULL"}</span></li>`)
            .join("");
          return `
            <details class="rounded-md border border-white/10 bg-zinc-900/40 px-3 py-2">
              <summary class="cursor-pointer font-mono text-xs text-cyan-200">${escapeHTML(fullName)}</summary>
              <ul class="mt-2 space-y-1 pl-2">${cols || `<li class="text-[11px] text-zinc-600">No columns</li>`}</ul>
              <button type="button" data-action="admin#insertSchemaQuery" data-table-name="${escapeHTML(fullName)}" class="mt-2 rounded border border-white/10 px-2 py-0.5 text-[11px] text-zinc-400 hover:border-cyan-300/30">Insert SELECT *</button>
            </details>
          `;
        })
        .join("")}
    </div>
  `;
}

function isNumericValue(value: unknown): boolean {
  if (typeof value === "number" && Number.isFinite(value)) return true;
  if (typeof value === "string" && value.trim() !== "") {
    const n = Number(value);
    return Number.isFinite(n);
  }
  return false;
}

function renderResultsTable(result: DataSourceQueryResult | null): string {
  if (!result) {
    return `<p class="text-sm text-zinc-500">Run a SQL query or ask a question to see results.</p>`;
  }
  const columns = result.columns || [];
  const rows = result.rows || [];
  if (!columns.length) {
    return `<p class="text-sm text-zinc-500">Query returned no columns.</p>`;
  }
  const head = columns.map((col) => `<th class="px-3 py-2 text-left font-mono text-[11px] uppercase tracking-wide text-zinc-500">${escapeHTML(col)}</th>`).join("");
  const body = rows
    .map((row) => {
      const cells = columns
        .map((col) => {
          const value = row[col];
          const text = value == null ? "" : String(value);
          return `<td class="px-3 py-2 font-mono text-xs text-zinc-300">${escapeHTML(text)}</td>`;
        })
        .join("");
      return `<tr class="border-t border-white/5">${cells}</tr>`;
    })
    .join("");
  const meta = [
    result.answer ? `<p class="text-sm text-zinc-300">${escapeHTML(result.answer)}</p>` : "",
    result.query_steps && result.query_steps.length > 1
      ? `<p class="mt-1 text-[11px] text-cyan-300/80">${result.query_steps.length} investigation queries</p>`
      : "",
    result.text_insights && result.text_insights.length
      ? `<details class="mt-2 rounded border border-violet-300/20 bg-violet-300/5 px-3 py-2"><summary class="cursor-pointer text-[11px] font-semibold uppercase tracking-[.18em] text-violet-200/90">Text insights (${result.text_insights.length})</summary><ul class="mt-2 space-y-2">${result.text_insights.map((insight) => `<li class="text-xs text-zinc-300"><span class="font-mono text-violet-200/90">${escapeHTML(insight.field)}</span> · ${insight.samples} samples · ${escapeHTML(insight.summary)}${insight.themes && insight.themes.length ? `<div class="mt-1 text-[10px] text-zinc-500">Themes: ${escapeHTML(insight.themes.join(", "))}</div>` : ""}</li>`).join("")}</ul></details>`
      : "",
    result.hard_facts && result.hard_facts.length
      ? `<details class="mt-2 rounded border border-white/10 bg-zinc-950/40 px-3 py-2"><summary class="cursor-pointer text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Hard facts (${result.hard_facts.length})</summary><ul class="mt-2 space-y-1">${result.hard_facts.slice(0, 12).map((fact) => `<li class="font-mono text-[10px] text-zinc-400">${escapeHTML(fact)}</li>`).join("")}${result.hard_facts.length > 12 ? `<li class="text-[10px] text-zinc-600">… +${result.hard_facts.length - 12} more</li>` : ""}</ul></details>`
      : "",
    result.sql ? `<p class="mt-2 font-mono text-[11px] text-zinc-500">${escapeHTML(result.sql)}</p>` : "",
    `<p class="mt-1 text-[11px] text-zinc-600">${rows.length} row${rows.length === 1 ? "" : "s"}</p>`,
  ].join("");
  return `
    ${meta}
    <div class="scrollbar mt-3 max-h-[360px] overflow-auto rounded-lg border border-white/10">
      <table class="min-w-full border-collapse">
        <thead class="sticky top-0 bg-zinc-950/95"><tr>${head}</tr></thead>
        <tbody>${body || `<tr><td class="px-3 py-4 text-sm text-zinc-500" colspan="${columns.length}">No rows</td></tr>`}</tbody>
      </table>
    </div>
  `;
}

function renderChart(state: DataSourcesViewState): string {
  const result = state.queryResult;
  if (!result?.rows?.length || !result.columns?.length) {
    return "";
  }
  const numericCols = result.columns.filter((col) => result.rows.some((row) => isNumericValue(row[col])));
  if (!numericCols.length) {
    return `<p class="mt-4 text-xs text-zinc-600">No numeric columns detected for charting.</p>`;
  }
  const labelCol = state.chartLabelCol || result.columns[0] || "";
  const valueCol = state.chartValueCol || numericCols[0] || "";
  const labelOptions = result.columns
    .map((col) => `<option value="${escapeHTML(col)}" ${col === labelCol ? "selected" : ""}>${escapeHTML(col)}</option>`)
    .join("");
  const valueOptions = numericCols
    .map((col) => `<option value="${escapeHTML(col)}" ${col === valueCol ? "selected" : ""}>${escapeHTML(col)}</option>`)
    .join("");
  const chartRows = result.rows.slice(0, 24);
  const maxVal = Math.max(
    ...chartRows.map((row) => {
      const n = Number(row[valueCol]);
      return Number.isFinite(n) ? n : 0;
    }),
    1,
  );
  const bars = chartRows
    .map((row) => {
      const label = String(row[labelCol] ?? "");
      const n = Number(row[valueCol]);
      const value = Number.isFinite(n) ? n : 0;
      const width = Math.max(2, Math.round((value / maxVal) * 100));
      return `
        <div class="grid grid-cols-[minmax(0,140px)_1fr_auto] items-center gap-2 text-xs">
          <span class="truncate font-mono text-zinc-400" title="${escapeHTML(label)}">${escapeHTML(label)}</span>
          <div class="h-5 rounded bg-zinc-900/80">
            <div class="h-full rounded bg-cyan-400/70" style="width:${width}%"></div>
          </div>
          <span class="font-mono text-zinc-300">${escapeHTML(String(value))}</span>
        </div>
      `;
    })
    .join("");
  return `
    <section class="mt-4 rounded-lg border border-white/10 bg-zinc-950/40 p-4">
      <h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Chart</h4>
      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="block text-xs text-zinc-500">
          Label column
          <select data-ds-field="chart_label" data-action="change->admin#updateDataSourceChart" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-2 py-1.5 text-sm text-zinc-100">${labelOptions}</select>
        </label>
        <label class="block text-xs text-zinc-500">
          Value column
          <select data-ds-field="chart_value" data-action="change->admin#updateDataSourceChart" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-2 py-1.5 text-sm text-zinc-100">${valueOptions}</select>
        </label>
      </div>
      <div class="mt-4 space-y-2">${bars}</div>
    </section>
  `;
}

function renderExplorer(state: DataSourcesViewState): string {
  const selected = state.sources.find((s) => s.id === state.selectedId);
  if (!selected) {
    return `<p class="text-sm text-zinc-500">Add and select a data source to explore it.</p>`;
  }
  return `
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 class="text-sm font-semibold text-zinc-200">${escapeHTML(selected.name)}</h4>
          <p class="text-xs text-zinc-500">${escapeHTML(selected.domain || "generic")} · ${escapeHTML(selected.privacy_mode || "strict")} privacy · Query builder · read-only · max 500 rows · <a href="?panel=data&amp;data_source=${escapeHTML(selected.id)}" class="text-cyan-300 hover:text-cyan-200">Open data chat</a></p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button type="button" data-action="admin#exploreDataSource" data-source-id="${escapeHTML(selected.id)}" class="rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-1.5 text-xs font-semibold text-cyan-100 hover:bg-cyan-300/20">Explore database</button>
          <button type="button" data-action="admin#loadDataSourceCatalog" data-source-id="${escapeHTML(selected.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300 hover:border-cyan-300/30">Load schema map</button>
          <button type="button" data-action="admin#loadDataSourceSchema" data-source-id="${escapeHTML(selected.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300 hover:border-cyan-300/30">Load raw schema</button>
        </div>
      </div>

      <div class="grid gap-4 lg:grid-cols-[minmax(0,280px)_minmax(0,1fr)]">
        <div class="space-y-4">
          <div>
            <h5 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Schema map</h5>
            <div class="mt-2">${renderCatalogBrowser(state.catalog, state.catalogReady)}</div>
          </div>
          <div>
            <h5 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Raw schema</h5>
            <div class="mt-2">${renderSchemaBrowser(state.schema)}</div>
          </div>
        </div>
        <div class="space-y-4">
          <div>
            <h5 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">SQL</h5>
            <textarea data-ds-field="sql" rows="5" placeholder="SELECT * FROM my_table LIMIT 20" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
            <div class="mt-2 flex flex-wrap gap-2">
              <button type="button" data-action="admin#runDataSourceQuery" data-source-id="${escapeHTML(selected.id)}" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Run query</button>
            </div>
          </div>
          <div>
            <h5 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Ask in plain language</h5>
            <form data-action="submit->admin#askDataSource" class="mt-2 flex flex-wrap gap-2">
              <input type="hidden" name="source_id" value="${escapeHTML(selected.id)}" />
              <input data-ds-field="question" placeholder="What themes show up in patient portal feedback this week?" class="min-w-[220px] flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
              <button type="submit" class="rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-2 text-xs font-semibold text-cyan-100 hover:bg-cyan-300/20">Ask (job)</button>
            </form>
            <p class="mt-2 text-[11px] text-zinc-600">Natural-language questions use the schema map, pick relevant tables, plan SQL, and run as background jobs. Strict privacy mode blocks queries that may expose PHI.</p>
          </div>
        </div>
      </div>

      <div>
        <h5 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Results</h5>
        <div class="mt-2">${renderResultsTable(state.queryResult)}</div>
        ${renderChart(state)}
      </div>
    </div>
  `;
}

export function renderDataSourcesPanel(state: DataSourcesViewState): string {
  const editingSource = state.editingId ? state.sources.find((s) => s.id === state.editingId) ?? null : null;
  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/50 p-5">
      <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Configured sources</h3>
      <p class="mt-1 text-xs text-zinc-500">Read-only PostgreSQL connections for ad-hoc queries, natural-language questions, and charts.</p>
      <div class="mt-4">${renderSourceList(state)}</div>
    </section>

    <section class="rounded-xl border border-white/10 bg-zinc-950/50 p-5">
      <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">${state.editingId ? "Edit data source" : "Add data source"}</h3>
      ${renderSourceForm(editingSource, state.editingId)}
    </section>

    <section class="rounded-xl border border-white/10 bg-zinc-950/50 p-5">
      <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Query explorer</h3>
      <div class="mt-4">${renderExplorer(state)}</div>
    </section>
  `;
}
