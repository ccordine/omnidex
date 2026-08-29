import { createBrowseDirectory, createProject, projectMutationFailure } from "./project_api";
import { fetchProjectModalComponent } from "./operational_component_api";
import type { ServerComponent } from "./server_component_api";
import { setGlobalLoading } from "./loading";
import { showToast } from "./toast";
import type { ProjectRecord } from "./project_types";

export type ProjectStatusTone = "idle" | "busy" | "error" | "ok";

export interface ProjectBrowserHost {
  detailRoot(): HTMLElement;
  modalPanel(): HTMLElement | null;
  openModal(component: ServerComponent): Promise<void>;
  closeModal(): void;
  setStatus(message: string, tone?: ProjectStatusTone): void;
  projectCreated(project: ProjectRecord): Promise<void>;
  reloadProjects(): Promise<void>;
}

export class ProjectBrowserCoordinator {
  private browsePath = "";
  private browseSelected = "";
  private browseOffset = 0;
  private browseMode: "create" | "edit" = "create";
  private browseProjectID: number | null = null;
  private pendingCreatePath = "";
  private mutationInFlight = false;
  private disabledStates = new Map<HTMLButtonElement, boolean>();

  constructor(private readonly host: ProjectBrowserHost) {}

  async showCreateModal(): Promise<void> {
    await this.host.openModal(await fetchProjectModalComponent("create"));
    this.setModalFeedback("", "idle");
  }

  async openBrowse(event: Event): Promise<void> {
    event.preventDefault();
    this.browseMode = "create";
    this.browseProjectID = null;
    await this.openBrowseAt(this.pendingCreatePath);
  }

  async browseForEdit(event: Event): Promise<void> {
    event.preventDefault();
    const target = event.currentTarget;
    if (!(target instanceof HTMLElement)) throw new Error("Edit browse operation requires one server-rendered control.");
    this.browseMode = "edit";
    this.browseProjectID = this.requiredPositiveID(target.dataset.projectId, "Edit browse operation");
    const location = this.host.detailRoot().querySelector("[data-projects-field='location']") as HTMLInputElement | null;
    if (!location) throw new Error("Project location field is unavailable.");
    await this.openBrowseAt(location.value);
  }

  async enterBrowseDir(event: Event): Promise<void> {
    event.preventDefault();
    await this.openBrowseAt(this.requiredDatasetPath(event, "Directory entry"));
  }

  async loadBrowsePage(event: Event): Promise<void> {
    event.preventDefault();
    const target = event.currentTarget;
    if (!(target instanceof HTMLElement)) throw new Error("Directory page requires one server-rendered control.");
    const rawOffset = target.dataset.pageOffset;
    if (rawOffset === undefined || !/^(0|[1-9][0-9]*)$/.test(rawOffset)) {
      throw new Error("Directory page offset is invalid.");
    }
    const offset = Number(rawOffset);
    if (!Number.isSafeInteger(offset)) throw new Error("Directory page offset exceeds the safe integer bound.");
    this.browsePath = this.browseField("currentPath");
    if (!this.browsePath) throw new Error("Directory browser did not provide its authoritative current path.");
    this.browseOffset = offset;
    this.setModalFeedback("Loading directories…", "busy");
    await this.renderBrowseView();
  }

  async selectBrowseDir(event: Event): Promise<void> {
    event.preventDefault();
    this.browseSelected = this.requiredDatasetPath(event, "Directory selection");
    await this.renderBrowseView();
  }

  async createBrowseFolder(event: Event): Promise<void> {
    event.preventDefault();
    this.browsePath = this.browseField("currentPath");
    if (!this.browsePath) throw new Error("Directory browser did not provide its authoritative current path.");
    const name = this.browseField("newFolderName");
    if (!name) return this.setModalFeedback("Enter a folder name.", "error");
    if (!this.beginMutation(event, "Creating folder…")) return;
    try {
      const payload = await createBrowseDirectory(this.browsePath, name);
      this.browseSelected = payload.path;
      await this.renderBrowseView();
      this.setModalFeedback(`Created folder “${name}”.`, "ok");
    } catch (error) {
      this.setModalFeedback(await this.reconcileFailure(error, () => this.renderBrowseView()), "error");
    } finally {
      this.endMutation(event);
    }
  }

  async confirmBrowse(event: Event): Promise<void> {
    event.preventDefault();
    const path = this.requiredDatasetPath(event, "Directory confirmation");
    if (this.browseMode === "create") {
      this.pendingCreatePath = path;
      const leaf = path.split("/").filter(Boolean).pop();
      if (!leaf) throw new Error("Selected directory does not provide an exact project name.");
      const query = new URLSearchParams({ selected: path, name: leaf });
      await this.host.openModal(await fetchProjectModalComponent("create", query));
      return;
    }
    if (!this.browseProjectID) throw new Error("Edit browse operation lacks a project id.");
    const location = this.host.detailRoot().querySelector("[data-projects-field='location']") as HTMLInputElement | null;
    if (!location) throw new Error("Project location field is unavailable.");
    location.value = path;
    this.host.closeModal();
  }

  async submitCreate(event: Event): Promise<void> {
    event.preventDefault();
    const location = this.modalField("selectedPath", true);
    if (!location.trim()) return this.setModalFeedback("Choose a working directory first.", "error");
    const name = this.modalField("createName");
    if (!name) return this.setModalFeedback("Enter a project name.", "error");
    if (!this.beginMutation(event, "Creating project…")) return;
    try {
      const payload = await createProject({
        name,
        location,
        description: this.modalField("createDesc", true),
      });
      this.host.closeModal();
      await this.host.projectCreated(payload.project);
      const degraded = projectMutationFailure(payload);
      if (degraded) {
        this.host.setStatus(degraded, "error");
        showToast(degraded, "error");
      }
    } catch (error) {
      this.setModalFeedback(await this.reconcileFailure(error, () => this.host.reloadProjects()), "error");
    } finally {
      this.endMutation(event);
    }
  }

  private async openBrowseAt(path: string): Promise<void> {
    this.browsePath = path;
    this.browseSelected = path;
    this.browseOffset = 0;
    await this.renderBrowseView();
  }

  private async renderBrowseView(): Promise<void> {
    const query = new URLSearchParams({
      path: this.browsePath,
      selected: this.browseSelected,
      mode: this.browseMode,
      offset: String(this.browseOffset),
    });
    await this.host.openModal(await fetchProjectModalComponent("browse", query));
    this.browsePath = this.browseField("currentPath");
    if (!this.browsePath) throw new Error("Directory browser did not provide its authoritative current path.");
    const authoritativeOffset = Number(this.browseField("currentOffset"));
    if (!Number.isSafeInteger(authoritativeOffset) || authoritativeOffset < 0) {
      throw new Error("Directory browser returned an invalid authoritative offset.");
    }
    this.browseOffset = authoritativeOffset;
  }

  private modalField(name: string, preserveBytes = false): string {
    const field = this.host.modalPanel()?.querySelector(`[data-projects-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    if (!field) throw new Error(`Project modal field ${JSON.stringify(name)} is unavailable.`);
    const value = field.value;
    return preserveBytes ? value : value.trim();
  }

  private browseField(name: string): string {
    const field = this.host.modalPanel()?.querySelector(`[data-browse-field="${name}"]`);
    if (!(field instanceof HTMLInputElement)) throw new Error(`Directory browser field ${JSON.stringify(name)} is unavailable.`);
    return field.value;
  }

  private requiredDatasetPath(event: Event, label: string): string {
    const current = event.currentTarget;
    if (!(current instanceof HTMLElement) || current.dataset.path === undefined || current.dataset.path === "" || current.dataset.path.includes("\0")) {
      throw new Error(`${label} lacks one exact server path.`);
    }
    return current.dataset.path;
  }

  private requiredPositiveID(raw: string | undefined, label: string): number {
    if (raw === undefined || !/^[1-9][0-9]*$/.test(raw)) throw new Error(`${label} requires one canonical project ID.`);
    const value = Number(raw);
    if (!Number.isSafeInteger(value)) throw new Error(`${label} project ID exceeds the safe integer bound.`);
    return value;
  }

  private beginMutation(event: Event, working: string): boolean {
    if (this.mutationInFlight) {
      this.setModalFeedback("A project browser mutation is already in progress.", "error");
      return false;
    }
    const initiator = event.currentTarget;
    if (!(initiator instanceof HTMLButtonElement) && !(initiator instanceof HTMLFormElement)) {
      throw new Error("Project browser mutation requires one server-rendered form or button.");
    }
    this.mutationInFlight = true;
    initiator.setAttribute("aria-busy", "true");
    const controls = [...(this.host.modalPanel()?.querySelectorAll<HTMLButtonElement>("button") ?? [])];
    this.disabledStates = new Map(controls.map((control) => [control, control.disabled]));
    for (const control of controls) control.disabled = true;
    this.setModalFeedback(working, "busy");
    setGlobalLoading(true);
    return true;
  }

  private endMutation(event: Event): void {
    this.mutationInFlight = false;
    setGlobalLoading(false);
    this.disabledStates.forEach((disabled, control) => { control.disabled = disabled; });
    this.disabledStates.clear();
    if (event.currentTarget instanceof HTMLElement) event.currentTarget.setAttribute("aria-busy", "false");
  }

  private setModalFeedback(message: string, tone: ProjectStatusTone): void {
    const node = this.host.modalPanel()?.querySelector("[data-projects-modal-feedback]") as HTMLElement | null;
    if (!node) {
      if (message) throw new Error("Project modal feedback target is unavailable.");
      return;
    }
    node.textContent = message;
    node.classList.toggle("hidden", !message);
    if (message) this.host.setStatus(message, tone);
    if (message && (tone === "ok" || tone === "error")) showToast(message, tone);
  }

  private async reconcileFailure(error: unknown, reconcile: () => Promise<void>): Promise<string> {
    try {
      await reconcile();
      return errorMessage(error);
    } catch (reconciliationError) {
      return `${errorMessage(error)} Authoritative reconciliation failed: ${errorMessage(reconciliationError)}`;
    }
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
