import { Controller } from "@hotwired/stimulus";
import {
  activateProject,
  browseDirectory,
  createProject,
  deleteProject,
  fetchProjects,
  fetchProject,
  fetchProjectMap,
  fetchRecipes,
  fetchWorkspace,
  scanProjectMap,
  surveyProject,
  updateProject,
} from "../lib/project_api";
import { renderBrowseModal, renderProjectDetail, renderProjectList } from "../lib/project_render";
import { collectModelFieldValues, clearModelFieldInputs } from "../lib/model_config_render";
import { collectAgentFieldValues, clearAgentFieldInputs } from "../lib/agent_config_render";
import { fetchAgentDefaults } from "../lib/agent_config_api";
import type { ResolvedAgentConfig } from "../lib/agent_config_types";
import { renderRecyclrBundle } from "../lib/recyclr";
import type { ResolvedModelConfig } from "../lib/model_config_types";
import type { ProjectRecord, ProjectMapSummary, RecipeCatalogItem } from "../lib/project_types";
import type GxController from "./gx_controller";

export default class ProjectsController extends Controller {
  static targets = [
    "list",
    "detail",
    "status",
    "activeBadge",
    "createPanel",
    "createName",
    "createDesc",
    "createRecipe",
    "selectedPath",
    "createForm",
  ];

  declare readonly listTarget: HTMLElement;
  declare readonly detailTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly activeBadgeTarget: HTMLElement;
  declare readonly createPanelTarget: HTMLElement;
  declare readonly createNameTarget: HTMLInputElement;
  declare readonly createDescTarget: HTMLTextAreaElement;
  declare readonly createRecipeTarget: HTMLSelectElement;
  declare readonly selectedPathTarget: HTMLInputElement;
  declare readonly createFormTarget: HTMLFormElement;

  private projects: ProjectRecord[] = [];
  private recipes: RecipeCatalogItem[] = [];
  private selectedProjectID: number | null = null;
  private browsePath = "";
  private browseSelected = "";
  private browseMode: "create" | "edit" = "create";
  private browseProjectID: number | null = null;
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

  private gxHost(): GxController | null {
    return (window as Window & { omniRecyclr?: GxController }).omniRecyclr ?? null;
  }

  openModal(html: string) {
    renderRecyclrBundle(this.gxHost(), "modal", html);
    const modal = document.querySelector('[data-chat-target="modal"]') as HTMLElement | null;
    modal?.classList.remove("hidden");
    modal?.classList.add("grid");
    const panel = document.querySelector('[data-chat-target="modalPanel"]') as HTMLElement | null;
    panel?.classList.remove("max-w-5xl");
    panel?.classList.add("max-w-6xl");
  }

  closeBrowse() {
    document.dispatchEvent(new CustomEvent("omni:modal-closed"));
    const modal = document.querySelector('[data-chat-target="modal"]') as HTMLElement | null;
    modal?.classList.add("hidden");
    modal?.classList.remove("grid");
  }

  async load() {
    this.setStatus("Loading projects…", "busy");
    try {
      const [projectsPayload, recipesPayload, workspace] = await Promise.all([
        fetchProjects(),
        fetchRecipes().catch(() => ({ recipes: [], root: "" })),
        fetchWorkspace().catch(() => ({ active_project_id: 0 })),
      ]);
      this.projects = projectsPayload.projects ?? [];
      this.recipes = recipesPayload.recipes ?? [];
      this.listTarget.innerHTML = renderProjectList(this.projects);
      this.updateActiveBadge(workspace.active_project_id);
      if (this.selectedProjectID) {
        await this.renderDetail(this.selectedProjectID);
      } else {
        this.detailTarget.innerHTML = "";
        this.detailTarget.classList.add("hidden");
        this.listTarget.classList.remove("hidden");
      }
      this.setStatus(`${this.projects.length} projects`, "ok");
    } catch (error) {
      this.listTarget.innerHTML = `<div class="rounded-xl border border-rose-400/20 bg-rose-400/5 p-6 text-sm text-rose-200">${error instanceof Error ? error.message : String(error)}</div>`;
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  updateActiveBadge(activeID: number) {
    const active = this.projects.find((project) => project.id === activeID);
    this.activeBadgeTarget.textContent = active ? active.name : "None selected";
  }

  showCreatePanel() {
    this.selectedProjectID = null;
    this.createPanelTarget.classList.remove("hidden");
    this.detailTarget.classList.add("hidden");
    this.listTarget.classList.remove("hidden");
    this.selectedPathTarget.value = "";
    this.createFormTarget.reset();
    this.populateRecipeSelect(this.createRecipeTarget);
  }

  populateRecipeSelect(select: HTMLSelectElement) {
    select.innerHTML = `<option value="">No catalog recipe</option>${this.recipes
      .map((recipe) => `<option value="${recipe.id}">${recipe.id}</option>`)
      .join("")}`;
  }

  async openBrowse(event: Event) {
    event.preventDefault();
    this.browseMode = "create";
    this.browseProjectID = null;
    await this.openBrowseAt("");
  }

  async browseForEdit(event: Event) {
    event.preventDefault();
    this.browseMode = "edit";
    this.browseProjectID = Number((event.currentTarget as HTMLElement).dataset.projectId || 0) || null;
    const location = (this.detailTarget.querySelector('[data-projects-field="location"]') as HTMLInputElement | null)?.value;
    await this.openBrowseAt(location || "");
  }

  async openBrowseAt(path: string) {
    this.setStatus("Browsing directories…", "busy");
    try {
      const data = await browseDirectory(path);
      this.browsePath = data.path;
      this.browseSelected = data.path;
      this.openModal(renderBrowseModal(data, this.browseSelected, this.browseMode));
      this.setStatus("Browse open", "idle");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async enterBrowseDir(event: Event) {
    event.preventDefault();
    const path = (event.currentTarget as HTMLElement).dataset.path || "";
    await this.openBrowseAt(path);
  }

  selectBrowseFile(event: Event) {
    event.preventDefault();
    this.browseSelected = (event.currentTarget as HTMLElement).dataset.path || this.browseSelected;
  }

  confirmBrowse(event: Event) {
    event.preventDefault();
    const path = (event.currentTarget as HTMLElement).dataset.path || this.browseSelected || this.browsePath;
    if (this.browseMode === "create") {
      this.selectedPathTarget.value = path;
      if (!this.createNameTarget.value.trim()) {
        this.createNameTarget.value = path.split("/").filter(Boolean).pop() || "project";
      }
    } else if (this.browseProjectID) {
      const input = this.detailTarget.querySelector('[data-projects-field="location"]') as HTMLInputElement | null;
      if (input) input.value = path;
    }
    this.closeBrowse();
  }

  async submitCreate(event: Event) {
    event.preventDefault();
    const location = this.selectedPathTarget.value.trim();
    const name = this.createNameTarget.value.trim();
    if (!location) {
      this.setStatus("Choose a working directory first", "error");
      return;
    }
    this.setStatus("Creating project…", "busy");
    try {
      const payload = await createProject({
        name: name || location.split("/").filter(Boolean).pop() || "project",
        location,
        description: this.createDescTarget.value.trim(),
        recipe_id: this.createRecipeTarget.value,
        activate: true,
      });
      this.selectedProjectID = payload.project.id;
      this.createPanelTarget.classList.add("hidden");
      await this.load();
      document.dispatchEvent(new CustomEvent("omni:project-activated", { detail: { project_id: payload.active_project_id } }));
      this.setStatus("Project created", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async openProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.selectedProjectID = id;
    this.createPanelTarget.classList.add("hidden");
    await this.renderDetail(id);
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
      );
      this.detailTarget.classList.remove("hidden");
      this.listTarget.classList.add("hidden");
      this.setStatus(project.name, "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  backToList() {
    this.selectedProjectID = null;
    this.detailTarget.classList.add("hidden");
    this.listTarget.classList.remove("hidden");
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

  async activateProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Activating…", "busy");
    try {
      const payload = await activateProject(id);
      document.dispatchEvent(new CustomEvent("omni:project-activated", { detail: { project_id: payload.active_project_id } }));
      await this.load();
      this.selectedProjectID = id;
      await this.renderDetail(id);
      this.setStatus("Active project updated", "ok");
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
