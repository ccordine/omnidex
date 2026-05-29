export function escapeHTML(value: unknown): string {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function cssEscape(value: string): string {
  if (window.CSS && typeof window.CSS.escape === "function") {
    return window.CSS.escape(String(value));
  }
  return String(value).replaceAll('"', '\\"');
}

export function trimText(value: unknown, max: number): string {
  const text = String(value ?? "").trim();
  return text.length > max ? `${text.slice(0, max)}...` : text;
}

export function hashText(value: unknown): string {
  const text = String(value ?? "");
  let hash = 0;
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) >>> 0;
  }
  return hash.toString(36);
}

export function formatTime(value?: string): string {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value));
}

export function formatDateTime(value?: string): string {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function formatDurationMS(value: unknown): string {
  const ms = Number(value ?? 0);
  if (!Number.isFinite(ms) || ms <= 0) return "n/a";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${Math.round(ms / 1000)}s`;
  return `${Math.round(ms / 60000)}m`;
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

export function badgeClass(mode: "error" | "active" | "ready"): string {
  const base = "rounded-full border px-3 py-1 text-xs font-medium";
  if (mode === "error") return `${base} border-rose-300/35 bg-rose-400/10 text-rose-100`;
  if (mode === "active") return `${base} border-cyan-300/35 bg-cyan-300/10 text-cyan-100`;
  return `${base} border-emerald-300/35 bg-emerald-300/10 text-emerald-100`;
}

export function statusPillClass(status?: string): string {
  const base = "rounded px-2 py-1 text-[11px] font-semibold uppercase tracking-[.14em]";
  switch (status) {
    case "completed":
    case "approved":
    case "durable":
    case "done":
    case "review":
      return `${base} bg-emerald-300/15 text-emerald-200`;
    case "running":
    case "in_progress":
      return `${base} bg-cyan-300/15 text-cyan-200`;
    case "waiting_input":
    case "pending":
    case "candidate":
    case "ready":
    case "assigned":
      return `${base} bg-amber-300/15 text-amber-200`;
    case "failed":
    case "canceled":
    case "rejected":
      return `${base} bg-rose-300/15 text-rose-200`;
    default:
      return `${base} bg-zinc-300/10 text-zinc-300`;
  }
}

export function emptyState(text: string): string {
  return `<div class="rounded-lg border border-dashed border-white/10 bg-white/[.03] p-5 text-sm leading-6 text-zinc-500">${escapeHTML(text)}</div>`;
}
