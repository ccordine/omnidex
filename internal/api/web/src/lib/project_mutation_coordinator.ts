import {
  deleteProject,
  pauseProjectAutoWork,
  projectAutoWorkFailure,
  projectMutationFailure,
  scanProjectMap,
  startProjectAutoWork,
  updateProject,
  validateProjectRevision,
} from "./project_api";
import { patchScrumAutoWork } from "./scrum_api";
import { clearConfigValues, collectConfigValues } from "./config_form_values";
import { setGlobalLoading } from "./loading";
import { HTTPResponseError } from "./api";

export type ProjectMutationHost = {
  detailRoot(): HTMLElement;
  reloadDetail(): Promise<void>;
  reloadList(): Promise<void>;
  projectDeleted(): void;
  setStatus(message: string, tone?: "idle" | "busy" | "error" | "ok"): void;
  success(message: string): void;
  failure(error: unknown): void;
};

export class ProjectMutationCoordinator {
  private mutationInFlight = false;

  constructor(private readonly host: ProjectMutationHost) {}

  private id(event: Event): number {
    const initiator = event.currentTarget;
    if (!(initiator instanceof HTMLButtonElement)) {
      throw new Error("Project mutation requires one server-rendered button control.");
    }
    const rawID = initiator.dataset.projectId;
    if (rawID === undefined || !/^[1-9][0-9]*$/.test(rawID)) {
      throw new Error("Project action requires one canonical positive project id.");
    }
    const id = Number(rawID);
    if (!Number.isSafeInteger(id)) throw new Error("Project action project id exceeds the safe integer bound.");
    return id;
  }

  private field(name: string, preserveBytes = false): string {
    const node = this.host.detailRoot().querySelector(`[data-projects-field='${name}']`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    if (!node) throw new Error(`Project field ${JSON.stringify(name)} is missing from server state.`);
    const value = node.value;
    return preserveBytes ? value : value.trim();
  }

  private revision(event: Event): string {
    const initiator = event.currentTarget;
    if (!(initiator instanceof HTMLButtonElement)) throw new Error("Project mutation requires one server-rendered button control.");
    return validateProjectRevision(
      initiator.dataset.projectUpdatedAt,
      "Project action server-rendered project revision",
    );
  }

  async saveProject(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    const revision = this.revision(event);
    await this.run(event, "Saving project…", "Project saved", async () => {
      const outcome = await updateProject(id, revision, {
        name: this.field("name"),
        location: this.field("location"),
        description: this.field("description", true),
      });
      await this.reconcileProject();
      const degraded = projectMutationFailure(outcome);
      if (degraded) throw new ReconciledMutationError(degraded);
    }, () => this.reconcileProject());
  }

  async saveModelConfig(event: Event): Promise<void> {
    event.preventDefault();
    await this.saveConfig(event, "project-model", "model_config", "Model settings saved");
  }

  async clearModelConfig(event: Event): Promise<void> {
    event.preventDefault();
    clearConfigValues(this.host.detailRoot(), "project-model");
    await this.saveConfig(event, "project-model", "model_config", "Model overrides cleared");
  }

  private async saveConfig(event: Event, scope: string, key: "model_config", message: string): Promise<void> {
    const id = this.id(event);
    const revision = this.revision(event);
    const values = collectConfigValues(this.host.detailRoot(), scope);
    await this.run(event, "Saving configuration…", message, async () => {
      const outcome = await updateProject(id, revision, { [key]: values });
      await this.reconcileProject();
      const degraded = projectMutationFailure(outcome);
      if (degraded) throw new ReconciledMutationError(degraded);
    }, () => this.reconcileProject());
  }

  async saveScrumAutomation(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    const enabledControl = this.host.detailRoot().querySelector("[data-projects-field='autoWorkEnabled']");
    if (!(enabledControl instanceof HTMLInputElement) || enabledControl.type !== "checkbox") {
      throw new Error("Scrum automation enabled control is missing from server state.");
    }
    const source_columns = [...this.host.detailRoot().querySelectorAll<HTMLInputElement>("[data-projects-field='autoWorkColumn']:checked")]
      .map((node) => {
        const column = node.dataset.autoWorkColumn;
        if (column === undefined || column === "" || column !== column.trim()) {
          throw new Error("Scrum automation source column is not exact server authority.");
        }
        return column;
      });
    await this.run(event, "Saving automation…", "Scrum automation saved", async () => {
      const outcome = await patchScrumAutoWork({ enabled: enabledControl.checked, source_columns }, id);
      await this.reconcileProject();
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: id } }));
      if (outcome.commit_state === "committed_degraded") {
        throw new ReconciledMutationError(outcome.operation_error);
      }
    }, () => this.reconcileProject());
  }

  async scanProjectMap(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    await this.run(event, "Scanning project…", "Codebase map updated", async () => { await scanProjectMap(id); await this.host.reloadDetail(); }, () => this.reconcileProject());
  }

  async refreshProjectGit(event: Event): Promise<void> {
    event.preventDefault();
    this.id(event);
    await this.run(event, "Refreshing Git status…", "Git status refreshed", () => this.host.reloadDetail());
  }

  async startAutoWork(event: Event): Promise<void> {
    event.preventDefault(); event.stopPropagation();
    const id = this.id(event);
    await this.run(event, "Starting auto-work…", "Auto-work started", async () => {
      const outcome = await startProjectAutoWork(id);
      await this.reconcileProject();
      const degraded = projectAutoWorkFailure(outcome);
      if (degraded) throw new ReconciledMutationError(degraded);
    }, () => this.reconcileProject());
  }

  async pauseAutoWork(event: Event): Promise<void> {
    event.preventDefault(); event.stopPropagation();
    const id = this.id(event);
    await this.run(event, "Pausing auto-work…", "Auto-work paused", async () => {
      const outcome = await pauseProjectAutoWork(id);
      await this.reconcileProject();
      const degraded = projectAutoWorkFailure(outcome);
      if (degraded) throw new ReconciledMutationError(degraded);
    }, () => this.reconcileProject());
  }

  async deleteProject(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    const revision = this.revision(event);
    if (!window.confirm("Delete this project and its Scrum cards?")) return;
    await this.run(
      event,
      "Deleting project…",
      "Project deleted",
      async () => { await deleteProject(id, revision); this.host.projectDeleted(); await this.host.reloadList(); },
      () => this.reconcileDeletion(),
    );
  }

  private async reconcileProject(): Promise<void> {
    await this.host.reloadList();
    await this.host.reloadDetail();
  }

  private async reconcileDeletion(): Promise<void> {
    await this.host.reloadList();
    try {
      await this.host.reloadDetail();
    } catch (error) {
      if (error instanceof HTTPResponseError && error.status === 404) {
        this.host.projectDeleted();
        return;
      }
      throw error;
    }
  }

  private async run(
    event: Event,
    working: string,
    success: string,
    action: () => Promise<void>,
    reconcileOnError?: () => Promise<void>,
  ): Promise<void> {
    if (this.mutationInFlight) {
      this.host.failure(new Error("A project mutation is already in progress."));
      return;
    }
    const initiator = event.currentTarget;
    if (!(initiator instanceof HTMLButtonElement)) {
      throw new Error("Project mutation requires one server-rendered button control.");
    }
    this.mutationInFlight = true;
    const controls = [...document.querySelectorAll<HTMLButtonElement>('button[data-action^="projects#"]')];
    if (!controls.includes(initiator)) controls.push(initiator);
    const disabledStates = new Map(controls.map((control) => [control, control.disabled]));
    const originalLabel = initiator.textContent;
    controls.forEach((control) => { control.disabled = true; });
    initiator.setAttribute("aria-busy", "true");
    initiator.textContent = working;
    this.host.setStatus(working, "busy");
    setGlobalLoading(true);
    try { await action(); this.host.success(success); }
    catch (error) {
      if (reconcileOnError && !(error instanceof ReconciledMutationError)) {
        try {
          await reconcileOnError();
        } catch (reconciliationError) {
          this.host.failure(new Error(
            `${errorMessage(error)} Authoritative project reconciliation failed: ${errorMessage(reconciliationError)}`,
          ));
          return;
        }
      }
      this.host.failure(error);
    }
    finally {
      this.mutationInFlight = false;
      setGlobalLoading(false);
      disabledStates.forEach((disabled, control) => { control.disabled = disabled; });
      initiator.setAttribute("aria-busy", "false");
      initiator.textContent = originalLabel;
    }
  }
}

class ReconciledMutationError extends Error {}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
