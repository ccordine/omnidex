import { t, tf, type MessageKey } from "./i18n";

const JOB_ACTION_MESSAGES: Readonly<Record<string, MessageKey>> = Object.freeze({
  objective_resolve: "job.action.understanding",
  v3_coding: "job.action.coding",
});

const JOB_STATUS_MESSAGES: Readonly<Record<string, MessageKey>> = Object.freeze({
  pending: "status.pending",
  running: "status.running",
  waiting_input: "status.waitingInput",
  completed: "status.completed",
  failed: "status.failed",
  canceled: "status.canceled",
});

const REALTIME_PHASE_MESSAGES: Readonly<Record<string, MessageKey>> = Object.freeze({
  queued: "job.phase.queued",
  state_changed: "job.phase.stateChanged",
  output: "job.phase.output",
  finished: "job.phase.finished",
});

export function describeChatJobProgress(details: Record<string, any>): string {
  const steps = details?.steps || [];
  const current = steps.find((step: Record<string, any>) => step.status === "running") ||
    steps.find((step: Record<string, any>) => step.status === "pending");
  if (!current?.action) return "";
  const jobID = Number(details?.job?.id);
  if (!Number.isSafeInteger(jobID) || jobID <= 0) {
    throw new Error("Job progress did not include a valid positive integer job id.");
  }
  return `${describeJobAction(current.action)} (#${jobID})`;
}

export function describeJobAction(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error("Job action must be a non-empty string.");
  }
  const action = value.trim();
  const message = JOB_ACTION_MESSAGES[action];
  if (message) return t(message);
  return tf("job.action.other", { action: action.replace(/_/g, " ") });
}

export function describeJobStatus(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error("Job status must be a non-empty string.");
  }
  const status = value.trim();
  const message = JOB_STATUS_MESSAGES[status];
  if (!message) throw new Error(`Unsupported job status ${JSON.stringify(status)}.`);
  return t(message);
}

export function describeRealtimeJobPhase(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error("Realtime job phase must be a non-empty string.");
  }
  const phase = value.trim();
  const message = REALTIME_PHASE_MESSAGES[phase];
  if (!message) throw new Error(`Unsupported realtime job phase ${JSON.stringify(phase)}.`);
  return t(message);
}
