import { jsonRequest, readJSON } from "./api";
import { fetchChatJobsPage, requireServerComponentBundle } from "./chat_component_api";
import { requireJobDetails } from "./chat_execution_contract";
import { LifecycleOperationAttempt } from "./lifecycle_operation";

export interface ChatJobsHost {
  queueEnabled(): boolean;
  jobFilter(): HTMLSelectElement;
  hasJobBadge(): boolean;
  jobBadge(): HTMLElement;
  setCurrentJobID(id: number | string | null): void;
  renderComponentBundle(bundle: string): Promise<void>;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
}

export function authoritativeControlJobID(payload: unknown, expectedJobID: string, expectedOperationID: string): string {
  const expectedID = Number(expectedJobID);
  if (!Number.isSafeInteger(expectedID) || expectedID <= 0) {
    throw new Error("Job control requires a valid expected job id.");
  }
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) throw new Error("Job control response must be one typed object.");
  const receipt = payload as Record<string, unknown>;
  const keys = Object.keys(receipt);
  if (keys.length !== 3 || !keys.includes("job_id") || !keys.includes("operation_id") || !keys.includes("status")) {
    throw new Error("Job control response must contain only the exact authoritative receipt fields.");
  }
  const id = receipt.job_id;
  if (typeof id !== "number" || !Number.isSafeInteger(id) || id <= 0) {
    throw new Error("Job control response contains an invalid authoritative job id.");
  }
  if (id !== expectedID) {
    throw new Error(`Job control expected job ${expectedID}, but the server returned job ${id}.`);
  }
  if (typeof receipt.operation_id !== "string" || receipt.operation_id !== expectedOperationID ||
      !/^lifecycle_operation_[0-9a-f]{64}$/.test(receipt.operation_id)) {
    throw new Error("Job control response does not attest the submitted lifecycle operation.");
  }
  if (typeof receipt.status !== "string" ||
      !["pending", "running", "completed", "failed", "canceled", "waiting_input"].includes(receipt.status)) {
    throw new Error("Job control response contains an unregistered authoritative job status.");
  }
  return String(id);
}

export class ChatJobsCoordinator {
  private readonly lifecycleOperationAttempt = new LifecycleOperationAttempt();

  constructor(private readonly host: ChatJobsHost) {}

  async load(options: { quiet?: boolean; strict?: boolean; offset?: number } = {}): Promise<void> {
    if (!this.host.queueEnabled()) throw new Error("Job components require repository mode.");
    const offset = options.offset ?? 0;
    try {
      const page = await fetchChatJobsPage(this.host.jobFilter().value, offset);
      await this.host.renderComponentBundle(page.html.bundle);
      this.host.addEvent("jobs_loaded", {
        status: this.host.jobFilter().value || "all",
        next_offset: page.next_offset ?? 0,
        has_more: page.has_more,
      });
    } catch (error) {
      if (!options.quiet) this.host.addEvent("jobs_failed", { error: errorMessage(error) });
      if (options.strict) throw error;
    }
  }

  async loadMore(event: Event): Promise<void> {
    const button = requirePageButton(event, "jobs");
    await withButtonFeedback(button, "Loading jobs…", () => this.load({
      strict: true,
      offset: Number(button.dataset.nextOffset),
    }));
  }

  async select(event: Event): Promise<void> {
    const id = jobIDFromEvent(event);
    const details = await readJSON<Record<string, unknown>>(await fetch(`/v1/ui/chat/jobs/${id}`));
    const jobID = await this.renderDetails(details, Number(id));
    this.host.setCurrentJobID(jobID);
    if (this.host.hasJobBadge()) this.host.jobBadge().textContent = `#${jobID}`;
  }

  async renderDetails(details: Record<string, unknown>, expectedJobID: number): Promise<number> {
    const authoritative = requireJobDetails(details, expectedJobID);
    await this.host.renderComponentBundle(requireServerComponentBundle(details, "Job details"));
    return authoritative.job.id;
  }

  async interrupt(event: Event): Promise<void> {
    await this.promptedControl(event, "interrupt", "Interrupt with what instruction?", "Interrupting job…");
  }

  async replan(event: Event): Promise<void> {
    await this.promptedControl(event, "replan", "What should Omni change in the plan?", "Replanning job…");
  }

  async cancel(event: Event): Promise<void> {
    const reason = window.prompt("Cancel reason?", "Canceled from Omni UI");
    if (reason === null || !reason.trim()) return;
    const id = jobIDFromEvent(event);
    const button = jobControlButton(event);
    await withButtonFeedback(button, "Canceling job…", async () => {
      const attemptKey = { scope: id, action: "cancel", content: reason };
      const operationID = this.lifecycleOperationAttempt.acquire(attemptKey);
      const control = await readJSON(await fetch(
        `/v1/jobs/${id}/cancel`,
        jsonRequest({ operation_id: operationID, reason }),
      ));
      const authoritativeID = authoritativeControlJobID(control, id, operationID);
      this.lifecycleOperationAttempt.confirm(attemptKey, operationID);
      await this.load({ strict: true });
      this.host.addEvent("job_canceled", { id: authoritativeID });
    });
  }

  private async promptedControl(
    event: Event,
    action: "interrupt" | "replan",
    question: string,
    loading: string,
  ): Promise<void> {
    const feedback = window.prompt(question);
    if (feedback === null || !feedback.trim()) return;
    const id = jobIDFromEvent(event);
    await withButtonFeedback(jobControlButton(event), loading, () => this.postControl(id, action, feedback));
  }

  private async postControl(id: string, action: "interrupt" | "replan", feedback: string): Promise<void> {
    const attemptKey = { scope: id, action, content: feedback };
    const operationID = this.lifecycleOperationAttempt.acquire(attemptKey);
    const control = await readJSON(await fetch(
      `/v1/jobs/${id}/${action}`,
      jsonRequest({ operation_id: operationID, feedback }),
    ));
    const authoritativeID = authoritativeControlJobID(control, id, operationID);
    this.lifecycleOperationAttempt.confirm(attemptKey, operationID);
    const details = await readJSON<Record<string, unknown>>(await fetch(`/v1/ui/chat/jobs/${authoritativeID}`));
    await this.renderDetails(details, Number(authoritativeID));
    this.host.addEvent(`job_${action}`, { id: authoritativeID });
  }
}

function jobIDFromEvent(event: Event): string {
  const id = (event.currentTarget as HTMLElement).dataset.jobId;
  if (!id || !/^[1-9][0-9]*$/.test(id)) throw new Error("Job control is missing its canonical job id.");
  return id;
}

function jobControlButton(event: Event): HTMLButtonElement {
  const button = event.currentTarget;
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error("Job control requires an authoritative server-rendered button.");
  }
  return button;
}

function requirePageButton(event: Event, section: string): HTMLButtonElement {
  const button = event.currentTarget as HTMLButtonElement;
  const offset = Number(button.dataset.nextOffset ?? "");
  if (button.dataset.pageSection !== section || !Number.isSafeInteger(offset) || offset < 1) {
    throw new Error(`The server-rendered ${section} page cursor is invalid.`);
  }
  return button;
}

async function withButtonFeedback(
  button: HTMLButtonElement,
  loading: string,
  operation: () => Promise<void>,
): Promise<void> {
  const label = button.textContent;
  button.disabled = true;
  button.setAttribute("aria-busy", "true");
  button.textContent = loading;
  try {
    await operation();
  } finally {
    button.disabled = false;
    button.setAttribute("aria-busy", "false");
    button.textContent = label;
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
