import { Controller } from "@hotwired/stimulus";
import {
  browseDirectory,
  createBrowseDirectory,
  fetchHostBridgeStatus,
  createProject,
  deleteProject,
  fetchProjects,
  fetchProject,
  fetchProjectDebuggerStatus,
  fetchProjectMap,
  fetchRecipes,
  runProjectDebugger,
  scanProjectMap,
  surveyProject,
  updateProject,
} from "../lib/project_api";
import { renderBrowseModal, renderProjectCreateModal, renderProjectDetail, renderProjectList } from "../lib/project_render";
import { renderProjectDebuggerModal } from "../lib/project_debugger_render";
import { patchScrumAutoReview } from "../lib/scrum_api";
import { fetchJobRecord } from "../lib/data_api";
import { collectModelFieldValues, clearModelFieldInputs } from "../lib/model_config_render";
import { collectAgentFieldValues, clearAgentFieldInputs } from "../lib/agent_config_render";
import { fetchAgentDefaults } from "../lib/agent_config_api";
import type { ResolvedAgentConfig } from "../lib/agent_config_types";
import { renderRecyclrBundle } from "../lib/recyclr";
import { closeModalShell, openModalShell } from "../lib/modal";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { t } from "../lib/i18n";
import { showToast } from "../lib/toast";
import type { ResolvedModelConfig } from "../lib/model_config_types";
import type { BrowseResponse, DebuggerLastRun, ProjectMapSummary, ProjectRecord, RecipeCatalogItem } from "../lib/project_types";
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
  private debuggerProjectID: number | null = null;
  private debuggerProjectName = "";
  private debuggerLastRun: DebuggerLastRun | null = null;
  private debuggerRunning = false;
  private debuggerPollTimer: number | null = null;

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
    this.stopDebuggerPolling();
  }

  setStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.statusTarget.textContent = message;
    this.statusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  private actionOk(message: string) {
    reportOk(this.setStatus.bind(this), message);
  }

  private actionFail(error: unknown) {
    reportError(this.setStatus.bind(this), error);
  }

  private actionFailMessage(message: string) {
    reportErrorMessage(this.setStatus.bind(this), message);
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
    if (message && (tone === "ok" || tone === "error")) {
      showToast(message, tone);
    }
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
    this.openBadgeTarget.textContent = name?.trim() || t("session.noneOpen");
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

  /** Backend auto-sync runs on GET project; refresh map summary once it likely finished. */
  private async refreshProjectMapAfterAutoSync(projectID: number) {
    await new Promise((resolve) => window.setTimeout(resolve, 2500));
    if (this.selectedProjectID !== projectID) return;
    try {
      this.currentProjectMap = await fetchProjectMap(projectID);
      if (this.activeTab === "map") {
        await this.renderDetail(projectID);
      }
    } catch {
      // map refresh is best-effort after background sync
    }
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
      await this.load();
      this.actionOk(`Project “${createdName}” created`);
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
      this.detailTarget.classList.add("flex");
      this.listTarget.classList.add("hidden");
      this.dispatchProjectOpened(project);
      void this.refreshProjectMapAfterAutoSync(id);
      this.setStatus(project.name, "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  backToList() {
    this.selectedProjectID = null;
    this.activeTab = "scrum";
    this.detailTarget.classList.add("hidden");
    this.detailTarget.classList.remove("flex");
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
      this.actionOk("Project saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionFailMessage("Recipe JSON is invalid");
      return;
    }
    this.setStatus("Saving recipe…", "busy");
    try {
      await updateProject(id, { recipe_id: this.fieldValue("recipeId"), recipe });
      await this.renderDetail(id);
      this.actionOk("Recipe saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Model settings saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Model overrides cleared");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Agent settings saved");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async saveScrumAutomation(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    const enabled = Boolean(
      (this.detailTarget.querySelector('[data-projects-field="autoReviewEnabled"]') as HTMLInputElement | null)?.checked,
    );
    const bounceColumn =
      (this.detailTarget.querySelector('[data-projects-field="autoReviewBounce"]') as HTMLSelectElement | null)?.value?.trim() ||
      "assigned";
    this.setStatus("Saving scrum automation…", "busy");
    try {
      await patchScrumAutoReview({ enabled, bounce_column: bounceColumn }, id);
      await this.renderDetail(id);
      this.actionOk("Scrum automation saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Agent overrides cleared");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Project stack detected");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk(this.currentProjectMap.message || "Codebase map updated");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Project deleted");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async openDebuggerModal(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || this.selectedProjectID || 0);
    if (!id) return;
    this.debuggerProjectID = id;
    this.debuggerProjectName =
      this.detailTarget.querySelector("h3")?.textContent?.trim() ||
      this.projects.find((project) => project.id === id)?.name ||
      "Project";
    this.setStatus("Loading debugger…", "busy");
    try {
      const payload = await fetchProjectDebuggerStatus(id);
      this.debuggerLastRun = payload.last_run ?? null;
      this.debuggerRunning = payload.last_run?.status === "running";
      this.openModal(this.debuggerModalHTML(payload.agent_config));
      if (this.debuggerRunning && payload.last_run?.job_id) {
        this.startDebuggerPolling(id, payload.last_run.job_id);
      }
      this.setStatus(this.debuggerProjectName, "ok");
    } catch (error) {
      this.actionFail(error);
    }
  }

  closeDebuggerModal() {
    this.stopDebuggerPolling();
    this.closeBrowse();
  }

  async runDebugger(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || this.debuggerProjectID || 0);
    if (!id || this.debuggerRunning) return;
    this.debuggerRunning = true;
    this.refreshDebuggerModal();
    this.setStatus("Starting debugger scan…", "busy");
    try {
      const payload = await runProjectDebugger(id);
      this.debuggerLastRun = payload.last_run;
      this.refreshDebuggerModal();
      this.startDebuggerPolling(id, payload.job.id);
      this.actionOk(payload.message || `Debugger job #${payload.job.id} queued`);
    } catch (error) {
      this.debuggerRunning = false;
      this.refreshDebuggerModal();
      this.actionFail(error);
    }
  }

  private debuggerModalHTML(agentConfig?: Record<string, unknown>): string {
    if (!this.debuggerProjectID) return "";
    const resolved = (agentConfig?.resolved as Record<string, unknown> | undefined) ?? {};
    const system =
      (typeof agentConfig?.system === "string" && agentConfig.system) ||
      (typeof resolved.system === "string" && resolved.system) ||
      this.currentAgentConfig?.system ||
      "omnidex";
    const source =
      (typeof agentConfig?.source === "string" && agentConfig.source) ||
      this.currentAgentConfig?.source ||
      "env";
    return renderProjectDebuggerModal({
      projectID: this.debuggerProjectID,
      projectName: this.debuggerProjectName,
      agentSystem: system,
      agentSource: source,
      lastRun: this.debuggerLastRun,
      running: this.debuggerRunning,
      statusText: this.debuggerRunning
        ? "Scanning codebase map and backlog for bugs…"
        : this.debuggerLastRun?.summary || "",
    });
  }

  private refreshDebuggerModal(agentConfig?: Record<string, unknown>) {
    const panel = this.modalPanel();
    if (!panel || !this.debuggerProjectID) return;
    panel.innerHTML = this.debuggerModalHTML(agentConfig);
  }

  private startDebuggerPolling(projectID: number, jobID: number) {
    this.stopDebuggerPolling();
    const tick = async () => {
      try {
        const [jobDetails, statusPayload] = await Promise.all([
          fetchJobRecord(jobID),
          fetchProjectDebuggerStatus(projectID),
        ]);
        const jobStatus = jobDetails.job?.status || "";
        this.debuggerLastRun = statusPayload.last_run ?? this.debuggerLastRun;
        if (jobStatus === "completed" || jobStatus === "failed" || jobStatus === "canceled") {
          this.debuggerRunning = false;
          this.refreshDebuggerModal(statusPayload.agent_config);
          this.stopDebuggerPolling();
          if (jobStatus === "completed") {
            this.actionOk(`Debugger finished — ${this.debuggerLastRun?.cards_created?.length ?? 0} ticket(s) created`);
            if (this.selectedProjectID === projectID && this.activeTab === "scrum") {
              document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: projectID } }));
            }
          } else {
            this.setStatus(this.debuggerLastRun?.error || `Debugger ${jobStatus}`, "error");
            this.refreshDebuggerModal(statusPayload.agent_config);
          }
          return;
        }
        this.debuggerRunning = true;
        this.refreshDebuggerModal(statusPayload.agent_config);
      } catch {
        /* keep polling */
      }
    };
    void tick();
    this.debuggerPollTimer = window.setInterval(() => void tick(), 900);
  }

  private stopDebuggerPolling() {
    if (this.debuggerPollTimer != null) {
      window.clearInterval(this.debuggerPollTimer);
      this.debuggerPollTimer = null;
    }
  }
}
