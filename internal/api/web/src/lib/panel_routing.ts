export const OMNI_PANELS = ["chat", "data", "projects", "jobs", "memory", "metrics", "admin"] as const;

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

export type ScrumCardTab = "card" | "files" | "tests" | "config" | "recipe" | "channel";

const SCRUM_CARD_TABS: ScrumCardTab[] = ["card", "files", "tests", "config", "recipe", "channel"];

export function isScrumCardTab(value: string | null | undefined): value is ScrumCardTab {
  return Boolean(value && SCRUM_CARD_TABS.includes(value as ScrumCardTab));
}

export function parseScrumCardFromLocation(loc: Pick<Location, "search"> = window.location): string {
  return new URLSearchParams(loc.search).get("scrum_card")?.trim() || "";
}

export function parseScrumTabFromLocation(loc: Pick<Location, "search"> = window.location): ScrumCardTab {
  const param = new URLSearchParams(loc.search).get("scrum_tab");
  return isScrumCardTab(param) ? param : "card";
}

export function parseDataSourceFromLocation(loc: Pick<Location, "search"> = window.location): string {
  return new URLSearchParams(loc.search).get("data_source")?.trim() || "";
}

export function parseDataChannelFromLocation(loc: Pick<Location, "search"> = window.location): string {
  return new URLSearchParams(loc.search).get("data_channel")?.trim() || "";
}

/** Build a same-origin path + query for the given panel (survives refresh). */
export function panelHref(panel: OmniPanel, loc: Pick<Location, "pathname" | "search" | "hash"> = window.location, extra: Record<string, string> = {}): string {
  const url = new URL(loc.pathname || "/chat", window.location.origin);
  url.hash = loc.hash;
  if (panel === "chat") {
    url.searchParams.delete("panel");
    url.searchParams.delete("admin_tab");
    url.searchParams.delete("data_source");
    url.searchParams.delete("data_channel");
  } else {
    url.searchParams.set("panel", panel);
    if (panel !== "admin") {
      url.searchParams.delete("admin_tab");
    }
    if (panel !== "data") {
      url.searchParams.delete("data_source");
      url.searchParams.delete("data_channel");
    }
  }
  for (const [key, value] of Object.entries(extra)) {
    if (value) url.searchParams.set(key, value);
    else url.searchParams.delete(key);
  }
  return `${url.pathname}${url.search}${url.hash}`;
}

export function scrumModalHref(
  cardID: string,
  tab: ScrumCardTab,
  loc: Pick<Location, "pathname" | "search" | "hash"> = window.location,
): string {
  return panelHref("projects", loc, {
    scrum_card: cardID,
    scrum_tab: tab,
  });
}

export function clearScrumModalHref(loc: Pick<Location, "pathname" | "search" | "hash"> = window.location): string {
  return panelHref("projects", loc, {
    scrum_card: "",
    scrum_tab: "",
  });
}
