import { SCRUM_COLUMNS, type ScrumColumn } from "./scrum_types";

export function requireScrumViewportColumn(value: unknown, label: string): ScrumColumn {
  if (typeof value !== "string" || !SCRUM_COLUMNS.includes(value as ScrumColumn)) {
    throw new Error(`${label} must be one exact registered Scrum column.`);
  }
  return value as ScrumColumn;
}

function columnSessionKey(projectID: number): string {
  if (!Number.isSafeInteger(projectID) || projectID <= 0) {
    throw new Error("Scrum viewport state requires one positive safe-integer project ID.");
  }
  return `omni.scrum.column.${projectID}`;
}

export function resolveScrumViewportColumn(projectID: number, current: ScrumColumn): ScrumColumn {
  const fromURL = new URLSearchParams(window.location.search).get("scrum_column");
  if (fromURL !== null) return requireScrumViewportColumn(fromURL, "Scrum viewport URL column");
  const stored = sessionStorage.getItem(columnSessionKey(projectID));
  if (stored !== null) return requireScrumViewportColumn(stored, "Scrum viewport session column");
  return current;
}

export function persistScrumViewportColumn(projectID: number, column: unknown, updateURL = true): ScrumColumn {
  const next = requireScrumViewportColumn(column, "Scrum viewport column");
  sessionStorage.setItem(columnSessionKey(projectID), next);
  if (updateURL) {
    const url = new URL(window.location.href);
    url.searchParams.set("scrum_column", next);
    history.replaceState(null, document.title, `${url.pathname}${url.search}${url.hash}`);
  }
  return next;
}
