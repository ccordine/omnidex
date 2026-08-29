import { readJSON, SCRUM_CHANNEL_RESPONSE_MAX_BYTES } from "./api";
import { projectQuery } from "./project_api";
import {
  validateScrumCardFilePageResponse,
  validateScrumCardModalResponse,
} from "./scrum_response_authority";
import type { ScrumCardFilePage, ScrumCardModalResponse } from "./scrum_types";

const SCRUM_MODAL_TABS = ["card", "files", "tests", "channel"] as const;
type ScrumModalTab = (typeof SCRUM_MODAL_TABS)[number];
type ScrumModalOptions =
  | { tab: "files"; filePath: string; fileOffset: number }
  | { tab: Exclude<ScrumModalTab, "files"> };
const MAX_SCRUM_FILE_PATH_BYTES = 4096;
const MAX_SCRUM_FILE_OFFSET = 1_000_000;

function requireScrumModalTab(tab: unknown): ScrumModalTab {
  if (typeof tab !== "string" || !SCRUM_MODAL_TABS.includes(tab as ScrumModalTab)) {
    throw new Error("Scrum card modal tab must be one exact registered value.");
  }
  return tab as ScrumModalTab;
}

function requireScrumFilePath(value: unknown): string {
  if (typeof value !== "string" || value.includes("\0")) {
    throw new Error("Scrum card file path must be a string without NUL.");
  }
  const encoded = new TextEncoder().encode(value);
  if (new TextDecoder().decode(encoded) !== value) {
    throw new Error("Scrum card file path must be valid Unicode.");
  }
  if (encoded.byteLength > MAX_SCRUM_FILE_PATH_BYTES) {
    throw new Error(`Scrum card file path exceeds ${MAX_SCRUM_FILE_PATH_BYTES} UTF-8 bytes.`);
  }
  if (value === "") return value;
  const segments = value.split("/");
  if (value.startsWith("/") || value.endsWith("/") || value.includes("\\") ||
      /^[A-Za-z]:\//.test(value) || segments.some((segment) => !segment || segment === "." || segment === "..")) {
    throw new Error("Scrum card file path must be an exact canonical relative path or empty root.");
  }
  return value;
}

function requireScrumFileOffset(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0 || value > MAX_SCRUM_FILE_OFFSET) {
    throw new Error(`Scrum card file offset must be an integer between 0 and ${MAX_SCRUM_FILE_OFFSET}.`);
  }
  return value;
}

export async function fetchScrumCardModal(
  cardID: string,
  projectID: number,
  options: ScrumModalOptions = { tab: "card" },
): Promise<ScrumCardModalResponse> {
  const query = new URLSearchParams(projectQuery(projectID).slice(1));
  const tab = requireScrumModalTab(options.tab);
  query.set("tab", tab);
  if (tab === "files") {
    if (!("filePath" in options) || !("fileOffset" in options)) {
      throw new Error("Scrum card modal files tab requires explicit root path and page offset authority.");
    }
    query.set("file_path", requireScrumFilePath(options.filePath));
    query.set("file_offset", String(requireScrumFileOffset(options.fileOffset)));
  } else if ("filePath" in options || "fileOffset" in options) {
    throw new Error("Scrum card modal file state is accepted only for the files tab.");
  }
  const suffix = query.toString() ? `?${query.toString()}` : "";
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}/modal${suffix}`);
  return validateScrumCardModalResponse(
    await readJSON<unknown>(response, SCRUM_CHANNEL_RESPONSE_MAX_BYTES), cardID, projectID, tab,
    tab === "files" && "filePath" in options ? options.filePath : "",
    tab === "files" && "fileOffset" in options ? options.fileOffset : 0,
  );
}

export async function fetchScrumCardFilePage(
  cardID: string,
  projectID: number,
  path: string,
  offset: number,
): Promise<ScrumCardFilePage> {
  const query = new URLSearchParams(projectQuery(projectID).slice(1));
  query.set("file_path", requireScrumFilePath(path));
  query.set("file_offset", String(requireScrumFileOffset(offset)));
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}/files?${query.toString()}`);
  return validateScrumCardFilePageResponse(await readJSON<unknown>(response), path, offset);
}
