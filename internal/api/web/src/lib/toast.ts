export type ToastTone = "info" | "busy" | "error" | "ok";

const toneClasses: Record<ToastTone, string> = {
  info: "border-white/15 bg-zinc-900/95 text-zinc-100",
  busy: "border-cyan-300/30 bg-cyan-950/95 text-cyan-100",
  error: "border-rose-400/35 bg-rose-950/95 text-rose-100",
  ok: "border-emerald-400/35 bg-emerald-950/95 text-emerald-100",
};

let hideTimer: number | null = null;
let hideToken = 0;

function requireToastSurface(): { root: HTMLElement; toast: HTMLElement } {
  const root = document.getElementById("omni-toast-root");
  const toast = document.getElementById("omni-toast");
  if (!root || !toast) throw new Error("The server-rendered toast surface is unavailable.");
  return { root, toast };
}

export function showToast(message: string, tone: ToastTone = "info", durationMs = 5200): void {
  const text = String(message ?? "").trim();
  if (!text) return;

  const { root, toast } = requireToastSurface();
  const token = ++hideToken;
  if (hideTimer != null) window.clearTimeout(hideTimer);
  hideTimer = null;

  toast.className = `omni-toast pointer-events-auto max-w-lg rounded-lg border px-4 py-3 text-sm shadow-lg backdrop-blur ${toneClasses[tone] ?? toneClasses.info}`;
  toast.setAttribute("role", tone === "error" ? "alert" : "status");
  root.setAttribute("aria-live", tone === "error" ? "assertive" : "polite");
  toast.textContent = text;
  toast.hidden = false;
  requestAnimationFrame(() => toast.classList.add("is-visible"));

  hideTimer = window.setTimeout(() => {
    if (token !== hideToken) return;
    toast.classList.remove("is-visible");
    window.setTimeout(() => {
      if (token !== hideToken) return;
      toast.hidden = true;
      toast.textContent = "";
      hideTimer = null;
    }, 220);
  }, durationMs);
}
