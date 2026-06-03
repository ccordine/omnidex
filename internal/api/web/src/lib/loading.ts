import { escapeHTML } from "./dom";

type SpinnerTone = "cyan" | "violet" | "zinc";
type SpinnerSize = "sm" | "md";

const GLOBAL_LOADING_KEY = "__recyclrGlobalLoadingStateV1";

function spinnerToneClass(tone: SpinnerTone): string {
  switch (tone) {
    case "violet":
      return "border-violet-300/25 border-t-violet-200";
    case "zinc":
      return "border-zinc-500/25 border-t-zinc-200";
    case "cyan":
    default:
      return "border-cyan-300/25 border-t-cyan-200";
  }
}

function spinnerSizeClass(size: SpinnerSize): string {
  return size === "sm" ? "h-3 w-3 border-2" : "h-4 w-4 border-2";
}

export function renderSpinner(tone: SpinnerTone = "cyan", size: SpinnerSize = "md", extraClass = ""): string {
  return `<span class="inline-block shrink-0 animate-spin rounded-full ${spinnerSizeClass(size)} ${spinnerToneClass(tone)} ${extraClass}" aria-hidden="true"></span>`;
}

export function renderLoadingInline(label = "Loading...", tone: SpinnerTone = "cyan"): string {
  return `<span class="inline-flex items-center gap-2 text-sm text-zinc-400" role="status" aria-live="polite">${renderSpinner(tone, "sm")}<span>${escapeHTML(label)}</span></span>`;
}

export function renderLoadingBlock(label = "Loading...", tone: SpinnerTone = "cyan"): string {
  return `<div class="flex min-h-[4rem] items-center gap-2 rounded-md border border-white/10 bg-zinc-950/40 px-3 py-3 text-sm text-zinc-400" role="status" aria-live="polite">${renderSpinner(tone)}<span>${escapeHTML(label)}</span></div>`;
}

export function renderLoadingPre(label = "Loading...", tone: SpinnerTone = "cyan"): string {
  return `<div class="flex items-center gap-2 text-sm text-zinc-400" role="status" aria-live="polite">${renderSpinner(tone, "sm")}<span>${escapeHTML(label)}</span></div>`;
}

export function renderPendingStatus(action: string, tone: SpinnerTone = "violet"): string {
  return `<span data-scrum-pending-status="${escapeHTML(action)}" class="hidden inline-flex items-center gap-1.5 text-[11px] text-zinc-500" aria-live="polite"><span class="hidden inline-block shrink-0 animate-spin rounded-full ${spinnerSizeClass("sm")} ${spinnerToneClass(tone)}" data-scrum-pending-spinner aria-hidden="true"></span><span data-scrum-pending-text></span></span>`;
}

export function setGlobalLoading(loading: boolean): void {
  const ref = window as Window & { [GLOBAL_LOADING_KEY]?: { activeCount: number } };
  const state = ref[GLOBAL_LOADING_KEY] ?? { activeCount: 0 };
  ref[GLOBAL_LOADING_KEY] = state;
  state.activeCount = loading ? state.activeCount + 1 : Math.max(0, state.activeCount - 1);

  const indicator = document.querySelector("#gx-global-loading-indicator") as HTMLElement | null;
  if (!indicator) return;
  const active = state.activeCount > 0;
  indicator.classList.toggle("hidden", !active);
  indicator.classList.toggle("flex", active);
  indicator.setAttribute("aria-hidden", active ? "false" : "true");
}
