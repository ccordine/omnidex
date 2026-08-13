const GLOBAL_LOADING_KEY = "__recyclrGlobalLoadingStateV1";

export function setGlobalLoading(loading: boolean): void {
  const ref = window as Window & { [GLOBAL_LOADING_KEY]?: { activeCount: number } };
  const state = ref[GLOBAL_LOADING_KEY] ?? { activeCount: 0 };
  ref[GLOBAL_LOADING_KEY] = state;
  state.activeCount = loading ? state.activeCount + 1 : Math.max(0, state.activeCount - 1);

  const indicator = document.querySelector("#recyclr-global-loading-indicator") as HTMLElement | null;
  if (!indicator) return;
  const active = state.activeCount > 0;
  indicator.classList.toggle("hidden", !active);
  indicator.classList.toggle("flex", active);
  indicator.setAttribute("aria-hidden", active ? "false" : "true");
}
