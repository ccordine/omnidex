export const OMNI_PANELS = ["chat", "projects", "jobs", "memory", "metrics", "admin"] as const;

export type OmniPanel = (typeof OMNI_PANELS)[number];

export function isOmniPanel(value: string | null | undefined): value is OmniPanel {
  return Boolean(value && OMNI_PANELS.includes(value as OmniPanel));
}

export function parsePanelFromLocation(loc: Pick<Location, "search"> = window.location): OmniPanel {
  const param = new URLSearchParams(loc.search).get("panel");
  return isOmniPanel(param) ? param : "chat";
}

export function parseAdminTabFromLocation(loc: Pick<Location, "search"> = window.location): string {
  return new URLSearchParams(loc.search).get("admin_tab")?.trim() || "overview";
}

/** Build a same-origin path + query for the given panel (survives refresh). */
export function panelHref(panel: OmniPanel, loc: Pick<Location, "pathname" | "search" | "hash"> = window.location, extra: Record<string, string> = {}): string {
  const url = new URL(loc.pathname || "/chat", window.location.origin);
  url.hash = loc.hash;
  if (panel === "chat") {
    url.searchParams.delete("panel");
    url.searchParams.delete("admin_tab");
  } else {
    url.searchParams.set("panel", panel);
    if (panel !== "admin") {
      url.searchParams.delete("admin_tab");
    }
  }
  for (const [key, value] of Object.entries(extra)) {
    if (value) url.searchParams.set(key, value);
    else url.searchParams.delete(key);
  }
  return `${url.pathname}${url.search}${url.hash}`;
}
