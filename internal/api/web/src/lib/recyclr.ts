import RecyclrModule from "recyclrjs";
import { cssEscape } from "./dom";

type RecyclrEvent = {
  selector: string;
  location: string;
  selection: string;
};

type RecyclrGX = {
  render: (events: RecyclrEvent[]) => void;
  history?: boolean;
  consumeRealtime?: (message: Record<string, unknown>) => void | Promise<void>;
};

type RecyclrStream = {
  start: () => void;
  stop: () => void;
};

const RecyclrGXCtor = (RecyclrModule as { GX?: new (options: Record<string, unknown>) => RecyclrGX }).GX;
const createRecyclrStreamFn = (RecyclrModule as { createRecyclrStream?: (options: Record<string, unknown>) => RecyclrStream }).createRecyclrStream;

export function createRecyclrGX(): RecyclrGX | null {
  if (!RecyclrGXCtor) return null;
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
  gx: RecyclrGX,
  onMessage?: (message: Record<string, unknown>) => void,
): RecyclrStream | null {
  if (!createRecyclrStreamFn) return null;
  return createRecyclrStreamFn({
    wsUrl: "/v1/realtime/ws",
    sseUrl: "/v1/realtime/sse",
    topics: ["ui", "metrics"],
    gx,
    debug: false,
    onMessage,
  });
}

export function renderRecyclrBundle(host: { renderBundle: (html: string) => void } | null, target: string, html: string): void {
  const bundle = `<template data-recyclr-target="${cssEscape(target)}">${html}</template>`;
  if (host && typeof host.renderBundle === "function") {
    host.renderBundle(bundle);
    return;
  }
  const sink = document.querySelector(`[data-recyclr-sink="${cssEscape(target)}"]`);
  if (sink) sink.innerHTML = html;
}
