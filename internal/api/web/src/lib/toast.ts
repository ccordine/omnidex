export type ToastTone = "info" | "busy" | "error" | "ok";

const toneClasses: Record<ToastTone, string> = {
  info: "border-white/15 bg-zinc-900/95 text-zinc-100",
  busy: "border-cyan-300/30 bg-cyan-950/95 text-cyan-100",
  error: "border-rose-400/35 bg-rose-950/95 text-rose-100",
  ok: "border-emerald-400/35 bg-emerald-950/95 text-emerald-100",
};

function toastRoot(): HTMLElement {
  let root = document.getElementById("omni-toast-root");
  if (!root) {
    root = document.createElement("div");
    root.id = "omni-toast-root";
    root.className = "pointer-events-none fixed inset-x-0 bottom-4 z-[60] flex flex-col items-center gap-2 px-4";
    document.body.appendChild(root);
  }
  return root;
}

export function showToast(message: string, tone: ToastTone = "info", durationMs = 5200): void {
  const text = String(message ?? "").trim();
  if (!text) return;

  const toast = document.createElement("div");
  toast.className = `omni-toast pointer-events-auto max-w-lg rounded-lg border px-4 py-3 text-sm shadow-lg backdrop-blur ${toneClasses[tone] ?? toneClasses.info}`;
  toast.setAttribute("role", tone === "error" ? "alert" : "status");
  toast.textContent = text;

  toastRoot().appendChild(toast);
  requestAnimationFrame(() => toast.classList.add("is-visible"));

  window.setTimeout(() => {
    toast.classList.remove("is-visible");
    window.setTimeout(() => toast.remove(), 220);
  }, durationMs);
}
