import RecyclrModule from "recyclrjs";
import { cssEscape } from "./dom";
import { setGlobalLoading } from "./loading";
import type { RecyclrRenderEvent } from "./recyclr_bundle_queue";

export type RecyclrGX = {
  render: (events: RecyclrRenderEvent[]) => void;
  history?: boolean;
};

export type RecyclrBundleHost = {
  renderBundle: (html: string) => void | Promise<void>;
};

export type RecyclrStream = {
  start: () => void;
  stop: () => void;
  isConnected: () => boolean;
  transport: () => string | null;
};

const RecyclrGXCtor = (RecyclrModule as { GX?: new (options: Record<string, unknown>) => RecyclrGX }).GX;
const createRecyclrStreamFn = (RecyclrModule as { createRecyclrStream?: (options: Record<string, unknown>) => RecyclrStream }).createRecyclrStream;

export function createRecyclrGX(): RecyclrGX {
  if (!RecyclrGXCtor) throw new Error("RecyclrJS GX is unavailable.");
  return new RecyclrGXCtor({
    url: location.href,
    method: "get",
    selection: "[data-recyclr-target]",
    history: true,
    dispatch: true,
    debug: false,
  });
}

export function createRecyclrRealtimeStream(
  onMessage: (message: Record<string, unknown>) => void,
): RecyclrStream {
  if (!createRecyclrStreamFn) throw new Error("RecyclrJS realtime transport is unavailable.");
  return createRecyclrStreamFn({
    wsUrl: "/v1/realtime/ws",
    topics: ["ui", "metrics", "scrum", "jobs"],
    heartbeatMs: 10_000,
    backoffBaseMs: 250,
    backoffMaxMs: 5_000,
    debug: false,
    onMessage,
  });
}

export type RecyclrSinkMode = "html" | "text";

/** Build a Recyclr bundle without string-concatenating untrusted HTML into a template literal. */
export function buildRecyclrBundle(target: string, html: string, location = "innerHTML"): string {
  const template = document.createElement("template");
  template.dataset.recyclrTarget = target;
  template.dataset.recyclrLocation = location;
  template.innerHTML = html;
  return template.outerHTML;
}

export async function renderRecyclrBundle(
  host: RecyclrBundleHost | null,
  target: string,
  html: string,
  mode: RecyclrSinkMode = "html",
): Promise<void> {
  if (!host || typeof host.renderBundle !== "function") {
    throw new Error("The page-scoped Recyclr controller is unavailable.");
  }
  if (!document.querySelector(`[data-recyclr-sink="${cssEscape(target)}"]`)) {
    throw new Error(`Required Recyclr sink ${JSON.stringify(target)} is unavailable.`);
  }
  const bundle = buildRecyclrBundle(target, mode === "text" ? escapeText(html) : html);
  setGlobalLoading(true);
  try {
    await host.renderBundle(bundle);
  } finally {
    setGlobalLoading(false);
  }
}

function escapeText(value: string): string {
  const node = document.createElement("span");
  node.textContent = value;
  return node.innerHTML;
}
