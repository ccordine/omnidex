import type { StatusTone } from "./types";

const ollamaDownloadReasons = new Set([
  "queued", "running", "completed", "failed", "coordinator_error",
]);

export interface ChatOllamaDownloadHost {
  roleplayIsCurrent(): boolean;
  hasSelectedChannel(): boolean;
  setStatus(message: string, tone: StatusTone): void;
  refreshRoleplay(): Promise<void>;
  addEvent(type: string, details: Record<string, unknown>): void;
  reportError(error: unknown): void;
}

export function handleOllamaDownload(event: Event, host: ChatOllamaDownloadHost): void {
  const detail = (event as CustomEvent<Record<string, unknown>>).detail;
  if (!detail || typeof detail !== "object" || Array.isArray(detail)) {
    throw new Error("Ollama download event is missing typed detail.");
  }
  const reason = String(detail.reason ?? "").trim();
  if (!ollamaDownloadReasons.has(reason)) {
    throw new Error(`Ollama download event has unregistered reason ${JSON.stringify(reason)}.`);
  }
  if (!host.roleplayIsCurrent()) return;
  const summary = String(detail.summary ?? "").trim();
  if (!summary) throw new Error("Ollama download event is missing its status summary.");
  host.setStatus(
    summary,
    reason === "failed" || reason === "coordinator_error"
      ? "error"
      : reason === "completed" ? "ready" : "active",
  );
  if (reason !== "completed" || !host.hasSelectedChannel()) return;
  void host.refreshRoleplay().catch((error) => {
    host.addEvent("roleplay_models_refresh_failed", {
      error: error instanceof Error ? error.message : String(error),
    });
    host.reportError(error);
  });
}
