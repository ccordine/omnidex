import { Controller } from "@hotwired/stimulus";
import {
  browseDirectory,
  createBrowseDirectory,
  fetchHostBridgeStatus,
  createProject,
  deleteProject,
  fetchProjects,
  fetchProject,
  fetchProjectMap,
  fetchRecipes,
  scanProjectMap,
  surveyProject,
  updateProject,
} from "../lib/project_api";
import { renderBrowseModal, renderProjectCreateModal, renderProjectDetail, renderProjectList } from "../lib/project_render";
import { collectModelFieldValues, clearModelFieldInputs } from "../lib/model_config_render";
import { collectAgentFieldValues, clearAgentFieldInputs } from "../lib/agent_config_render";
import { fetchAgentDefaults } from "../lib/agent_config_api";
import type { ResolvedAgentConfig } from "../lib/agent_config_types";
import { renderRecyclrBundle } from "../lib/recyclr";
import { closeModalShell, openModalShell } from "../lib/modal";
import { showToast } from "../lib/toast";
import type { ResolvedModelConfig } from "../lib/model_config_types";
import type { BrowseResponse, ProjectMapSummary, ProjectRecord, RecipeCatalogItem } from "../lib/project_types";
import type GxController from "./gx_controller";

export default class ProjectsController extends Controller {
  static targets = ["list", "detail", "status", "openBadge"];

  declare readonly listTarget: HTMLElement;
  declare readonly detailTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly hasOpenBadgeTarget: boolean;
  declare readonly openBadgeTarget: HTMLElement;

  private projects: ProjectRecord[] = [];
  private recipes: RecipeCatalogItem[] = [];
  private selectedProjectID: number | null = null;
  private activeTab = "scrum";
  private browsePath = "";
  private browseSelected = "";
  private browseData: BrowseResponse | null = null;
  private browseMode: "create" | "edit" = "create";
  private browseProjectID: number | null = null;
  private pendingCreatePath = "";
  private pendingCreateName = "";
  private panelShownHandler: ((event: Event) => void) | null = null;
  private currentModelConfig: ResolvedModelConfig | null = null;
  private currentAgentConfig: ResolvedAgentConfig | null = null;
  private currentProjectMap: ProjectMapSummary | null = null;

  connect() {
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "projects") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
  }

  disconnect() {
    if (this.panelShownHandler) {
      document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    }
  }

  setStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.statusTarget.textContent = message;
    this.statusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  private setModalFeedback(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const toneClasses: Record<string, string[]> = {
      idle: ["border-white/10", "bg-zinc-900/80", "text-zinc-300"],
      busy: ["border-cyan-300/30", "bg-cyan-300/10", "text-cyan-100"],
      error: ["border-rose-400/30", "bg-rose-400/10", "text-rose-100"],
      ok: ["border-emerald-400/30", "bg-emerald-400/10", "text-emerald-100"],
    };
    const allToneClasses = Object.values(toneClasses).flat();
    const slots = this.modalPanel()?.querySelectorAll("[data-projects-modal-feedback]") ?? [];
    if (slots.length === 0) {
      if (message) this.setStatus(message, tone);
      return;
    }
    slots.forEach((slot) => {
      const node = slot as HTMLElement;
      node.classList.remove(...allToneClasses);
      if (!message) {
        node.classList.add("hidden");
        node.textContent = "";
        return;
      }
      node.classList.remove("hidden");
      node.classList.add(...(toneClasses[tone] ?? toneClasses.idle));
      node.setAttribute("role", tone === "error" ? "alert" : "status");
      node.textContent = message;
    });
    if (message) this.setStatus(message, tone);
  }

  private setCreateSubmitting(submitting: boolean) {
    const button = this.modalPanel()?.querySelector("[data-projects-create-submit]") as HTMLButtonElement | null;
    if (!button) return;
    button.disabled = submitting;
    button.textContent = submitting ? "Creating project…" : "Create project";
  }

  private gxHost(): GxController | null {
    return (window as Window & { omniRecyclr?: GxController }).omniRecyclr ?? null;
  }

  openModal(html: string) {
    renderRecyclrBundle(this.gxHost(), "modal", html);
    openModalShell({ wide: true });
  }

  closeBrowse() {
    closeModalShell();
  }

  closeCreateModal() {
    this.closeBrowse();
  }

  private modalPanel(): HTMLElement | null {
    return document.querySelector('[data-chat-target="modalPanel"]');
  }

  private modalField(name: string): string {
    const field = this.modalPanel()?.querySelector(`[data-projects-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    return field?.value?.trim() ?? "";
  }

  private setModalField(name: string, value: string) {
    const field = this.modalPanel()?.querySelector(`[data-projects-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | null;
    if (field) field.value = value;
  }

  updateOpenBadge(name: string | null) {
    if (!this.hasOpenBadgeTarget) return;
    this.openBadgeTarget.textContent = name?.trim() || "None open";
  }

  private dispatchProjectOpened(project: ProjectRecord) {
    document.dispatchEvent(
      new CustomEvent("omni:project-opened", {
        detail: {
          project_id: project.id,
          name: project.name,
          location: project.location,
        },
      }),
    );
    this.updateOpenBadge(project.name);
  }

  private dispatchProjectClosed() {
    document.dispatchEvent(new CustomEvent("omni:project-closed"));
    this.updateOpenBadge(null);
  }

  async load() {
    this.setStatus("Loading projects…", "busy");
    try {
      const [projectsPayload, recipesPayload] = await Promise.all([
        fetchProjects(),
        fetchRecipes().catch(() => ({ recipes: [], root: "" })),
      ]);
      this.projects = projectsPayload.projects ?? [];
      this.recipes = recipesPayload.recipes ?? [];
      this.listTarget.innerHTML = renderProjectList(this.projects);
      if (this.selectedProjectID) {
        await this.renderDetail(this.selectedProjectID);
      } else {
        this.detailTarget.innerHTML = "";
        this.detailTarget.classList.add("hidden");
        this.listTarget.classList.remove("hidden");
        this.dispatchProjectClosed();
      }
      this.setStatus(`${this.projects.length} projects`, "ok");
    } catch (error) {
      this.listTarget.innerHTML = `<div class="rounded-xl border border-rose-400/20 bg-rose-400/5 p-6 text-sm text-rose-200">${error instanceof Error ? error.message : String(error)}</div>`;
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  showCreateModal() {
    this.openModal(renderProjectCreateModal(this.recipes));
    this.setModalFeedback("", "idle");
    this.setCreateSubmitting(false);
  }

  async openBrowse(event: Event) {
    event.preventDefault();
    this.browseMode = "create";
    this.browseProjectID = null;
    await this.openBrowseAt(this.pendingCreatePath || "");
  }

  async browseForEdit(event: Event) {
    event.preventDefault();
    this.browseMode = "edit";
    this.browseProjectID = Number((event.currentTarget as HTMLElement).dataset.projectId || 0) || null;
    const location = (this.detailTarget.querySelector('[data-projects-field="location"]') as HTMLInputElement | null)?.value;
    await this.openBrowseAt(location || "");
  }

  private browseField(name: string): string {
    const field = this.modalPanel()?.querySelector(`[data-browse-field="${name}"]`) as HTMLInputElement | null;
    return field?.value?.trim() ?? "";
  }

  private renderBrowseView() {
    if (!this.browseData) return;
    renderRecyclrBundle(this.gxHost(), "modal", renderBrowseModal(this.browseData, this.browseSelected, this.browseMode));
    openModalShell({ wide: true });
  }

  private async showHostBridgeHint() {
    try {
      const payload = await fetchHostBridgeStatus();
      if (payload.reachable) return;
      const tips = Array.isArray(payload.suggestions) ? payload.suggestions.filter((item) => typeof item === "string") : [];
      if (tips.length > 0) {
        this.setStatus(`Host bridge unavailable — ${tips[0]}`, "error");
      } else if (typeof payload.message === "string" && payload.message.trim()) {
        this.setStatus(payload.message, "error");
      }
    } catch {
      // ignore secondary status failures
    }
  }

  async openBrowseAt(path: string) {
    this.setStatus("Browsing directories…", "busy");
    try {
      const data = await browseDirectory(path);
      this.browseData = data;
      this.browsePath = data.path;
      this.browseSelected = data.path;
      this.renderBrowseView();
      this.setModalFeedback("", "idle");
      this.setStatus("Browse open", "idle");
    } catch (error) {
      await this.showHostBridgeHint();
      const message = error instanceof Error ? error.message : String(error);
      this.setModalFeedback(message, "error");
      this.setStatus(message, "error");
    }
  }

  async enterBrowseDir(event: Event) {
    event.preventDefault();
    const path = (event.currentTarget as HTMLElement).dataset.path || "";
    await this.openBrowseAt(path);
  }

  selectBrowseDir(event: Event) {
    event.preventDefault();
    this.browseSelected = (event.currentTarget as HTMLElement).dataset.path || this.browseSelected;
    this.renderBrowseView();
  }

  async createBrowseFolder(event: Event) {
    event.preventDefault();
    const name = this.browseField("newFolderName");
    if (!name) {
      this.setModalFeedback("Enter a folder name.", "error");
      return;
    }
    const parent = this.browsePath;
    this.setModalFeedback("Creating folder…", "busy");
    this.setStatus("Creating folder…", "busy");
    try {
      const payload = await createBrowseDirectory(parent, name);
      await this.openBrowseAt(parent);
      this.browseSelected = payload.path;
      this.renderBrowseView();
      const field = this.modalPanel()?.querySelector('[data-browse-field="newFolderName"]') as HTMLInputElement | null;
      if (field) field.value = "";
      this.setModalFeedback(`Created folder “${name}”.`, "ok");
      this.setStatus("Folder created", "ok");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.setModalFeedback(message, "error");
      this.setStatus(message, "error");
    }
  }

  confirmBrowse(event: Event) {
    event.preventDefault();
    const path = (event.currentTarget as HTMLElement).dataset.path || this.browseSelected || this.browsePath;
    if (this.browseMode === "create") {
      this.pendingCreatePath = path;
      this.pendingCreateName = path.split("/").filter(Boolean).pop() || "project";
      this.openModal(renderProjectCreateModal(this.recipes));
      this.setModalField("selectedPath", this.pendingCreatePath);
      this.setModalField("createName", this.pendingCreateName);
      this.setModalFeedback("", "idle");
      this.setCreateSubmitting(false);
      return;
    } else if (this.browseProjectID) {
      const input = this.detailTarget.querySelector('[data-projects-field="location"]') as HTMLInputElement | null;
      if (input) input.value = path;
    }
    this.closeBrowse();
  }

  async submitCreate(event: Event) {
    event.preventDefault();
    const location = this.modalField("selectedPath");
    const name = this.modalField("createName");
    if (!location) {
      this.setModalFeedback("Choose a working directory first.", "error");
      return;
    }
    this.setModalFeedback("Creating project…", "busy");
    this.setCreateSubmitting(true);
    try {
      const payload = await createProject({
        name: name || location.split("/").filter(Boolean).pop() || "project",
        location,
        description: this.modalField("createDesc"),
        recipe_id: this.modalField("createRecipe"),
        activate: false,
      });
      this.selectedProjectID = payload.project.id;
      this.activeTab = "scrum";
      const createdName = payload.project.name || name || "project";
      this.closeCreateModal();
      showToast(`Project “${createdName}” created.`, "ok");
      await this.load();
      this.setStatus("Project created", "ok");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.setModalFeedback(message, "error");
      this.setCreateSubmitting(false);
    }
  }

  async openProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.selectedProjectID = id;
    this.activeTab = "scrum";
    await this.renderDetail(id);
  }

  private applyTabState() {
    const tab = this.activeTab;
    this.detailTarget.querySelectorAll("[data-project-tab-panel]").forEach((panel) => {
      panel.classList.toggle("hidden", panel.getAttribute("data-project-tab-panel") !== tab);
    });
    this.detailTarget.querySelectorAll("[data-project-tab]").forEach((button) => {
      const active = button.getAttribute("data-project-tab") === tab;
      button.classList.toggle("border-cyan-300/40", active);
      button.classList.toggle("bg-cyan-300/10", active);
      button.classList.toggle("text-cyan-100", active);
      button.classList.toggle("border-white/10", !active);
      button.classList.toggle("text-zinc-400", !active);
    });
  }

  showTab(event: Event) {
    event.preventDefault();
    this.activeTab = (event.currentTarget as HTMLElement).dataset.projectTab || "scrum";
    this.applyTabState();
    document.dispatchEvent(
      new CustomEvent("omni:project-tab", {
        detail: { tab: this.activeTab, project_id: this.selectedProjectID },
      }),
    );
  }

  async renderDetail(id: number) {
    this.setStatus("Loading project…", "busy");
    try {
      const [{ project, modelConfig }, agentPayload, projectMap] = await Promise.all([
        fetchProject(id),
        fetchAgentDefaults(id).catch(() => null),
        fetchProjectMap(id).catch(() => null),
      ]);
      this.currentModelConfig = modelConfig ?? null;
      this.currentAgentConfig = agentPayload?.resolved ?? null;
      this.currentProjectMap = projectMap;
      this.detailTarget.innerHTML = renderProjectDetail(
        project,
        this.recipes,
        modelConfig?.fields ?? [],
        modelConfig?.source ?? "env",
        agentPayload?.resolved?.fields ?? agentPayload?.fields ?? [],
        agentPayload?.resolved?.source ?? "env",
        agentPayload?.resolved?.system ?? "omnidex",
        projectMap,
        this.activeTab,
      );
      this.applyTabState();
      document.dispatchEvent(
        new CustomEvent("omni:project-tab", {
          detail: { tab: this.activeTab, project_id: this.selectedProjectID },
        }),
      );
      this.detailTarget.classList.remove("hidden");
      this.listTarget.classList.add("hidden");
      this.dispatchProjectOpened(project);
      this.setStatus(project.name, "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  backToList() {
    this.selectedProjectID = null;
    this.activeTab = "scrum";
    this.detailTarget.classList.add("hidden");
    this.listTarget.classList.remove("hidden");
    this.dispatchProjectClosed();
  }

  fieldValue(name: string): string {
    const node = this.detailTarget.querySelector(`[data-projects-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    return node?.value?.trim() ?? "";
  }

  async saveProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Saving…", "busy");
    try {
      await updateProject(id, {
        name: this.fieldValue("name"),
        location: this.fieldValue("location"),
        description: this.fieldValue("description"),
        recipe_id: this.fieldValue("recipeId"),
      });
      await this.load();
      this.selectedProjectID = id;
      await this.renderDetail(id);
      this.setStatus("Project saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async saveRecipe(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    const raw = (this.detailTarget.querySelector('[data-projects-field="recipeJson"]') as HTMLTextAreaElement | null)?.value ?? "{}";
    let recipe: Record<string, unknown>;
    try {
      recipe = JSON.parse(raw) as Record<string, unknown>;
    } catch {
      this.setStatus("Recipe JSON is invalid", "error");
      return;
    }
    this.setStatus("Saving recipe…", "busy");
    try {
      await updateProject(id, { recipe_id: this.fieldValue("recipeId"), recipe });
      await this.renderDetail(id);
      this.setStatus("Recipe saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  loadCatalogRecipe(event: Event) {
    event.preventDefault();
    const recipeID = this.fieldValue("recipeId");
    const recipe = this.recipes.find((entry) => entry.id === recipeID);
    const editor = this.detailTarget.querySelector('[data-projects-field="recipeJson"]') as HTMLTextAreaElement | null;
    if (recipe && editor) editor.value = JSON.stringify(recipe, null, 2);
  }

  async saveModelConfig(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Saving model settings…", "busy");
    try {
      await updateProject(id, { model_config: collectModelFieldValues(this.detailTarget, "project") });
      await this.renderDetail(id);
      this.setStatus("Model settings saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async clearModelConfig(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    clearModelFieldInputs(this.detailTarget, "project");
    this.setStatus("Clearing model overrides…", "busy");
    try {
      await updateProject(id, { model_config: {} });
      await this.renderDetail(id);
      this.setStatus("Model overrides cleared", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async saveAgentConfig(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Saving agent settings…", "busy");
    try {
      await updateProject(id, { agent_config: collectAgentFieldValues(this.detailTarget, "project") });
      await this.renderDetail(id);
      this.setStatus("Agent settings saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async clearAgentConfig(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    clearAgentFieldInputs(this.detailTarget, "project");
    this.setStatus("Clearing agent overrides…", "busy");
    try {
      await updateProject(id, { agent_config: {} });
      await this.renderDetail(id);
      this.setStatus("Agent overrides cleared", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async rescanProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Detecting project stack…", "busy");
    try {
      await surveyProject(id);
      await this.renderDetail(id);
      this.setStatus("Project stack detected", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async scanProjectMap(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Scanning project directory and updating map…", "busy");
    try {
      this.currentProjectMap = await scanProjectMap(id);
      await this.renderDetail(id);
      this.setStatus(this.currentProjectMap.message || "Codebase map updated", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async deleteProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id || !window.confirm("Delete this project and its scrum cards?")) return;
    this.setStatus("Deleting…", "busy");
    try {
      await deleteProject(id);
      this.selectedProjectID = null;
      await this.load();
      this.setStatus("Project deleted", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }
}
