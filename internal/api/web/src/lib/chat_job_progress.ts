import { hashText, trimText } from "./dom";
import { contextEventType } from "./render";
import type { JobContext } from "./types";

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
  const labels: Record<string, string> = {
    v3_chat_fastpath: "Replying…",
    v3_intent_parse: "Understanding request…",
    v3_capability_audit: "Checking tools…",
    v3_workspace_research: "Scanning workspace…",
    v3_memory_retrieval: "Checking memory…",
    v3_planning: "Planning…",
    v3_external_research: "Searching…",
    v3_analysis: "Analyzing…",
    v3_response_draft: "Drafting reply…",
    v3_verification: "Verifying…",
    v3_finalize: "Finishing…",
    retrieve: "Checking memory…",
    analyze: "Analyzing…",
    roleplay: "Composing reply…",
    verify: "Verifying…",
    plan: "Planning…",
    web_search: "Searching web…",
  };
  const label = labels[current.action] || `${String(current.action).replace(/_/g, " ")}…`;
  return `${label} (#${details?.job?.id || "?"})`;
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
