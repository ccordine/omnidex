import RecyclrModule from "recyclrjs";
import type { RecyclrRenderEvent } from "./recyclr_bundle_queue";

export type RecyclrGX = {
  render: (events: RecyclrRenderEvent[]) => void;
  history?: boolean;
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
