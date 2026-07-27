import { fetchJobRecord, type JobRecord } from "./data_api";
import { fetchProjectDebuggerStatus, runProjectDebugger } from "./project_api";
import { renderProjectDebuggerModal } from "./project_debugger_render";
import { observeRealtimeJob, type RealtimeJobObservation } from "./realtime_job_observer";
import type { ResolvedAgentConfig } from "./agent_config_types";
import type { DebuggerLastRun } from "./project_types";
import type { ProjectStatusTone } from "./project_browser_coordinator";

type DebuggerStatus = Awaited<ReturnType<typeof fetchProjectDebuggerStatus>>;

export interface ProjectDebuggerHost {
  selectedProjectID(): number | null;
  activeTab(): string;
  projectName(projectID: number): string;
  agentConfig(): ResolvedAgentConfig | null;
  modalPanel(): HTMLElement | null;
  openModal(html: string): Promise<void>;
  closeModal(): void;
  setStatus(message: string, tone?: ProjectStatusTone): void;
  actionOk(message: string): void;
  actionFail(error: unknown): void;
  refreshScrum(projectID: number): void;
}

export class ProjectDebuggerCoordinator {
  private projectID: number | null = null;
  private projectName = "";
  private lastRun: DebuggerLastRun | null = null;
  private running = false;
  private observation: RealtimeJobObservation<{ job: JobRecord; status: DebuggerStatus }> | null = null;

  constructor(private readonly host: ProjectDebuggerHost) {}

  disconnect(): void {
    this.stopObservation("Projects controller disconnected.");
  }

  async open(event: Event): Promise<void> {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || this.host.selectedProjectID() || 0);
    if (!id) throw new Error("A project is required before codebase analysis can open.");
    this.projectID = id;
    this.projectName = this.host.projectName(id);
    this.host.setStatus("Loading analysis…", "busy");
    try {
      const payload = await fetchProjectDebuggerStatus(id);
      this.lastRun = payload.last_run ?? null;
      this.running = payload.last_run?.status === "running";
      await this.host.openModal(this.modalHTML(payload.agent_config));
      if (this.running) {
        const jobID = payload.last_run?.job_id;
        if (!jobID) throw new Error("Running project analysis did not include its job id.");
        this.startObservation(id, jobID);
      }
      this.host.setStatus(this.projectName, "ok");
    } catch (error) {
      this.host.actionFail(error);
    }
  }

  close(): void {
    this.stopObservation("Project analysis modal closed.");
    this.host.closeModal();
  }

  async run(event: Event): Promise<void> {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || this.projectID || 0);
    if (!id) throw new Error("A project is required before codebase analysis can run.");
    if (this.running) return;
    this.running = true;
    this.refreshModal();
    this.host.setStatus("Starting codebase analysis…", "busy");
    try {
      const payload = await runProjectDebugger(id);
      this.lastRun = payload.last_run;
      this.refreshModal();
      this.startObservation(id, payload.job.id);
      this.host.actionOk(payload.message || `Analysis job #${payload.job.id} queued`);
    } catch (error) {
      this.running = false;
      this.refreshModal();
      this.host.actionFail(error);
    }
  }

  private modalHTML(agentConfig?: Record<string, unknown>): string {
    if (!this.projectID) throw new Error("Project analysis modal has no project id.");
    const resolved = (agentConfig?.resolved as Record<string, unknown> | undefined) ?? {};
    const configured = this.host.agentConfig();
    const system =
      (typeof agentConfig?.system === "string" && agentConfig.system) ||
      (typeof resolved.system === "string" && resolved.system) ||
      configured?.system ||
      "omnidex";
    const source = (typeof agentConfig?.source === "string" && agentConfig.source) || configured?.source || "env";
    return renderProjectDebuggerModal({
      projectID: this.projectID,
      projectName: this.projectName,
      agentSystem: system,
      agentSource: source,
      lastRun: this.lastRun,
      running: this.running,
      statusText: this.running ? "Analyzing codebase map and backlog…" : this.lastRun?.summary || "",
    });
  }

  private refreshModal(agentConfig?: Record<string, unknown>): void {
    const panel = this.host.modalPanel();
    if (!panel || !this.projectID) throw new Error("Project analysis modal is unavailable.");
    panel.innerHTML = this.modalHTML(agentConfig);
  }

  private startObservation(projectID: number, jobID: number): void {
    this.stopObservation("A newer project analysis replaced this observation.");
    const observation = observeRealtimeJob({
      jobID,
      timeoutMs: 30 * 60 * 1000,
      load: async () => {
        const [jobDetails, statusPayload] = await Promise.all([
          fetchJobRecord(jobID),
          fetchProjectDebuggerStatus(projectID),
        ]);
        if (!jobDetails.job || jobDetails.job.id !== jobID) {
          throw new Error(`Authoritative job response did not include project analysis job #${jobID}.`);
        }
        return {
          status: jobDetails.job.status,
          error: jobDetails.job.error,
          data: { job: jobDetails.job, status: statusPayload },
        };
      },
      onUpdate: ({ status, data }) => {
        if (this.observation !== observation) return;
        this.lastRun = data.status.last_run ?? this.lastRun;
        this.running = !["completed", "failed", "canceled"].includes(status);
        this.refreshModal(data.status.agent_config);
      },
    });
    this.observation = observation;
    void observation.completion
      .then(({ data }) => {
        if (this.observation !== observation) return;
        this.running = false;
        this.lastRun = data.status.last_run ?? this.lastRun;
        this.refreshModal(data.status.agent_config);
        const cards = this.lastRun?.cards_created?.length ?? 0;
        const tickets = this.lastRun?.cards_created?.filter((card) => card.ticket_job_id).length ?? 0;
        this.host.actionOk(`Analysis finished — ${cards} backlog card(s), ${tickets} planning ticket(s) queued`);
        if (this.host.selectedProjectID() === projectID && this.host.activeTab() === "scrum") {
          this.host.refreshScrum(projectID);
        }
      })
      .catch((error) => {
        if (this.observation !== observation) return;
        this.running = false;
        const message = this.lastRun?.error || (error instanceof Error ? error.message : String(error));
        this.host.setStatus(message, "error");
        this.refreshModal();
      })
      .finally(() => {
        if (this.observation === observation) this.observation = null;
      });
  }

  private stopObservation(reason: string): void {
    const observation = this.observation;
    this.observation = null;
    observation?.cancel(reason);
  }
}
