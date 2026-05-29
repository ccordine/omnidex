import { escapeHTML, trimText, formatDateTime, emptyState, statusPillClass } from "./dom";

export function renderStep(step) {
  const state = stepVisualState(step.status);
  return `
    <article class="${stepCardClass(state)}">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex min-w-0 items-center gap-2">
          <span class="${stepMarkerClass(state)}">${escapeHTML(stepMarkerText(state))}</span>
          <div class="font-mono text-xs text-cyan-200">step #${step.id}</div>
        </div>
        <span class="${statusPillClass(step.status)}">${escapeHTML(step.status || "unknown")}</span>
      </div>
      <div class="mt-2 text-sm font-medium text-zinc-100">${escapeHTML(step.action || "step")}</div>
      ${step.output ? `<pre class="mt-3 max-h-44 overflow-auto whitespace-pre-wrap rounded bg-zinc-950/70 p-3 text-xs leading-5 text-zinc-300">${escapeHTML(step.output)}</pre>` : ""}
      ${step.error ? `<pre class="mt-3 max-h-44 overflow-auto whitespace-pre-wrap rounded bg-rose-400/10 p-3 text-xs leading-5 text-rose-100">${escapeHTML(step.error)}</pre>` : ""}
    </article>
  `;
}

export function renderStepSummary(steps) {
  let completed = 0;
  let incomplete = 0;
  let failed = 0;
  const active = steps.find((step) => stepVisualState(step.status) === "active");
  for (const step of steps) {
    const state = stepVisualState(step.status);
    if (state === "done") {
      completed += 1;
    } else {
      incomplete += 1;
      if (state === "failed") failed += 1;
    }
  }
  const activeText = active ? `active #${active.id}${active.action ? ` ${active.action}` : ""}` : "no active step";
  return `
    <div class="flex flex-wrap items-center gap-2 text-[11px] font-medium">
      <span class="rounded border border-cyan-300/30 bg-cyan-300/10 px-2 py-1 text-cyan-100">${escapeHTML(activeText)}</span>
      <span class="rounded border border-emerald-300/25 bg-emerald-300/10 px-2 py-1 text-emerald-100">done ${completed}</span>
      <span class="rounded border border-amber-300/25 bg-amber-300/10 px-2 py-1 text-amber-100">incomplete ${incomplete}</span>
      ${failed ? `<span class="rounded border border-rose-300/25 bg-rose-300/10 px-2 py-1 text-rose-100">failed ${failed}</span>` : ""}
    </div>
  `;
}

export function stepVisualState(status) {
  switch (String(status || "").toLowerCase()) {
    case "running":
    case "waiting_input":
      return "active";
    case "completed":
      return "done";
    case "failed":
    case "canceled":
      return "failed";
    case "pending":
      return "pending";
    default:
      return "unknown";
  }
}

export function stepCardClass(state) {
  const base = "rounded-md border p-3 transition";
  switch (state) {
    case "active":
      return `${base} active-step-card border-cyan-300/60 bg-cyan-300/10`;
    case "done":
      return `${base} border-emerald-300/25 bg-emerald-300/[.06]`;
    case "failed":
      return `${base} border-rose-300/35 bg-rose-400/[.08]`;
    case "pending":
      return `${base} border-white/10 bg-white/[.025] opacity-80`;
    default:
      return `${base} border-white/10 bg-white/[.035]`;
  }
}

export function stepMarkerClass(state) {
  const base = "rounded px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-[.14em]";
  switch (state) {
    case "active":
      return `${base} bg-cyan-300 text-zinc-950`;
    case "done":
      return `${base} bg-emerald-300/20 text-emerald-100`;
    case "failed":
      return `${base} bg-rose-300/20 text-rose-100`;
    case "pending":
      return `${base} bg-zinc-300/10 text-zinc-400`;
    default:
      return `${base} bg-zinc-300/10 text-zinc-300`;
  }
}

export function stepMarkerText(state) {
  switch (state) {
    case "active":
      return "active";
    case "done":
      return "done";
    case "failed":
      return "stop";
    case "pending":
      return "todo";
    default:
      return "step";
  }
}

export function renderContext(context) {
  return `
    <button type="button" data-action="chat#openContextItem" data-context-id="${escapeHTML(context.id || "")}" class="block w-full rounded-md border border-white/10 bg-white/[.03] p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-zinc-400">${escapeHTML(context.key || "context")}</span>
        <span class="font-mono text-[11px] text-zinc-600">ctx ${escapeHTML(context.id || "")} · step ${escapeHTML(context.step_id || "")}</span>
      </div>
      <pre class="mt-2 max-h-40 overflow-auto whitespace-pre-wrap text-xs leading-5 text-zinc-300">${escapeHTML(trimText(context.value || "", 1200))}</pre>
    </button>
  `;
}

export function contextEventType(key) {
  key = String(key || "").toLowerCase();
  if (key.includes("prompt")) return "llm_prompt";
  if (key.includes("response") || key.includes("completion")) return "llm_response";
  if (key.includes("context")) return "llm_context";
  return "context_recorded";
}

export function renderEventModal(event) {
  const full = event.full || {};
  const context = full.context;
  const step = full.step;
  const job = full.job || full.job_snapshot;
  return `
    <div class="border-b border-white/10 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">${escapeHTML(event.id)}</div>
          <h2 class="mt-1 text-xl font-semibold text-zinc-100">${escapeHTML(event.type)}</h2>
          <p class="mt-1 text-xs text-zinc-500">${formatDateTime(event.at)}</p>
        </div>
        <button type="button" data-action="chat#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Close</button>
      </div>
    </div>
    <div class="grid gap-4 p-4 lg:grid-cols-[320px_minmax(0,1fr)]">
      <section class="rounded-md border border-white/10 bg-white/[.03] p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Details</h3>
        <div class="mt-3 space-y-2 font-mono text-xs text-zinc-300">${renderDetailRows(event.details)}</div>
        ${job ? `<div class="mt-4 rounded border border-white/10 bg-zinc-950/60 p-3"><div class="text-xs uppercase tracking-[.16em] text-zinc-500">Job</div><div class="mt-1 font-mono text-xs text-cyan-200">#${escapeHTML(job.id || "")}</div><div class="mt-1 text-xs text-zinc-300">${escapeHTML(job.status || "")}</div></div>` : ""}
        ${step ? `<div class="mt-4 rounded border border-white/10 bg-zinc-950/60 p-3"><div class="text-xs uppercase tracking-[.16em] text-zinc-500">Step</div><div class="mt-1 font-mono text-xs text-cyan-200">#${escapeHTML(step.id || "")}</div><div class="mt-1 text-xs text-zinc-300">${escapeHTML(step.action || step.status || "")}</div></div>` : ""}
      </section>
      <section class="min-w-0 rounded-md border border-white/10 bg-white/[.03] p-4">
        ${context ? renderContextPayload(context) : ""}
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Full payload</h3>
        <pre class="scrollbar mt-3 max-h-[58vh] overflow-auto whitespace-pre-wrap rounded bg-zinc-950/70 p-3 text-xs leading-5 text-zinc-300">${escapeHTML(JSON.stringify(full, null, 2))}</pre>
      </section>
    </div>
  `;
}

export function renderContextModal(context) {
  return `
    <div class="border-b border-white/10 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">ctx ${escapeHTML(context.id || "")}</div>
          <h2 class="mt-1 text-xl font-semibold text-zinc-100">${escapeHTML(context.key || "context")}</h2>
          <p class="mt-1 text-xs text-zinc-500">step ${escapeHTML(context.step_id || "")}</p>
        </div>
        <button type="button" data-action="chat#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Close</button>
      </div>
    </div>
    <div class="p-4">
      ${renderContextPayload(context)}
      <h3 class="mt-4 text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Raw context</h3>
      <pre class="scrollbar mt-3 max-h-[58vh] overflow-auto whitespace-pre-wrap rounded bg-zinc-950/70 p-3 text-xs leading-5 text-zinc-300">${escapeHTML(JSON.stringify(context, null, 2))}</pre>
    </div>
  `;
}

export function renderContextPayload(context) {
  return `
    <div class="mb-4 rounded-md border border-cyan-300/20 bg-cyan-300/5 p-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-cyan-200">Captured context</h3>
        <span class="font-mono text-[11px] text-zinc-500">step ${escapeHTML(context.step_id || "")}</span>
      </div>
      <pre class="scrollbar mt-3 max-h-[44vh] overflow-auto whitespace-pre-wrap text-xs leading-5 text-zinc-200">${escapeHTML(context.value || "")}</pre>
    </div>
  `;
}

export function renderResearchStatus(payload) {
  const generation = payload.generation_provider || {};
  const ollama = payload.ollama || {};
  const web = payload.web_search || {};
  const warnings = payload.warnings || [];
  const probes = web.probes || [];
  return `
    <div class="space-y-3">
      <div class="grid grid-cols-2 gap-2 text-xs">
        ${metricTile("Runnable", payload.research_runnable ? "yes" : "no", payload.research_runnable ? "ok" : "bad")}
        ${metricTile("Provider", generation.provider || "unknown", generation.reachable ? "ok" : "bad")}
        ${metricTile("Ollama", ollama.reachable ? "reachable" : "down", ollama.reachable ? "ok" : "bad")}
        ${metricTile("Web", web.enabled ? (web.reachable_provider ? "reachable" : "degraded") : "disabled", web.enabled && web.reachable_provider ? "ok" : "warn")}
      </div>
      <div class="rounded border border-white/10 bg-white/[.03] p-3">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Ollama</div>
        <dl class="mt-2 space-y-1 font-mono text-xs text-zinc-300">
          <div><span class="text-zinc-500">base_url</span> ${escapeHTML(ollama.base_url || "n/a")}</div>
          <div><span class="text-zinc-500">configured</span> ${escapeHTML((ollama.configured_models || []).join(", ") || "none")}</div>
          <div><span class="text-zinc-500">missing</span> ${escapeHTML((ollama.missing_models || []).join(", ") || "none")}</div>
          <div><span class="text-zinc-500">embedding</span> ${escapeHTML(ollama.embedding_model || "n/a")} ${ollama.embedding_available ? "(available)" : "(not found)"}</div>
          ${ollama.last_provider_error ? `<div class="text-rose-200"><span class="text-rose-300">error</span> ${escapeHTML(ollama.last_provider_error)}</div>` : ""}
        </dl>
      </div>
      <div class="rounded border border-white/10 bg-white/[.03] p-3">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Web Providers</div>
        <div class="mt-2 space-y-2">
          ${probes.map((probe) => `
            <div class="rounded border border-white/10 bg-zinc-950/40 p-2 font-mono text-xs">
              <div class="flex items-center justify-between gap-2">
                <span class="text-zinc-200">${escapeHTML(probe.provider || "provider")}</span>
                <span class="${probe.reachable ? "text-emerald-200" : "text-rose-200"}">${probe.reachable ? "ok" : "failed"}</span>
              </div>
              <div class="mt-1 truncate text-zinc-500">${escapeHTML(probe.target_url || "")}</div>
              ${probe.error ? `<div class="mt-1 text-rose-200">${escapeHTML(probe.error)}</div>` : `<div class="mt-1 text-zinc-400">status=${escapeHTML(probe.status_code || "")}</div>`}
            </div>
          `).join("") || `<div class="text-zinc-500">No provider probes.</div>`}
        </div>
      </div>
      ${warnings.length ? `<div class="rounded border border-amber-300/30 bg-amber-300/10 p-3 text-sm leading-6 text-amber-100">${warnings.map(escapeHTML).join("<br>")}</div>` : ""}
    </div>
  `;
}

export function renderMetricsDashboard(live, models, playbooks, benchmarks) {
  const statusCounts = live.status_counts || {};
  const liveRuns = live.live_runs || [];
  const recentRuns = live.recent_runs || [];
  const blockers = live.common_blockers || [];
  const completed = Number(statusCounts.completed || 0);
  const failed = Number(statusCounts.failed || 0);
  const cancelled = Number(statusCounts.canceled || statusCounts.cancelled || 0);
  const totalTerminal = completed + failed + cancelled;
  const successRate = totalTerminal > 0 ? `${Math.round((completed / totalTerminal) * 100)}%` : "n/a";
  return `
    <div class="grid gap-4 xl:grid-cols-4">
      ${metricTile("Live Runs", String(liveRuns.length), liveRuns.length ? "warn" : "ok")}
      ${metricTile("Recent Runs", String(recentRuns.length), "ok")}
      ${metricTile("Success Rate", successRate, completed >= failed ? "ok" : "warn")}
      ${metricTile("Blocker Types", String(blockers.length), blockers.length ? "warn" : "ok")}
    </div>
    <div class="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Live Run Timeline</h3>
          <span class="font-mono text-xs text-zinc-500">${escapeHTML(liveRuns.length)} active</span>
        </div>
        <div class="mt-3 space-y-3">${liveRuns.map(renderMetricRun).join("") || emptyState("No active telemetry runs.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Run Health</h3>
        <div class="mt-3 grid grid-cols-2 gap-2 text-xs">
          ${Object.entries(statusCounts).map(([key, value]) => metricTile(key, String(value), key === "completed" ? "ok" : key === "failed" ? "bad" : "warn")).join("") || emptyState("No status counts yet.")}
        </div>
      </section>
    </div>
    <div class="grid gap-4 xl:grid-cols-3">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Recent Outcomes</h3>
        <div class="mt-3 space-y-2">${recentRuns.slice(0, 8).map(renderMetricRun).join("") || emptyState("No telemetry runs yet.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Common Blockers</h3>
        <div class="mt-3 space-y-2">${blockers.map(renderMetricCount).join("") || emptyState("No blocker metrics yet.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Model Performance</h3>
        <div class="mt-3 space-y-2">${models.slice(0, 8).map(renderMetricModel).join("") || emptyState("No model metrics yet.")}</div>
      </section>
    </div>
    <div class="grid gap-4 xl:grid-cols-2">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Playbook Leverage</h3>
        <div class="mt-3 space-y-2">${playbooks.slice(0, 8).map(renderMetricPlaybook).join("") || emptyState("No playbook usage metrics yet.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Benchmarks</h3>
        <div class="mt-3 space-y-2">${benchmarks.slice(0, 8).map(renderMetricBenchmark).join("") || emptyState("No benchmark metrics yet.")}</div>
      </section>
    </div>
  `;
}

export function renderMetricRun(run) {
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-cyan-200">${escapeHTML((run.id || "").slice(0, 8) || "run")}</span>
        <span class="${statusPillClass(run.status)}">${escapeHTML(run.status || "unknown")}</span>
      </div>
      <div class="mt-2 text-sm text-zinc-200">${escapeHTML(run.task_kind || run.project_type || "unclassified task")}</div>
      <div class="mt-1 font-mono text-xs text-zinc-500">${escapeHTML(run.workspace_id || "workspace n/a")} · ${escapeHTML(formatDurationMS(run.duration_ms))}</div>
    </div>
  `;
}

export function renderMetricCount(item) {
  return `
    <div class="flex items-center justify-between gap-3 rounded border border-white/10 bg-white/[.03] p-3">
      <span class="font-mono text-xs text-zinc-300">${escapeHTML(item.key || "unknown")}</span>
      <span class="font-mono text-xs text-amber-200">${escapeHTML(item.count || 0)}</span>
    </div>
  `;
}

export function renderMetricModel(model) {
  const calls = Number(model.calls || 0);
  const successes = Number(model.successes || 0);
  const rate = calls > 0 ? `${Math.round((successes / calls) * 100)}%` : "n/a";
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex items-center justify-between gap-2">
        <span class="truncate text-sm text-zinc-200">${escapeHTML(model.role || "role")}</span>
        <span class="font-mono text-xs text-emerald-200">${escapeHTML(rate)}</span>
      </div>
      <div class="mt-1 truncate font-mono text-xs text-zinc-500">${escapeHTML(model.provider || "provider")} / ${escapeHTML(model.model || "model")}</div>
      <div class="mt-2 grid grid-cols-3 gap-2 text-center font-mono text-xs text-zinc-300">
        <span>calls ${escapeHTML(calls)}</span>
        <span>bad ${escapeHTML(model.failures || 0)}</span>
        <span>${escapeHTML(Math.round(model.avg_latency_ms || 0))}ms</span>
      </div>
    </div>
  `;
}

export function renderMetricPlaybook(playbook) {
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="truncate font-mono text-xs text-cyan-200">${escapeHTML(playbook.playbook_id || "playbook")}</div>
      <div class="mt-2 grid grid-cols-4 gap-2 text-center font-mono text-xs text-zinc-300">
        <span>uses ${escapeHTML(playbook.uses || 0)}</span>
        <span>reused ${escapeHTML(playbook.reused || 0)}</span>
        <span>ok ${escapeHTML(playbook.successes || 0)}</span>
        <span>bad ${escapeHTML(playbook.failures || 0)}</span>
      </div>
    </div>
  `;
}

export function renderMetricBenchmark(item) {
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex items-center justify-between gap-2">
        <span class="truncate font-mono text-xs text-cyan-200">${escapeHTML(item.benchmark_id || "benchmark")}</span>
        <span class="font-mono text-xs text-zinc-500">${escapeHTML(item.runs || 0)} runs</span>
      </div>
      <div class="mt-2 grid grid-cols-3 gap-2 text-center font-mono text-xs text-zinc-300">
        <span>ok ${escapeHTML(item.successes || 0)}</span>
        <span>bad ${escapeHTML(item.failures || 0)}</span>
        <span>${escapeHTML(formatDurationMS(item.avg_duration_ms))}</span>
      </div>
    </div>
  `;
}

export function metricTile(label, value, mode) {
  const tone = mode === "ok" ? "text-emerald-200" : mode === "bad" ? "text-rose-200" : "text-amber-200";
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="text-[11px] uppercase tracking-[.16em] text-zinc-500">${escapeHTML(label)}</div>
      <div class="mt-1 truncate font-mono text-xs ${tone}">${escapeHTML(value)}</div>
    </div>
  `;
}

export function formatDurationMS(value) {
  const ms = Number(value || 0);
  if (!Number.isFinite(ms) || ms <= 0) return "n/a";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${Math.round(ms / 1000)}s`;
  return `${Math.round(ms / 60000)}m`;
}

export function renderDetailRows(details) {
  const entries = Object.entries(details || {});
  if (entries.length === 0) return `<div class="text-zinc-500">No details.</div>`;
  return entries
    .map(([key, value]) => `
      <div class="grid grid-cols-[96px_minmax(0,1fr)] gap-3">
        <span class="text-zinc-500">${escapeHTML(key)}</span>
        <span class="break-words text-zinc-200">${escapeHTML(String(value))}</span>
      </div>
    `)
    .join("");
}

