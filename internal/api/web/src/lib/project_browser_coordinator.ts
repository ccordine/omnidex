import { createBrowseDirectory, createProject } from "./project_api";
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
}

export class ProjectBrowserCoordinator {
  private browsePath = "";
  private browseSelected = "";
  private browseOffset = 0;
  private browseMode: "create" | "edit" = "create";
  private browseProjectID: number | null = null;
  private pendingCreatePath = "";

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

	async loadCreateRecipePage(event: Event): Promise<void> {
		event.preventDefault();
		const offset = Number((event.currentTarget as HTMLElement).dataset.pageOffset ?? -1);
		if (!Number.isSafeInteger(offset) || offset < 0) throw new Error("Create-project recipe page offset is invalid.");
		const query = new URLSearchParams({
			selected: this.modalField("selectedPath"),
			name: this.modalField("createName"),
			description: this.modalField("createDesc"),
			recipe_id: this.modalField("createRecipe"),
			recipe_offset: String(offset),
		});
		await this.host.openModal(await fetchProjectModalComponent("create", query));
	}

  async browseForEdit(event: Event): Promise<void> {
    event.preventDefault();
    this.browseMode = "edit";
    this.browseProjectID = Number((event.currentTarget as HTMLElement).dataset.projectId || 0) || null;
    const location = this.host.detailRoot().querySelector("[data-projects-field='location']") as HTMLInputElement | null;
    await this.openBrowseAt(location?.value ?? "");
  }

  async enterBrowseDir(event: Event): Promise<void> {
    event.preventDefault();
    await this.openBrowseAt((event.currentTarget as HTMLElement).dataset.path ?? "");
  }

  async loadBrowsePage(event: Event): Promise<void> {
    event.preventDefault();
    const offset = Number((event.currentTarget as HTMLElement).dataset.pageOffset ?? -1);
    if (!Number.isSafeInteger(offset) || offset < 0) {
      throw new Error("Directory page offset is invalid.");
    }
    this.browsePath = this.browseField("currentPath");
    if (!this.browsePath) throw new Error("Directory browser did not provide its authoritative current path.");
    this.browseOffset = offset;
    this.setModalFeedback("Loading directories…", "busy");
    await this.renderBrowseView();
  }

  async selectBrowseDir(event: Event): Promise<void> {
    event.preventDefault();
    this.browseSelected = (event.currentTarget as HTMLElement).dataset.path ?? this.browseSelected;
    await this.renderBrowseView();
  }

  async createBrowseFolder(event: Event): Promise<void> {
    event.preventDefault();
    this.browsePath = this.browseField("currentPath");
    if (!this.browsePath) throw new Error("Directory browser did not provide its authoritative current path.");
    const name = this.browseField("newFolderName");
    if (!name) return this.setModalFeedback("Enter a folder name.", "error");
    this.setModalFeedback("Creating folder…", "busy");
    setGlobalLoading(true);
    try {
      const payload = await createBrowseDirectory(this.browsePath, name);
      this.browseSelected = payload.path;
      await this.renderBrowseView();
      this.setModalFeedback(`Created folder “${name}”.`, "ok");
    } catch (error) {
      this.setModalFeedback(errorMessage(error), "error");
    } finally {
      setGlobalLoading(false);
    }
  }

  async confirmBrowse(event: Event): Promise<void> {
    event.preventDefault();
    const path = (event.currentTarget as HTMLElement).dataset.path ?? this.browseSelected;
    if (this.browseMode === "create") {
      this.pendingCreatePath = path;
      const query = new URLSearchParams({ selected: path, name: path.split("/").filter(Boolean).pop() || "project" });
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
    const location = this.modalField("selectedPath");
    if (!location) return this.setModalFeedback("Choose a working directory first.", "error");
    this.setModalFeedback("Creating project…", "busy");
    setGlobalLoading(true);
    try {
      const payload = await createProject({
        name: this.modalField("createName") || location.split("/").filter(Boolean).pop() || "project",
        location,
        description: this.modalField("createDesc"),
        recipe_id: this.modalField("createRecipe"),
      });
      this.host.closeModal();
      await this.host.projectCreated(payload.project);
    } catch (error) {
      this.setModalFeedback(errorMessage(error), "error");
    } finally {
      setGlobalLoading(false);
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

  private modalField(name: string): string {
    const field = this.host.modalPanel()?.querySelector(`[data-projects-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    return field?.value?.trim() ?? "";
  }

  private browseField(name: string): string {
    return (this.host.modalPanel()?.querySelector(`[data-browse-field="${name}"]`) as HTMLInputElement | null)?.value.trim() ?? "";
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
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
