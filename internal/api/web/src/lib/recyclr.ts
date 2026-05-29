import RecyclrModule from "recyclrjs";
import { cssEscape } from "./dom";

type RecyclrEvent = {
  selector: string;
  location: string;
  selection: string;
};

type RecyclrGX = {
  render: (events: RecyclrEvent[]) => void;
};

const RecyclrGXCtor = (RecyclrModule as { GX?: new (options: Record<string, unknown>) => RecyclrGX }).GX;

export function createRecyclrGX(): RecyclrGX | null {
  if (!RecyclrGXCtor) return null;
  return new RecyclrGXCtor({
    url: location.href,
    method: "get",
    selection: "[data-recyclr-target]",
    history: false,
    dispatch: true,
    debug: false,
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
