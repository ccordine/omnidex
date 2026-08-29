import { readJSON } from "./api";
import {
  describeChatJobProgress,
  describeJobStatus,
  describeRealtimeJobPhase,
} from "./chat_job_progress";
import {
  positiveJobID,
  requireJobDetails,
  requiredMessage,
} from "./chat_execution_contract";
import { requireServerComponentBundle } from "./chat_component_api";
import { t, tf } from "./i18n";
import { debounce } from "./main_thread";
import type { OmniPanel } from "./panel_routing";
import type { StatusTone } from "./types";

type JobCompletion = {
  jobID: number;
  resolve: () => void;
  reject: (error: unknown) => void;
};

const authoritativeReconcileDelayMilliseconds = 750;

export interface ChatExecutionHost {
  currentPanel(): OmniPanel;
  hasJobBadge(): boolean;
  jobBadge(): HTMLElement;
  setActivityLabel(label: string): void;
  setStatus(text: string, mode: StatusTone): void;
  renderProgressActivity(label: string): void;
  renderJobState(bundle: string): Promise<void>;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  loadJobs(options: { quiet?: boolean; strict?: boolean }): Promise<void>;
  loadGlobalActivity(options: { quiet?: boolean; strict?: boolean }): Promise<void>;
  reportError(error: unknown): void;
}

export class ChatExecutionCoordinator {
  private jobID: number | null = null;
  private pending: JobCompletion | null = null;
  private refreshPromise: Promise<void> | null = null;
  private refreshRequested = false;
  private authoritativeRefreshTimer: number | null = null;
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
      if (this.host.hasJobBadge()) this.host.jobBadge().textContent = t("job.none");
      return;
    }
    const jobID = positiveJobID(value, "Selected job");
    this.jobID = jobID;
    if (this.host.hasJobBadge()) this.host.jobBadge().textContent = `#${jobID}`;
  }

  async waitForExistingJob(value: number): Promise<void> {
    const jobID = positiveJobID(value, "Existing channel job");
    if (this.pending) throw new Error(`Job #${this.pending.jobID} is already active in this chat.`);
    this.setCurrentJobID(jobID);
    const label = tf("job.running", { id: jobID });
    this.host.setActivityLabel(label);
    this.host.renderProgressActivity(label);
    await this.waitForJob(jobID);
  }

  handleProgress(event: Event): void {
    const detail = (event as CustomEvent<{ jobID?: unknown; phase?: unknown; summary?: unknown }>).detail;
    const jobID = positiveJobID(detail?.jobID, "Realtime job update");
    const phaseLabel = describeRealtimeJobPhase(detail?.phase);
    if (this.jobID === jobID) {
      if (this.pending?.jobID === jobID) {
        this.host.setActivityLabel(phaseLabel);
        this.host.renderProgressActivity(phaseLabel);
        this.clearAuthoritativeRefresh();
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
    this.clearAuthoritativeRefresh();
    if (!this.pending) return;
    const pending = this.pending;
    this.pending = null;
    pending.reject(new Error("Chat controller disconnected before the active job completed."));
  }

  private waitForJob(jobID: number): Promise<void> {
    if (this.pending) {
      return Promise.reject(new Error(`Job #${this.pending.jobID} is already active in this chat.`));
    }
    this.host.setStatus(t("status.running"), "active");
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
    const payload = await readJSON<Record<string, any>>(await fetch(`/v1/ui/chat/jobs/${jobID}`));
    const details = requireJobDetails(payload, jobID);
    const jobStateBundle = requireServerComponentBundle(payload, `Job #${jobID} state`);
    const signature = JSON.stringify({
      status: details.job.status,
      result: details.job.result,
      error: details.job.error,
      steps: details.steps.map((step) => [step.id, step.action, step.status, step.generation]),
      generation: details.job.current_generation,
      progress: [details.progress.latest_context_id, details.progress.count],
      jobStateBundle,
    });
    if (signature !== this.lastSignature) {
      const stepLabel = describeChatJobProgress(details);
      const label = stepLabel || tf("job.runningStatus", {
        id: jobID,
        status: describeJobStatus(details.job.status),
      });
      this.host.setActivityLabel(label);
      if (this.pending?.jobID === jobID) {
        this.host.renderProgressActivity(label);
      }
      await this.host.renderJobState(jobStateBundle);
      this.lastSignature = signature;
    }

    if (details.job.status === "completed") {
      this.finishCompleted(jobID);
    } else if (details.job.status === "failed" || details.job.status === "canceled") {
      this.finishFailed(jobID, details.job.status, details.job.error);
    } else if (this.pending?.jobID === jobID) {
      this.scheduleAuthoritativeRefresh(jobID);
    }
  }

  private scheduleAuthoritativeRefresh(jobID: number): void {
    this.clearAuthoritativeRefresh();
    this.authoritativeRefreshTimer = window.setTimeout(() => {
      this.authoritativeRefreshTimer = null;
      if (this.pending?.jobID !== jobID) return;
      void this.refresh(jobID).catch((error) => this.failRefresh(jobID, error));
    }, authoritativeReconcileDelayMilliseconds);
  }

  private clearAuthoritativeRefresh(): void {
    if (this.authoritativeRefreshTimer === null) return;
    window.clearTimeout(this.authoritativeRefreshTimer);
    this.authoritativeRefreshTimer = null;
  }

  private finishCompleted(jobID: number): void {
    const pending = this.pending;
    if (!pending || pending.jobID !== jobID) return;
    this.clearAuthoritativeRefresh();
    this.pending = null;
    this.host.setStatus(describeJobStatus("completed"), "ready");
    pending.resolve();
  }

  private finishFailed(jobID: number, status: "failed" | "canceled", rawError: unknown): void {
    const pending = this.pending;
    if (!pending || pending.jobID !== jobID) return;
    const message = requiredMessage(rawError, `Job #${jobID} entered ${status} without an error message.`);
    this.clearAuthoritativeRefresh();
    this.pending = null;
    this.host.setStatus(describeJobStatus(status), "error");
    pending.reject(new Error(message));
  }

  private failRefresh(jobID: number, error: unknown): void {
    console.error(`Failed to reconcile job #${jobID}`, error);
    const pending = this.pending;
    if (pending?.jobID === jobID) {
      this.clearAuthoritativeRefresh();
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
