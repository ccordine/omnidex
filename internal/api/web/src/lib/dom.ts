export function cssEscape(value: string): string {
  if (window.CSS && typeof window.CSS.escape === "function") {
    return window.CSS.escape(String(value));
  }
  return String(value).replaceAll('"', '\\"');
}

export function badgeClass(mode: "error" | "active" | "ready"): string {
  const base = "rounded-full border px-3 py-1 text-xs font-medium";
  if (mode === "error") return `${base} border-rose-300/35 bg-rose-400/10 text-rose-100`;
  if (mode === "active") return `${base} border-cyan-300/35 bg-cyan-300/10 text-cyan-100`;
  return `${base} border-emerald-300/35 bg-emerald-300/10 text-emerald-100`;
}
