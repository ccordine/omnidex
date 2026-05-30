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

export function renderHostBridgeStatus(payload) {
  const suggestions = payload.suggestions || [];
  const tone = payload.reachable ? "ok" : payload.configured ? "bad" : "warn";
  return `
    <div class="space-y-3">
      <div class="grid grid-cols-2 gap-2 text-xs">
        ${metricTile("Bridge", payload.reachable ? "reachable" : "down", payload.reachable ? "ok" : "bad")}
        ${metricTile("Configured", payload.configured ? "yes" : "no", payload.configured ? "ok" : "warn")}
        ${metricTile("Picker", payload.picker_ready ? "ready" : "unavailable", payload.picker_ready ? "ok" : "warn")}
        ${metricTile("Native UI", payload.native_picker ? "yes" : "n/a", payload.native_picker ? "ok" : "warn")}
      </div>
      <div class="rounded border border-white/10 bg-white/[.03] p-3">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Host bridge</div>
        <dl class="mt-2 space-y-1 font-mono text-xs text-zinc-300">
          <div><span class="text-zinc-500">url</span> ${escapeHTML(payload.url || "not set")}</div>
          <div><span class="text-zinc-500">service</span> ${escapeHTML(payload.service || "n/a")}</div>
          ${payload.error ? `<div class="text-rose-200"><span class="text-rose-300">error</span> ${escapeHTML(payload.error)}</div>` : ""}
        </dl>
        <p class="mt-3 text-sm leading-6 ${tone === "ok" ? "text-emerald-100" : "text-zinc-300"}">${escapeHTML(payload.message || "")}</p>
      </div>
      ${suggestions.length ? `
        <div class="rounded border border-amber-300/30 bg-amber-300/10 p-3">
          <div class="text-xs font-semibold uppercase tracking-[.16em] text-amber-200">How to fix</div>
          <ol class="mt-2 list-decimal space-y-2 pl-5 text-sm leading-6 text-amber-50">
            ${suggestions.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}
          </ol>
        </div>
      ` : ""}
    </div>
  `;
}

export type MetricsGlance = {
  live_runs?: number;
  recent_errors?: number;
  struggle_signals?: number;
  failed_runs?: number;
  struggling?: boolean;
  tone?: string;
};

export function renderMetricsNavBadges(glance: MetricsGlance): string {
  const parts: string[] = [];
  const liveRuns = Number(glance.live_runs || 0);
  const recentErrors = Number(glance.recent_errors || 0);
  const struggleSignals = Number(glance.struggle_signals || 0);
  const struggling = Boolean(glance.struggling);

  if (liveRuns > 0) {
    parts.push(`<span class="inline-flex min-w-[1.25rem] items-center justify-center rounded-full border border-cyan-300/30 bg-cyan-300/10 px-1.5 py-0.5 text-[10px] font-semibold text-cyan-100" title="Live runs">${liveRuns}</span>`);
  }
  if (recentErrors > 0) {
    parts.push(`<span class="inline-flex min-w-[1.25rem] items-center justify-center rounded-full border border-rose-400/35 bg-rose-950/80 px-1.5 py-0.5 text-[10px] font-semibold text-rose-100" title="Errors in the last hour">${recentErrors}</span>`);
  } else if (struggling && struggleSignals > 0) {
    parts.push(`<span class="inline-flex min-w-[1.25rem] items-center justify-center rounded-full border border-amber-300/30 bg-amber-300/10 px-1.5 py-0.5 text-[10px] font-semibold text-amber-100" title="Struggle signals (7d)">${struggleSignals}</span>`);
  }
  if (parts.length === 0) {
    return `<span class="text-zinc-500">05</span>`;
  }
  return `<span class="flex items-center gap-1.5">${parts.join("")}</span>`;
}

export function renderMetricsDashboard(live, models, playbooks, benchmarks, contextShrink, contextUsage, operations) {
  const statusCounts = live.status_counts || {};
  const liveRuns = live.live_runs || [];
  const recentRuns = live.recent_runs || [];
  const blockers = live.common_blockers || [];
  const struggle = live.struggle || {};
  const shrinkSummary = contextShrink?.summary || {};
  const shrinkHistory = contextShrink?.history || [];
  const shrinkDaily = contextShrink?.daily || [];
  const usageSummary = contextUsage?.summary || {};
  const usageBySource = contextUsage?.by_source || [];
  const usageOverloads = contextUsage?.overloads || [];
  const usageHistory = contextUsage?.history || [];
  const usageDaily = contextUsage?.daily || [];
  const struggleEvents = struggle.struggle_events || [];
  const acceptEvents = struggle.accept_events || [];
  const recoveryAttempts = Number(struggle.recovery_attempts || 0);
  const recoverySuccesses = Number(struggle.recovery_successes || 0);
  const recentStruggleRuns = Number(struggle.recent_struggle_runs || 0);
  const completed = Number(statusCounts.completed || 0);
  const failed = Number(statusCounts.failed || 0);
  const cancelled = Number(statusCounts.canceled || statusCounts.cancelled || 0);
  const totalTerminal = completed + failed + cancelled;
  const successRate = totalTerminal > 0 ? `${Math.round((completed / totalTerminal) * 100)}%` : "n/a";
  const struggleTotal = struggleEvents.reduce((sum, item) => sum + Number(item.count || 0), 0);
  const acceptTotal = acceptEvents.reduce((sum, item) => sum + Number(item.count || 0), 0);
  const struggling = struggleTotal > acceptTotal || recentStruggleRuns > 0;
  const shrinkRequests = Number(shrinkSummary.requests || 0);
  const shrinkSaved = Number(shrinkSummary.avg_saved_pct || 0);
  const usageRequests = Number(usageSummary.requests || 0);
  const llmFailures = Number(usageSummary.failure_events || operations?.llm_failures || 0);
  const overloadEvents = Number(usageSummary.overload_events || 0);
  const avgUtilization = Number(usageSummary.avg_utilization_pct || 0);
  const avgContextDelta = Number(usageSummary.avg_delta_chars || operations?.avg_context_delta_chars || 0);
  const contextLimit = Number(usageSummary.context_limit_chars || 0);
  const contextOverloaded = overloadEvents > 0 || avgUtilization >= 95 || llmFailures > 0;
  return `
    <div class="grid gap-4 xl:grid-cols-6">
      ${metricTile("Live Runs", String(liveRuns.length), liveRuns.length ? "warn" : "ok")}
      ${metricTile("Success Rate", successRate, completed >= failed ? "ok" : "warn")}
      ${metricTile("LLM Failures", String(llmFailures), llmFailures > 0 ? "bad" : "ok")}
      ${metricTile("LLM Context", usageRequests ? `${avgUtilization.toFixed(1)}% avg` : "n/a", contextOverloaded ? "bad" : avgUtilization >= 80 ? "warn" : "ok")}
      ${metricTile("Context Δ", avgContextDelta ? `+${formatCompactChars(avgContextDelta)} avg` : "n/a", avgContextDelta >= 2000 ? "bad" : avgContextDelta >= 800 ? "warn" : "ok")}
      ${metricTile("Overloads", String(overloadEvents), overloadEvents > 0 ? "bad" : "ok")}
    </div>
    ${renderOperationsSection(operations)}
    <section class="rounded-lg border ${contextOverloaded ? "border-rose-300/25 bg-rose-300/5" : "border-violet-300/20 bg-violet-300/5"} p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-violet-200/90">LLM context usage</h3>
          <p class="mt-1 text-xs text-zinc-400">Tracks prompt size vs model window across coach, card ticket, tags, pilot, and agent paths.</p>
        </div>
        <div class="grid grid-cols-2 gap-2 text-right font-mono text-xs sm:grid-cols-4">
          <div><span class="text-zinc-500">limit</span><div class="text-violet-200">${escapeHTML(formatCompactChars(contextLimit))}</div></div>
          <div><span class="text-zinc-500">avg sent</span><div class="text-cyan-200">${escapeHTML(formatCompactChars(usageSummary.avg_sent_chars))}</div></div>
          <div><span class="text-zinc-500">peak sent</span><div class="text-rose-200/90">${escapeHTML(formatCompactChars(usageSummary.max_sent_chars))}</div></div>
          <div><span class="text-zinc-500">failures</span><div class="${llmFailures ? "text-rose-200" : "text-emerald-200"}">${escapeHTML(String(llmFailures))}</div></div>
          <div><span class="text-zinc-500">avg Δ</span><div class="text-amber-200">${escapeHTML(formatCompactChars(avgContextDelta))}</div></div>
        </div>
      </div>
      <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(0,1fr)]">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">By source</h4>
          <div class="mt-2 space-y-2">${usageBySource.map(renderContextUsageBySource).join("") || emptyState("No LLM context telemetry yet.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-rose-300/80">Recent overloads (≥95% window)</h4>
          <div class="mt-2 max-h-[28rem] space-y-2 overflow-y-auto pr-1">${usageOverloads.slice(0, 16).map(renderContextUsageEntry).join("") || emptyState("No context overload events — prompts are within model limits.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">30-day utilization</h4>
          <div class="mt-2 space-y-2">${usageDaily.slice(-14).map(renderContextUsageDaily).join("") || emptyState("Daily context averages appear after LLM calls are recorded.")}</div>
        </div>
      </div>
      <div class="mt-4">
        <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">Recent LLM calls</h4>
        <div class="mt-2 max-h-64 space-y-2 overflow-y-auto pr-1">${usageHistory.slice(0, 12).map(renderContextUsageEntry).join("") || emptyState("Run coach, card ticket, or channel pilot to populate context telemetry.")}</div>
      </div>
    </section>
    <section class="rounded-lg border border-cyan-300/20 bg-cyan-300/5 p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-cyan-200/90">Context minification</h3>
          <p class="mt-1 text-xs text-zinc-400">Scrum pilot raw channel + card metadata vs caveman prompt sent to the LLM.</p>
        </div>
        <div class="grid grid-cols-2 gap-2 text-right font-mono text-xs sm:grid-cols-4">
          <div><span class="text-zinc-500">avg raw</span><div class="text-rose-200">${escapeHTML(formatCompactChars(shrinkSummary.avg_raw_chars))}</div></div>
          <div><span class="text-zinc-500">avg shrunk</span><div class="text-cyan-200">${escapeHTML(formatCompactChars(shrinkSummary.avg_shrunk_chars))}</div></div>
          <div><span class="text-zinc-500">peak raw</span><div class="text-rose-200/90">${escapeHTML(formatCompactChars(shrinkSummary.max_raw_chars))}</div></div>
          <div><span class="text-zinc-500">min shrunk</span><div class="text-emerald-200">${escapeHTML(formatCompactChars(shrinkSummary.min_shrunk_chars))}</div></div>
        </div>
      </div>
      <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)]">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">Recent shrink events</h4>
          <div class="mt-2 max-h-[28rem] space-y-2 overflow-y-auto pr-1">${shrinkHistory.slice(0, 24).map(renderContextShrinkEntry).join("") || emptyState("No context shrink telemetry yet — send a card channel pilot message.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">30-day trend</h4>
          <div class="mt-2 space-y-2">${shrinkDaily.slice(-14).map(renderContextShrinkDaily).join("") || emptyState("Daily shrink averages will appear after a few pilot requests.")}</div>
        </div>
      </div>
    </section>
    <section class="rounded-lg border ${struggling ? "border-amber-300/25 bg-amber-300/5" : "border-white/10 bg-zinc-950/50"} p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Operational health (7d)</h3>
        <span class="font-mono text-xs ${struggling ? "text-amber-200" : "text-emerald-200"}">${recentStruggleRuns} runs with struggle signals</span>
      </div>
      <div class="mt-3 grid gap-4 md:grid-cols-2">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-rose-300/80">Stuck / rejected / replanning</h4>
          <div class="mt-2 space-y-2">${struggleEvents.map(renderMetricCount).join("") || emptyState("No struggle signals yet — good sign.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-emerald-300/80">Accepted / passing</h4>
          <div class="mt-2 space-y-2">${acceptEvents.map(renderMetricCount).join("") || emptyState("No acceptance signals recorded yet.")}</div>
        </div>
      </div>
    </section>
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

export function formatCompactChars(value) {
  const n = Number(value || 0);
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(Math.round(n));
}

const llmSourceLabels = {
  scrum_coach: "Card coach",
  scrum_card_ticket: "Card ticket",
  scrum_tags_suggest: "Tag suggest",
  scrum_pilot: "Channel pilot",
  scrum_outcome_classifier: "Outcome classifier",
  project_planning_chat: "Project planning",
  scrum_llm: "Scrum LLM",
};

export function llmActivityLabel(source) {
  const key = String(source || "").replace(/^worker:/, "");
  return llmSourceLabels[key] || String(source || "LLM").replace(/^worker:/, "Worker · ");
}

export function renderJobsPanel(jobs, llmActivity) {
  const jobItems = (jobs || []).map(
    (job) => `
      <button data-action="chat#selectJob" data-job-id="${job.id}" class="w-full rounded-lg border border-white/10 bg-zinc-950/50 p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">
        <div class="flex items-start justify-between gap-3">
          <div>
            <div class="font-mono text-xs text-cyan-200">#${job.id}</div>
            <div class="mt-1 line-clamp-2 text-sm font-medium text-zinc-100">${escapeHTML(job.instruction)}</div>
          </div>
          <span class="${statusPillClass(job.status)}">${escapeHTML(job.status)}</span>
        </div>
        <div class="mt-2 text-xs text-zinc-500">${escapeHTML(job.pipeline || "assistant")} · ${formatDateTime(job.updated_at)}</div>
      </button>
    `,
  ).join("");

  const llmItems = (llmActivity || []).map(
    (entry) => `
      <div class="rounded-lg border border-violet-300/15 bg-violet-300/5 p-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-sm font-medium text-violet-100">${escapeHTML(llmActivityLabel(entry.source))}</span>
          <span class="font-mono text-[11px] ${entry.success === false ? "text-rose-200" : "text-emerald-200"}">${entry.success === false ? escapeHTML(entry.error_class || "failed") : "ok"}</span>
        </div>
        <div class="mt-1 font-mono text-xs text-zinc-400">${escapeHTML(formatCompactChars(entry.sent_chars))} chars · ${Number(entry.utilization_pct || 0).toFixed(0)}% window</div>
        <div class="mt-1 text-[11px] text-zinc-500">${formatDateTime(entry.created_at)}${entry.card_id ? ` · card ${escapeHTML(String(entry.card_id).slice(0, 12))}` : ""}${entry.job_id ? ` · linked job #${escapeHTML(String(entry.job_id))}` : ""}</div>
      </div>
    `,
  ).join("");

  return `
    <section>
      <h3 class="text-[11px] font-semibold uppercase tracking-[.14em] text-cyan-200/80">Queue jobs</h3>
      <p class="mt-1 text-[11px] text-zinc-500">Agent runs from Play, channel messages, and chat queue. Filter above applies here.</p>
      <div class="mt-3 space-y-3">${jobItems || emptyState("No queue jobs matched this filter.")}</div>
    </section>
    <section class="mt-6 border-t border-white/10 pt-5">
      <h3 class="text-[11px] font-semibold uppercase tracking-[.14em] text-violet-200/80">Instant LLM actions</h3>
      <p class="mt-1 text-[11px] text-zinc-500">Coach, tag suggest, and ticket generate run inline — they do not create queue jobs.</p>
      <div class="mt-3 space-y-2">${llmItems || emptyState("No recent coach, tag, or ticket LLM calls yet.")}</div>
    </section>
  `;
}

export function renderContextShrinkEntry(entry) {
  const saved = Number(entry.saved_pct || 0);
  const meta = entry.metadata && typeof entry.metadata === "object" ? entry.metadata : {};
  const title = meta.card_title || entry.card_id || entry.source || "pilot";
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="truncate text-sm text-zinc-200">${escapeHTML(String(title))}</span>
        <span class="font-mono text-xs text-emerald-200">${saved.toFixed(1)}% saved</span>
      </div>
      <div class="mt-2 flex flex-wrap items-baseline gap-2 font-mono text-sm">
        <span class="text-rose-200/90">${escapeHTML(formatCompactChars(entry.raw_chars))}</span>
        <span class="text-zinc-500">→</span>
        <span class="text-cyan-200">${escapeHTML(formatCompactChars(entry.shrunk_chars))}</span>
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(formatDateTime(entry.created_at))} · ${escapeHTML(String(entry.chat_messages || 0))} msgs · ${escapeHTML(String(entry.selected_chunks || 0))} chunks</div>
    </div>
  `;
}

export function renderContextShrinkDaily(point) {
  const saved = Number(point.avg_saved_pct || 0);
  const width = Math.max(4, Math.min(100, saved));
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex items-center justify-between gap-2 font-mono text-xs">
        <span class="text-zinc-400">${escapeHTML(point.day || "day")}</span>
        <span class="text-emerald-200">${saved.toFixed(1)}% · ${escapeHTML(String(point.requests || 0))} req</span>
      </div>
      <div class="mt-2 h-2 overflow-hidden rounded bg-zinc-900">
        <div class="h-full rounded bg-gradient-to-r from-cyan-400/70 to-emerald-400/70" style="width:${width}%"></div>
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(formatCompactChars(point.avg_raw_chars))} → ${escapeHTML(formatCompactChars(point.avg_shrunk_chars))}</div>
    </div>
  `;
}

export function renderContextUsageBySource(row) {
  const utilization = Number(row.avg_utilization_pct || 0);
  const overloads = Number(row.overload_events || 0);
  const tone = overloads > 0 ? "border-rose-300/20 bg-rose-400/5" : utilization >= 80 ? "border-amber-300/20 bg-amber-300/5" : "border-white/10 bg-white/[.03]";
  return `
    <div class="rounded border ${tone} p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-zinc-200">${escapeHTML(row.source || "unknown")}</span>
        <span class="font-mono text-xs ${overloads ? "text-rose-200" : "text-emerald-200"}">${escapeHTML(String(row.requests || 0))} req · ${overloads} overload</span>
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">avg ${escapeHTML(formatCompactChars(row.avg_sent_chars))} · ${utilization.toFixed(1)}% util · peak ${escapeHTML(formatCompactChars(row.max_sent_chars))}</div>
    </div>
  `;
}

export function renderContextUsageEntry(entry) {
  const utilization = Number(entry.utilization_pct || 0);
  const meta = entry.metadata && typeof entry.metadata === "object" ? entry.metadata : {};
  const title = meta.card_title || entry.card_id || entry.scope || entry.source || "llm";
  const limit = Number(entry.context_limit_chars || 0);
  const failed = entry.success === false;
  const delta = Number(entry.delta_chars || 0);
  const borderTone = failed ? "border-rose-300/30 bg-rose-400/8" : entry.overloaded ? "border-rose-300/25 bg-rose-400/5" : entry.shrunk ? "border-cyan-300/20 bg-cyan-300/5" : "border-white/10 bg-white/[.03]";
  return `
    <div class="rounded border ${borderTone} p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="truncate text-sm text-zinc-200">${escapeHTML(String(title))}</span>
        <span class="font-mono text-xs ${failed ? "text-rose-200" : entry.overloaded ? "text-rose-200" : "text-violet-200"}">${failed ? escapeHTML(entry.error_class || "failed") : `${utilization.toFixed(1)}%${entry.overloaded ? " overload" : ""}`}</span>
      </div>
      <div class="mt-2 flex flex-wrap items-baseline gap-2 font-mono text-sm">
        <span class="text-cyan-200">${escapeHTML(formatCompactChars(entry.sent_chars))}</span>
        <span class="text-zinc-500">/</span>
        <span class="text-zinc-400">${escapeHTML(formatCompactChars(limit))}</span>
        ${delta > 0 ? `<span class="text-[11px] text-amber-200">+${escapeHTML(formatCompactChars(delta))}</span>` : ""}
        ${entry.shrunk ? `<span class="text-[11px] text-emerald-300/90">shrunk ${Number(entry.saved_pct || 0).toFixed(0)}%</span>` : ""}
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(formatDateTime(entry.created_at))} · ${escapeHTML(entry.source || "")}${entry.model ? ` · ${escapeHTML(entry.model)}` : ""}${entry.run_id ? ` · run ${escapeHTML(String(entry.run_id).slice(0, 8))}` : ""}</div>
    </div>
  `;
}

export function renderOperationsSection(operations) {
  if (!operations) return "";
  const failureCounts = operations.failure_counts || [];
  const recentFailures = operations.recent_failures || [];
  const loopStats = operations.loop_stats || [];
  const contextFloods = operations.context_floods || [];
  const runDiagnostics = operations.run_diagnostics || [];
  const llmFailureRate = Number(operations.llm_failure_rate_pct || 0);
  const totalFailureEvents = failureCounts.reduce((sum, item) => sum + Number(item.count || 0), 0);
  const hotLoops = loopStats.filter((row) => Number(row.avg_per_run || 0) > 0 || Number(row.total_events || 0) > 0);
  return `
    <section class="rounded-lg border border-rose-300/20 bg-rose-300/5 p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-rose-200/90">Under the hood (7d)</h3>
          <p class="mt-1 text-xs text-zinc-400">Failures, retry loops, context spikes, and per-run diagnostics from worker + API telemetry.</p>
        </div>
        <div class="grid grid-cols-2 gap-2 text-right font-mono text-xs sm:grid-cols-3">
          <div><span class="text-zinc-500">failure events</span><div class="text-rose-200">${escapeHTML(String(totalFailureEvents))}</div></div>
          <div><span class="text-zinc-500">LLM fail rate</span><div class="text-rose-200/90">${llmFailureRate.toFixed(1)}%</div></div>
          <div><span class="text-zinc-500">context floods</span><div class="text-amber-200">${escapeHTML(String(contextFloods.length))}</div></div>
        </div>
      </div>
      <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(0,1fr)]">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">Failure breakdown</h4>
          <div class="mt-2 space-y-2">${failureCounts.map(renderMetricCount).join("") || emptyState("No failure telemetry yet.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-rose-300/80">Recent failures</h4>
          <div class="mt-2 max-h-[28rem] space-y-2 overflow-y-auto pr-1">${recentFailures.slice(0, 20).map(renderOperationsFailure).join("") || emptyState("No recent failure events recorded.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">Loop / retry averages</h4>
          <div class="mt-2 space-y-2">${hotLoops.map(renderOperationsLoopStat).join("") || emptyState("Loop counters appear after agent runs with retries.")}</div>
        </div>
      </div>
      <div class="mt-4 grid gap-4 xl:grid-cols-2">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-amber-300/80">Context flood spikes</h4>
          <div class="mt-2 max-h-64 space-y-2 overflow-y-auto pr-1">${contextFloods.slice(0, 12).map(renderOperationsContextFlood).join("") || emptyState("No large context deltas detected.")}</div>
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.14em] text-zinc-500">Recent run diagnostics</h4>
          <div class="mt-2 max-h-64 space-y-2 overflow-y-auto pr-1">${runDiagnostics.map(renderOperationsRunDiagnostic).join("") || emptyState("Run diagnostics populate after queue jobs execute.")}</div>
        </div>
      </div>
    </section>
  `;
}

export function renderOperationsFailure(entry) {
  return `
    <div class="rounded border border-rose-300/20 bg-rose-400/5 p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-rose-200">${escapeHTML(entry.event_type || "event")}</span>
        <span class="font-mono text-[11px] text-zinc-500">${escapeHTML(formatDateTime(entry.created_at))}</span>
      </div>
      <div class="mt-1 truncate text-sm text-zinc-300">${escapeHTML(entry.message || "no message")}</div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">${entry.run_id ? `run ${escapeHTML(String(entry.run_id).slice(0, 8))}` : "no run"}${entry.job_id ? ` · job #${escapeHTML(String(entry.job_id))}` : ""}${entry.step_id ? ` · step ${escapeHTML(String(entry.step_id))}` : ""}</div>
    </div>
  `;
}

export function renderOperationsLoopStat(row) {
  const avg = Number(row.avg_per_run || 0);
  const delta = Number(row.delta_pct || 0);
  const deltaLabel = delta > 0 ? `+${delta.toFixed(1)}% vs prior week` : delta < 0 ? `${delta.toFixed(1)}% vs prior week` : "flat vs prior week";
  const tone = avg >= 2 ? "border-amber-300/20 bg-amber-300/5" : "border-white/10 bg-white/[.03]";
  return `
    <div class="rounded border ${tone} p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="text-sm text-zinc-200">${escapeHTML(row.label || row.key || "loop")}</span>
        <span class="font-mono text-xs text-cyan-200">${avg.toFixed(2)} avg/run · max ${escapeHTML(String(row.max_per_run || 0))}</span>
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(String(row.total_events || 0))} events · ${escapeHTML(String(row.runs_affected || 0))} runs · ${escapeHTML(deltaLabel)}</div>
    </div>
  `;
}

export function renderOperationsContextFlood(entry) {
  return `
    <div class="rounded border border-amber-300/20 bg-amber-300/5 p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-zinc-200">${escapeHTML(entry.source || "llm")}${entry.scope ? ` · ${escapeHTML(entry.scope)}` : ""}</span>
        <span class="font-mono text-xs text-amber-200">+${escapeHTML(formatCompactChars(entry.delta_chars))}</span>
      </div>
      <div class="mt-1 font-mono text-sm text-zinc-300">${escapeHTML(formatCompactChars(entry.sent_chars))} sent · ${Number(entry.utilization_pct || 0).toFixed(1)}% util${entry.success === false ? ` · ${escapeHTML(entry.error_class || "failed")}` : ""}</div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(formatDateTime(entry.created_at))}${entry.model ? ` · ${escapeHTML(entry.model)}` : ""}</div>
    </div>
  `;
}

export function renderOperationsRunDiagnostic(run) {
  const tone = Number(run.failure_events || 0) > 0 ? "border-rose-300/20 bg-rose-400/5" : Number(run.loop_events || 0) > 2 ? "border-amber-300/20 bg-amber-300/5" : "border-white/10 bg-white/[.03]";
  return `
    <div class="rounded border ${tone} p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-zinc-200">${escapeHTML(run.task_kind || "run")} · ${escapeHTML(run.status || "unknown")}</span>
        <span class="font-mono text-[11px] text-zinc-500">${escapeHTML(formatDateTime(run.started_at))}</span>
      </div>
      <div class="mt-2 grid grid-cols-2 gap-2 font-mono text-[11px] text-zinc-400 sm:grid-cols-4">
        <div>loops <span class="text-amber-200">${escapeHTML(String(run.loop_events || 0))}</span></div>
        <div>fails <span class="text-rose-200">${escapeHTML(String(run.failure_events || 0))}</span></div>
        <div>llm <span class="text-cyan-200">${escapeHTML(String(run.llm_calls || 0))}</span></div>
        <div>peak <span class="text-violet-200">${escapeHTML(formatCompactChars(run.max_prompt_chars))}</span></div>
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-600">run ${escapeHTML(String(run.run_id || "").slice(0, 8))}${run.duration_ms != null ? ` · ${escapeHTML(formatDurationMS(Number(run.duration_ms)))}` : ""}</div>
    </div>
  `;
}

export function renderContextUsageDaily(point) {
  const utilization = Number(point.avg_utilization_pct || 0);
  const width = Math.max(4, Math.min(100, utilization));
  const overloads = Number(point.overload_events || 0);
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex items-center justify-between gap-2 font-mono text-xs">
        <span class="text-zinc-400">${escapeHTML(point.day || "day")}</span>
        <span class="${overloads ? "text-rose-200" : "text-violet-200"}">${utilization.toFixed(1)}% · ${escapeHTML(String(point.requests || 0))} req${overloads ? ` · ${overloads} overload` : ""}</span>
      </div>
      <div class="mt-2 h-2 overflow-hidden rounded bg-zinc-900">
        <div class="h-full rounded bg-gradient-to-r from-violet-400/70 to-rose-400/70" style="width:${width}%"></div>
      </div>
      <div class="mt-1 font-mono text-[11px] text-zinc-500">avg sent ${escapeHTML(formatCompactChars(point.avg_sent_chars))}</div>
    </div>
  `;
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

