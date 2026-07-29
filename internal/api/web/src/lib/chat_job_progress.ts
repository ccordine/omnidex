import { hashText, trimText } from "./dom";
import { t, tf, type MessageKey } from "./i18n";
import { contextEventType } from "./render";
import type { JobContext } from "./types";

const JOB_ACTION_MESSAGES: Readonly<Record<string, MessageKey>> = Object.freeze({
  v3_intent_parse: "job.action.understanding",
  v3_capability_audit: "job.action.checkingTools",
  v3_workspace_research: "job.action.scanningWorkspace",
  v3_memory_retrieval: "job.action.checkingMemory",
  v3_planning: "job.action.planning",
  v3_external_research: "job.action.searching",
  v3_subtask: "job.action.executing",
  v3_coding: "job.action.coding",
  v3_analysis: "job.action.analyzing",
  v3_response_draft: "job.action.drafting",
  v3_verification: "job.action.verifying",
  v3_memory_review: "job.action.reviewingMemory",
  v3_finalize: "job.action.finishing",
  retrieve: "job.action.checkingMemory",
  analyze: "job.action.analyzing",
  roleplay: "job.action.composing",
  verify: "job.action.verifying",
  plan: "job.action.planning",
  web_search: "job.action.searchingWeb",
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

export interface ChatJobProgressHost {
  seenProgress: Set<string>;
  renderProgress(details: Record<string, any>): void;
  indexContexts(contexts: JobContext[]): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  addObservedEvent(key: string, type: string, details?: Record<string, unknown>, full?: unknown): void;
}

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

export function recordChatJobProgress(host: ChatJobProgressHost, details: Record<string, any>): void {
  host.renderProgress(details);
  host.indexContexts(details.contexts || []);
  const currentStep = [...(details.steps || [])].reverse().find((step) => step.status === "running") ||
    [...(details.steps || [])].reverse().find((step) => step.status);
  const stateKey = [
    "job-state",
    details.job?.id || "unknown",
    details.job?.status || "unknown",
    currentStep?.id || "no-step",
    currentStep?.status || "unknown",
    (details.steps || []).length,
    (details.contexts || []).length,
  ].join(":");
  host.addObservedEvent(stateKey, "job_update", {
    id: details.job?.id,
    status: details.job?.status,
    action: currentStep?.action || "waiting",
    steps: (details.steps || []).length,
    contexts: (details.contexts || []).length,
  }, details);
  for (const step of details.steps || []) {
    const outputKey = `step-output:${step.id}:${hashText(step.output || "")}`;
    if (step.output && step.status !== "running" && !host.seenProgress.has(outputKey)) {
      host.seenProgress.add(outputKey);
      host.addEvent("step_output", {
        step: step.id,
        status: step.status,
        output: trimText(step.output, 280),
      }, { step });
    }
    const errorKey = `step-error:${step.id}:${hashText(step.error || "")}`;
    if (step.error && !host.seenProgress.has(errorKey)) {
      host.seenProgress.add(errorKey);
      host.addEvent("step_error", {
        step: step.id,
        status: step.status,
        error: trimText(step.error, 280),
      }, { step });
    }
  }
  for (const context of details.contexts || []) {
    const key = `context:${context.id || `${context.step_id}:${context.key}`}`;
    if (host.seenProgress.has(key)) continue;
    host.seenProgress.add(key);
    host.addEvent(contextEventType(context.key), {
      context_id: context.id,
      step: context.step_id,
      key: context.key || "context",
      value: trimText(context.value || "", 220),
    }, { job: details.job, context });
  }
}
