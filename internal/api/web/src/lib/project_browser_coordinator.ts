import {
  browseDirectory,
  createBrowseDirectory,
  createProject,
  fetchHostBridgeStatus,
} from "./project_api";
import { renderBrowseModal, renderProjectCreateModal } from "./project_render";
import { setGlobalLoading } from "./loading";
import { showToast } from "./toast";
import type { BrowseResponse, ProjectRecord, RecipeCatalogItem } from "./project_types";

export type ProjectStatusTone = "idle" | "busy" | "error" | "ok";

export interface ProjectBrowserHost {
  recipes(): RecipeCatalogItem[];
  detailRoot(): HTMLElement;
  modalPanel(): HTMLElement | null;
  openModal(html: string): Promise<void>;
  closeModal(): void;
  setStatus(message: string, tone?: ProjectStatusTone): void;
  projectCreated(project: ProjectRecord): Promise<void>;
}

export class ProjectBrowserCoordinator {
  private browsePath = "";
  private browseSelected = "";
  private browseData: BrowseResponse | null = null;
  private browseMode: "create" | "edit" = "create";
  private browseProjectID: number | null = null;
  private pendingCreatePath = "";

  constructor(private readonly host: ProjectBrowserHost) {}

  async showCreateModal(): Promise<void> {
    await this.host.openModal(renderProjectCreateModal(this.host.recipes()));
    this.setModalFeedback("", "idle");
    this.setCreateSubmitting(false);
  }

  async openBrowse(event: Event): Promise<void> {
    event.preventDefault();
    this.browseMode = "create";
    this.browseProjectID = null;
    await this.openBrowseAt(this.pendingCreatePath);
  }

  async browseForEdit(event: Event): Promise<void> {
    event.preventDefault();
    this.browseMode = "edit";
    this.browseProjectID = Number((event.currentTarget as HTMLElement).dataset.projectId || 0) || null;
    const location = this.host.detailRoot().querySelector('[data-projects-field="location"]') as HTMLInputElement | null;
    await this.openBrowseAt(location?.value ?? "");
  }

  async enterBrowseDir(event: Event): Promise<void> {
    event.preventDefault();
    await this.openBrowseAt((event.currentTarget as HTMLElement).dataset.path || "");
  }

  async selectBrowseDir(event: Event): Promise<void> {
    event.preventDefault();
    this.browseSelected = (event.currentTarget as HTMLElement).dataset.path || this.browseSelected;
    await this.renderBrowseView();
  }

  async createBrowseFolder(event: Event): Promise<void> {
    event.preventDefault();
    const name = this.browseField("newFolderName");
    if (!name) {
      this.setModalFeedback("Enter a folder name.", "error");
      return;
    }
    const parent = this.browsePath;
    this.setModalFeedback("Creating folder…", "busy");
    this.host.setStatus("Creating folder…", "busy");
    setGlobalLoading(true);
    try {
      const payload = await createBrowseDirectory(parent, name);
      await this.openBrowseAt(parent);
      this.browseSelected = payload.path;
      await this.renderBrowseView();
      const field = this.host.modalPanel()?.querySelector('[data-browse-field="newFolderName"]') as HTMLInputElement | null;
      if (field) field.value = "";
      this.setModalFeedback(`Created folder “${name}”.`, "ok");
    } catch (error) {
      this.setModalFeedback(errorMessage(error), "error");
    } finally {
      setGlobalLoading(false);
    }
  }

  async confirmBrowse(event: Event): Promise<void> {
    event.preventDefault();
    const path = (event.currentTarget as HTMLElement).dataset.path || this.browseSelected || this.browsePath;
    if (this.browseMode === "create") {
      this.pendingCreatePath = path;
      const name = path.split("/").filter(Boolean).pop() || "project";
      await this.host.openModal(renderProjectCreateModal(this.host.recipes()));
      this.setModalField("selectedPath", path);
      this.setModalField("createName", name);
      this.setModalFeedback("", "idle");
      this.setCreateSubmitting(false);
      return;
    }
    if (this.browseProjectID) {
      const input = this.host.detailRoot().querySelector('[data-projects-field="location"]') as HTMLInputElement | null;
      if (!input) throw new Error("Project location field is unavailable.");
      input.value = path;
    }
    this.host.closeModal();
  }

  async submitCreate(event: Event): Promise<void> {
    event.preventDefault();
    const location = this.modalField("selectedPath");
    const name = this.modalField("createName");
    if (!location) {
      this.setModalFeedback("Choose a working directory first.", "error");
      return;
    }
    this.setModalFeedback("Creating project…", "busy");
    this.setCreateSubmitting(true);
    setGlobalLoading(true);
    try {
      const payload = await createProject({
        name: name || location.split("/").filter(Boolean).pop() || "project",
        location,
        description: this.modalField("createDesc"),
        recipe_id: this.modalField("createRecipe"),
      });
      this.host.closeModal();
      await this.host.projectCreated(payload.project);
    } catch (error) {
      this.setModalFeedback(errorMessage(error), "error");
      this.setCreateSubmitting(false);
    } finally {
      setGlobalLoading(false);
    }
  }

  private async openBrowseAt(path: string): Promise<void> {
    this.host.setStatus("Browsing directories…", "busy");
    setGlobalLoading(true);
    try {
      const data = await browseDirectory(path);
      this.browseData = data;
      this.browsePath = data.path;
      this.browseSelected = data.path;
      await this.renderBrowseView();
      this.setModalFeedback("", "idle");
      this.host.setStatus("Browse open", "idle");
    } catch (error) {
      await this.showHostBridgeHint();
      this.setModalFeedback(errorMessage(error), "error");
    } finally {
      setGlobalLoading(false);
    }
  }

  private async renderBrowseView(): Promise<void> {
    if (!this.browseData) throw new Error("Directory browser state is unavailable.");
    await this.host.openModal(renderBrowseModal(this.browseData, this.browseSelected, this.browseMode));
  }

  private async showHostBridgeHint(): Promise<void> {
    try {
      const payload = await fetchHostBridgeStatus();
      if (payload.reachable) return;
      const tips = Array.isArray(payload.suggestions) ? payload.suggestions.filter((item) => typeof item === "string") : [];
      if (tips.length > 0) {
        this.host.setStatus(`Host bridge unavailable — ${tips[0]}`, "error");
      } else if (typeof payload.message === "string" && payload.message.trim()) {
        this.host.setStatus(payload.message, "error");
      }
    } catch (error) {
      console.error("Host bridge status lookup failed while directory browsing was unavailable", error);
    }
  }

  private modalField(name: string): string {
    const field = this.host.modalPanel()?.querySelector(`[data-projects-field="${name}"]`) as
      | HTMLInputElement
      | HTMLTextAreaElement
      | HTMLSelectElement
      | null;
    return field?.value?.trim() ?? "";
  }

  private setModalField(name: string, value: string): void {
    const field = this.host.modalPanel()?.querySelector(`[data-projects-field="${name}"]`) as
      | HTMLInputElement
      | HTMLTextAreaElement
      | null;
    if (!field) throw new Error(`Project modal field ${JSON.stringify(name)} is unavailable.`);
    field.value = value;
  }

  private browseField(name: string): string {
    const field = this.host.modalPanel()?.querySelector(`[data-browse-field="${name}"]`) as HTMLInputElement | null;
    return field?.value?.trim() ?? "";
  }

  private setCreateSubmitting(submitting: boolean): void {
    const button = this.host.modalPanel()?.querySelector("[data-projects-create-submit]") as HTMLButtonElement | null;
    if (!button) throw new Error("Project create submit button is unavailable.");
    button.disabled = submitting;
    button.textContent = submitting ? "Creating project…" : "Create project";
  }

  private setModalFeedback(message: string, tone: ProjectStatusTone): void {
    const classes: Record<ProjectStatusTone, string[]> = {
      idle: ["border-white/10", "bg-zinc-900/80", "text-zinc-300"],
      busy: ["border-cyan-300/30", "bg-cyan-300/10", "text-cyan-100"],
      error: ["border-rose-400/30", "bg-rose-400/10", "text-rose-100"],
      ok: ["border-emerald-400/30", "bg-emerald-400/10", "text-emerald-100"],
    };
    const slots = this.host.modalPanel()?.querySelectorAll("[data-projects-modal-feedback]");
    if (!slots?.length) throw new Error("Project modal feedback target is unavailable.");
    const allClasses = Object.values(classes).flat();
    slots.forEach((slot) => {
      const node = slot as HTMLElement;
      node.classList.remove(...allClasses);
      node.classList.toggle("hidden", !message);
      node.classList.add(...classes[tone]);
      node.setAttribute("role", tone === "error" ? "alert" : "status");
      node.textContent = message;
    });
    if (message) this.host.setStatus(message, tone);
    if (message && (tone === "ok" || tone === "error")) showToast(message, tone);
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
