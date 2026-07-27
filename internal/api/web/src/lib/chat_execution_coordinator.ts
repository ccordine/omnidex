import { readJSON } from "./api";
import { describeChatJobProgress } from "./chat_job_progress";
import { debounce } from "./main_thread";
import type { OmniPanel } from "./panel_routing";

type JobRole = "assistant" | "error";
const JOB_STATUSES = ["pending", "running", "waiting_input", "completed", "failed", "canceled"] as const;
type JobStatus = (typeof JOB_STATUSES)[number];
type JobCompletion = {
  jobID: number;
  resolve: () => void;
  reject: (error: unknown) => void;
};

interface JobRecord {
  id: number;
  status: JobStatus;
  result?: unknown;
  error?: unknown;
}

interface JobDetails {
  job: JobRecord;
  steps: Array<Record<string, unknown>>;
  contexts: Array<Record<string, unknown>>;
}

export interface ChatExecutionHost {
  openedProjectID(): number | null;
  openedProjectLocation(): string | null;
  currentPanel(): OmniPanel;
  hasJobBadge(): boolean;
  jobBadge(): HTMLElement;
  setActivityLabel(label: string): void;
  setStatus(text: string, mode: string): void;
  renderProgressActivity(label: string): void;
  recordJobProgress(details: Record<string, any>): void;
  renderMessages(): void;
  renderJobDetails(details: Record<string, any>): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  addMessage(role: JobRole, content: string): void;
  setBusy(value: boolean): void;
  loadJobs(options: { quiet?: boolean; strict?: boolean }): Promise<void>;
  loadGlobalActivity(options: { quiet?: boolean; strict?: boolean }): Promise<void>;
  reportError(error: unknown): void;
}

export class ChatExecutionCoordinator {
  private jobID: number | null = null;
  private pending: JobCompletion | null = null;
  private refreshPromise: Promise<void> | null = null;
  private refreshRequested = false;
  private lastSignature = "";

  private readonly scheduleJobsPanelRefresh = debounce(() => {
    if (this.host.currentPanel() !== "jobs") return;
    void this.host.loadJobs({ quiet: true, strict: true }).catch((error) => {
      this.reportBackgroundFailure("jobs_panel_refresh", error);
    });
  }, 120);

  private readonly scheduleCurrentJobRefresh = debounce((jobID: number) => {
    void this.refresh(jobID).catch((error) => this.failRefresh(jobID, error));
  }, 75);

  constructor(private readonly host: ChatExecutionHost) {}

  currentJobID(): number | null {
    return this.jobID;
  }

  setCurrentJobID(value: number | string | null): void {
    if (value === null) {
      this.jobID = null;
      if (this.host.hasJobBadge()) this.host.jobBadge().textContent = "none";
      return;
    }
    const jobID = positiveJobID(value, "Selected job");
    this.jobID = jobID;
    if (this.host.hasJobBadge()) this.host.jobBadge().textContent = `#${jobID}`;
  }

  async submit(prompt: string): Promise<void> {
    const instruction = prompt.trim();
    if (!instruction) throw new Error("A non-empty job instruction is required.");
    if (this.pending) throw new Error(`Job #${this.pending.jobID} is already active in this chat.`);

    this.host.setActivityLabel("Queuing job…");
    this.host.setStatus("queuing", "active");
    this.host.renderProgressActivity("Queuing job…");
    const metadata: Record<string, unknown> = {
      source: "omni-web-chat",
      ui: "stimulus-tailwind-recyclr",
    };
    const projectID = this.host.openedProjectID();
    if (projectID !== null) metadata.project_id = projectID;
    const location = this.host.openedProjectLocation();
    if (location !== null) {
      metadata.client_cwd = location;
      metadata.project_directory = location;
    }
    const request = { instruction, pipeline: "chat", metadata };
    const payload = await readJSON<Record<string, any>>(await fetch("/v1/jobs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    }));
    const job = requireJobRecord(payload.job, "Queued job response");
    this.setCurrentJobID(job.id);
    const label = `Running job #${job.id}…`;
    this.host.setActivityLabel(label);
    this.host.renderProgressActivity(label);
    this.host.addEvent("job_created", { id: job.id, status: job.status }, { request, response: payload, job });
    await this.waitForJob(job.id);
  }

  handleProgress(event: Event): void {
    const detail = (event as CustomEvent<{ jobID?: unknown; phase?: unknown; summary?: unknown }>).detail;
    const jobID = positiveJobID(detail?.jobID, "Realtime job update");
    if (this.jobID === jobID) {
      const summary = typeof detail?.summary === "string" ? detail.summary.trim() : "";
      if (summary && this.pending?.jobID === jobID) {
        this.host.setActivityLabel(summary);
        this.host.renderProgressActivity(summary);
      }
      this.scheduleCurrentJobRefresh(jobID);
    }
    this.scheduleJobsPanelRefresh();
    if (detail?.phase === "finished") {
      void this.host.loadGlobalActivity({ quiet: true, strict: true }).catch((error) => {
        this.reportBackgroundFailure("global_activity_refresh", error);
      });
    }
  }

  async refreshCurrent(): Promise<void> {
    if (this.jobID === null) throw new Error("Cannot refresh a job because no current job is selected.");
    await this.refresh(this.jobID);
  }

  disconnect(): void {
    if (!this.pending) return;
    const pending = this.pending;
    this.pending = null;
    pending.reject(new Error("Chat controller disconnected before the active job completed."));
  }

  private waitForJob(jobID: number): Promise<void> {
    if (this.pending) {
      return Promise.reject(new Error(`Job #${this.pending.jobID} is already active in this chat.`));
    }
    this.host.setStatus("running", "active");
    this.lastSignature = "";
    return new Promise<void>((resolve, reject) => {
      this.pending = { jobID, resolve, reject };
      void this.refresh(jobID).catch((error) => this.failRefresh(jobID, error));
    });
  }

  private async refresh(jobID: number): Promise<void> {
    positiveJobID(jobID, "Job refresh");
    if (this.refreshPromise) {
      this.refreshRequested = true;
      await this.refreshPromise;
      return;
    }
    const operation = this.refreshLoop(jobID);
    this.refreshPromise = operation;
    try {
      await operation;
    } finally {
      if (this.refreshPromise === operation) this.refreshPromise = null;
    }
  }

  private async refreshLoop(jobID: number): Promise<void> {
    do {
      this.refreshRequested = false;
      await this.reconcile(jobID);
    } while (this.refreshRequested);
  }

  private async reconcile(jobID: number): Promise<void> {
    const payload = await readJSON<Record<string, any>>(await fetch(`/v1/jobs/${jobID}`));
    const details = requireJobDetails(payload, jobID);
    const signature = JSON.stringify({
      status: details.job.status,
      result: details.job.result,
      error: details.job.error,
      steps: details.steps.map((step) => [step.id, step.status, step.output, step.error]),
      contexts: details.contexts.length,
    });
    if (signature !== this.lastSignature) {
      const stepLabel = describeChatJobProgress(details);
      const label = stepLabel || `Running job #${jobID} · ${details.job.status}…`;
      this.host.setActivityLabel(label);
      if (this.pending?.jobID === jobID) {
        this.host.renderProgressActivity(label);
        this.host.recordJobProgress(details);
        this.host.renderMessages();
      }
      if (this.host.currentPanel() === "jobs" && this.jobID === jobID) {
        this.host.renderJobDetails(details);
      }
      this.lastSignature = signature;
    }

    if (details.job.status === "completed") {
      this.finish(jobID, "assistant", requiredMessage(details.job.result, `Completed job #${jobID} did not include a result.`), "completed", "ready");
    } else if (details.job.status === "failed" || details.job.status === "canceled") {
      this.finish(jobID, "error", requiredMessage(details.job.error, `Job #${jobID} entered ${details.job.status} without an error message.`), details.job.status, "error");
    }
  }

  private finish(jobID: number, role: JobRole, message: string, status: string, tone: string): void {
    const pending = this.pending;
    if (!pending || pending.jobID !== jobID) return;
    this.pending = null;
    this.host.addMessage(role, message);
    this.host.setStatus(status, tone);
    this.host.setBusy(false);
    pending.resolve();
  }

  private failRefresh(jobID: number, error: unknown): void {
    console.error(`Failed to reconcile job #${jobID}`, error);
    const pending = this.pending;
    if (pending?.jobID === jobID) {
      this.pending = null;
      pending.reject(error);
      return;
    }
    this.reportBackgroundFailure("job_refresh", error);
  }

  private reportBackgroundFailure(operation: string, error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`Chat ${operation} failed`, error);
    this.host.addEvent("realtime_refresh_failed", { operation, error: message });
    this.host.reportError(error);
  }
}

function positiveJobID(value: unknown, source: string): number {
  const jobID = typeof value === "number" || typeof value === "string" ? Number(value) : Number.NaN;
  if (!Number.isSafeInteger(jobID) || jobID <= 0) {
    throw new Error(`${source} did not include a valid positive integer id.`);
  }
  return jobID;
}

function requireJobRecord(value: unknown, source: string): JobRecord {
  if (!value || typeof value !== "object") throw new Error(`${source} did not include a job record.`);
  const raw = value as Record<string, unknown>;
  const id = positiveJobID(raw.id, source);
  if (!isJobStatus(raw.status)) {
    throw new Error(`${source} job #${id} has invalid status ${JSON.stringify(raw.status)}.`);
  }
  return { id, status: raw.status, result: raw.result, error: raw.error };
}

function isJobStatus(value: unknown): value is JobStatus {
  return typeof value === "string" && JOB_STATUSES.includes(value as JobStatus);
}

function requireJobDetails(value: Record<string, any>, requestedJobID: number): JobDetails {
  const job = requireJobRecord(value.job, `Job #${requestedJobID} response`);
  if (job.id !== requestedJobID) {
    throw new Error(`Job refresh requested #${requestedJobID} but the server returned #${job.id}.`);
  }
  if (!Array.isArray(value.steps)) throw new Error(`Job #${requestedJobID} response did not include a steps array.`);
  if (!Array.isArray(value.contexts)) throw new Error(`Job #${requestedJobID} response did not include a contexts array.`);
  return { job, steps: value.steps, contexts: value.contexts };
}

function requiredMessage(value: unknown, error: string): string {
  if (typeof value !== "string" || !value.trim()) throw new Error(error);
  return value;
}
