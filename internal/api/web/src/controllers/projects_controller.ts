import { Controller } from "@hotwired/stimulus";
import {
  deleteProject,
  fetchProjects,
  fetchProject,
  fetchProjectGit,
  fetchProjectMap,
  fetchRecipes,
  pauseProjectAutoWork,
  scanProjectMap,
  startProjectAutoWork,
  surveyProject,
  updateProject,
} from "../lib/project_api";
import { renderProjectDetail, renderProjectList } from "../lib/project_render";
import { renderProjectGitSection } from "../lib/project_git_render";
import { patchScrumAutomation } from "../lib/scrum_api";
import { collectModelFieldValues, clearModelFieldInputs } from "../lib/model_config_render";
import { collectAgentFieldValues, clearAgentFieldInputs } from "../lib/agent_config_render";
import { fetchAgentDefaults } from "../lib/agent_config_api";
import type { ResolvedAgentConfig } from "../lib/agent_config_types";
import { renderRecyclrBundle } from "../lib/recyclr";
import { closeModalShell, openModalShell } from "../lib/modal";
import { setGlobalLoading } from "../lib/loading";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { t } from "../lib/i18n";
import { showToast } from "../lib/toast";
import { escapeHTML } from "../lib/dom";
import type { ResolvedModelConfig } from "../lib/model_config_types";
import type { ProjectGitStatus, ProjectMapSummary, ProjectRecord, RecipeCatalogItem } from "../lib/project_types";
import type RecyclrController from "./recyclr_controller";
import { ProjectBrowserCoordinator } from "../lib/project_browser_coordinator";
import { ProjectDebuggerCoordinator } from "../lib/project_debugger_coordinator";

const PROJECT_TABS = new Set(["scrum", "chat", "terminal", "screen", "settings", "map", "git", "recipe"]);

export default class ProjectsController extends Controller {
  static targets = ["list", "detail", "status", "viewingBadge"];

  declare readonly listTarget: HTMLElement;
  declare readonly detailTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly hasListTarget: boolean;
  declare readonly hasDetailTarget: boolean;
  declare readonly hasStatusTarget: boolean;
  declare readonly hasViewingBadgeTarget: boolean;
  declare readonly viewingBadgeTarget: HTMLElement;

  private projects: ProjectRecord[] = [];
  private recipes: RecipeCatalogItem[] = [];
  private selectedProjectID: number | null = null;
  private activeTab = "scrum";
  private panelShownHandler: ((event: Event) => void) | null = null;
  private currentModelConfig: ResolvedModelConfig | null = null;
  private currentAgentConfig: ResolvedAgentConfig | null = null;
  private currentProjectMap: ProjectMapSummary | null = null;
  private currentProjectGit: ProjectGitStatus | null = null;
  private browser!: ProjectBrowserCoordinator;
  private debugger!: ProjectDebuggerCoordinator;
  private detailAbortController: AbortController | null = null;
  private pendingProjectsLoad = false;
  private pendingProjectsLoadReason = "";
  private pendingProjectsLoadTimer: number | null = null;
  private projectsPanelObserver: MutationObserver | null = null;
  private loadInFlight = false;

  connect() {
    this.browser = new ProjectBrowserCoordinator({
      recipes: () => this.recipes,
      detailRoot: () => this.detailTarget,
      modalPanel: () => this.modalPanel(),
      openModal: (html) => this.openModal(html),
      closeModal: () => this.closeBrowse(),
      setStatus: (message, tone) => this.setStatus(message, tone),
      projectCreated: async (project) => {
        this.selectedProjectID = project.id;
        this.setActiveProjectTab("scrum");
        await this.load();
        this.actionOk(`Project “${project.name || "project"}” created`);
      },
    });
    this.debugger = new ProjectDebuggerCoordinator({
      selectedProjectID: () => this.selectedProjectID,
      activeTab: () => this.activeTab,
      projectName: (projectID) =>
        this.detailTarget.querySelector("h3")?.textContent?.trim() ||
        this.projects.find((project) => project.id === projectID)?.name ||
        "Project",
      agentConfig: () => this.currentAgentConfig,
      modalPanel: () => this.modalPanel(),
      openModal: (html) => this.openModal(html),
      closeModal: () => this.closeBrowse(),
      setStatus: (message, tone) => this.setStatus(message, tone),
      actionOk: (message) => this.actionOk(message),
      actionFail: (error) => this.actionFail(error),
      refreshScrum: (projectID) => {
        document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: projectID } }));
      },
    });
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "projects") {
        this.requestProjectsLoad("panel-shown");
        return;
      }
      this.cancelPendingProjectsLoad();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
  }

  disconnect() {
    if (this.panelShownHandler) {
      document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    }
    this.cancelPendingProjectsLoad();
    this.projectsPanelObserver?.disconnect();
    this.projectsPanelObserver = null;
    this.detailAbortController?.abort();
    this.debugger?.disconnect();
  }

  private hasProjectsPanelTargets(): boolean {
    return this.hasStatusTarget && this.hasListTarget && this.hasDetailTarget;
  }

  private observePendingProjectsLoad() {
    if (this.projectsPanelObserver) return;
    this.projectsPanelObserver = new MutationObserver(() => this.flushPendingProjectsLoad());
    this.projectsPanelObserver.observe(this.element, { childList: true, subtree: true });
  }

  private clearPendingProjectsLoadTimer() {
    if (this.pendingProjectsLoadTimer === null) return;
    window.clearTimeout(this.pendingProjectsLoadTimer);
    this.pendingProjectsLoadTimer = null;
  }

  private cancelPendingProjectsLoad() {
    this.pendingProjectsLoad = false;
    this.pendingProjectsLoadReason = "";
    this.clearPendingProjectsLoadTimer();
    this.projectsPanelObserver?.disconnect();
    this.projectsPanelObserver = null;
  }

  private requestProjectsLoad(reason: string) {
    this.pendingProjectsLoad = true;
    this.pendingProjectsLoadReason = reason;
    this.observePendingProjectsLoad();
    this.flushPendingProjectsLoad();
    if (!this.pendingProjectsLoad || this.pendingProjectsLoadTimer !== null) return;
    this.pendingProjectsLoadTimer = window.setTimeout(() => {
      this.pendingProjectsLoadTimer = null;
      if (!this.pendingProjectsLoad) return;
      this.pendingProjectsLoad = false;
      const message = "Projects panel failed to load because required DOM targets were not mounted.";
      console.error(message, {
        reason: this.pendingProjectsLoadReason,
        hasStatusTarget: this.hasStatusTarget,
        hasListTarget: this.hasListTarget,
        hasDetailTarget: this.hasDetailTarget,
      });
      if (this.hasStatusTarget) this.setStatus(message, "error");
      showToast(message, "error");
    }, 2000);
  }

  private flushPendingProjectsLoad() {
    if (!this.pendingProjectsLoad || !this.hasProjectsPanelTargets()) return;
    this.cancelPendingProjectsLoad();
    void this.load();
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

  private isAbortError(error: unknown): boolean {
    return error instanceof DOMException && error.name === "AbortError";
  }

  private nextDetailSignal(): { signal: AbortSignal; cleanup: () => void } {
    this.detailAbortController?.abort();
    const controller = new AbortController();
    this.detailAbortController = controller;
    const timeout = window.setTimeout(() => controller.abort(), 20000);
    return {
      signal: controller.signal,
      cleanup: () => {
        window.clearTimeout(timeout);
        if (this.detailAbortController === controller) {
          this.detailAbortController = null;
        }
      },
    };
  }

  private recyclrHost(): RecyclrController {
    const controller = (window as Window & { omniRecyclr?: RecyclrController }).omniRecyclr;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  async openModal(html: string): Promise<void> {
    await renderRecyclrBundle(this.recyclrHost(), "modal", html);
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

  updateViewingBadge(name: string | null) {
    if (!this.hasViewingBadgeTarget) return;
    this.viewingBadgeTarget.textContent = name?.trim() || t("session.noneViewing");
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
    this.updateViewingBadge(project.name);
  }

  private dispatchProjectClosed() {
    document.dispatchEvent(new CustomEvent("omni:project-closed"));
    this.updateViewingBadge(null);
  }

  private projectTabSessionKey(projectID?: number | null): string {
    return `omni.project.tab.${projectID ?? "global"}`;
  }

  private normalizeProjectTab(tab?: string | null): string {
    const next = tab?.trim() || "";
    if (!next) return "scrum";
    if (!PROJECT_TABS.has(next)) throw new Error(`Unknown project tab ${JSON.stringify(next)}.`);
    return next;
  }

  private resolveProjectTab(projectID?: number | null): string {
    const params = new URLSearchParams(window.location.search);
    const fromURL = params.get("project_tab");
    if (fromURL) return this.normalizeProjectTab(fromURL);
    return this.normalizeProjectTab(sessionStorage.getItem(this.projectTabSessionKey(projectID)));
  }

  private setActiveProjectTab(tab: string, updateURL = true) {
    this.activeTab = this.normalizeProjectTab(tab);
    sessionStorage.setItem(this.projectTabSessionKey(this.selectedProjectID), this.activeTab);
    if (!updateURL) return;
    const url = new URL(window.location.href);
    url.searchParams.set("project_tab", this.activeTab);
    history.replaceState(null, document.title, `${url.pathname}${url.search}${url.hash}`);
  }

  async load() {
    if (!this.hasProjectsPanelTargets()) {
      this.requestProjectsLoad("load-before-targets");
      return;
    }
    if (this.loadInFlight) return;
    this.loadInFlight = true;
    this.setStatus("Loading projects…", "busy");
    setGlobalLoading(true);
    try {
      const [projectsPayload, recipesPayload] = await Promise.all([
        fetchProjects(),
        fetchRecipes(),
      ]);
      this.projects = projectsPayload.projects ?? [];
      this.recipes = recipesPayload.recipes ?? [];
      this.listTarget.innerHTML = renderProjectList(this.projects);
      if (this.selectedProjectID) {
        this.setActiveProjectTab(this.resolveProjectTab(this.selectedProjectID), false);
        await this.renderDetail(this.selectedProjectID);
      } else {
        this.detailTarget.innerHTML = "";
        this.detailTarget.classList.add("hidden");
        this.listTarget.classList.remove("hidden");
        this.dispatchProjectClosed();
      }
      this.setStatus(`${this.projects.length} projects`, "ok");
    } catch (error) {
      this.listTarget.innerHTML = `<div class="rounded-xl border border-rose-400/20 bg-rose-400/5 p-6 text-sm text-rose-200">${escapeHTML(error instanceof Error ? error.message : String(error))}</div>`;
      this.actionFail(error);
    } finally {
      setGlobalLoading(false);
      this.loadInFlight = false;
    }
  }

  async showCreateModal() {
    await this.runBrowserAction(() => this.browser.showCreateModal());
  }

  async openBrowse(event: Event) {
    await this.runBrowserAction(() => this.browser.openBrowse(event));
  }

  async browseForEdit(event: Event) {
    await this.runBrowserAction(() => this.browser.browseForEdit(event));
  }

  async enterBrowseDir(event: Event) {
    await this.runBrowserAction(() => this.browser.enterBrowseDir(event));
  }

  async selectBrowseDir(event: Event) {
    await this.runBrowserAction(() => this.browser.selectBrowseDir(event));
  }

  async createBrowseFolder(event: Event) {
    await this.runBrowserAction(() => this.browser.createBrowseFolder(event));
  }

  async confirmBrowse(event: Event) {
    await this.runBrowserAction(() => this.browser.confirmBrowse(event));
  }

  async submitCreate(event: Event) {
    await this.runBrowserAction(() => this.browser.submitCreate(event));
  }

  private async runBrowserAction(action: () => Promise<void>): Promise<void> {
    try {
      await action();
    } catch (error) {
      this.actionFail(error);
    }
  }

  async openProject(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    const previousProjectID = this.selectedProjectID;
    const previousTab = this.activeTab;
    this.selectedProjectID = id;
    this.setActiveProjectTab(this.resolveProjectTab(id));
    const loaded = await this.renderDetail(id);
    if (!loaded) {
      this.selectedProjectID = previousProjectID;
      this.activeTab = previousTab;
    }
  }

  private applyTabState() {
    const tab = this.activeTab;
    this.detailTarget.querySelectorAll("[data-project-tab]").forEach((button) => {
      const active = button.getAttribute("data-project-tab") === tab;
      button.classList.toggle("border-cyan-300/40", active);
      button.classList.toggle("bg-cyan-300/10", active);
      button.classList.toggle("text-cyan-100", active);
      button.classList.toggle("border-white/10", !active);
      button.classList.toggle("text-zinc-400", !active);
    });
  }

  async showTab(event: Event) {
    event.preventDefault();
    const tab = (event.currentTarget as HTMLElement).dataset.projectTab || "scrum";
    this.setActiveProjectTab(tab);
    if (this.selectedProjectID) {
      await this.renderDetail(this.selectedProjectID, { preserveStatus: true });
    } else {
      this.applyTabState();
    }
    document.dispatchEvent(
      new CustomEvent("omni:project-tab", {
        detail: { tab: this.activeTab, project_id: this.selectedProjectID },
      }),
    );
  }

  private async refreshProjectGitPanel(projectID: number) {
    const panel = this.detailTarget.querySelector('[data-project-tab-panel="git"]');
    if (!panel) return;
    this.currentProjectGit = await fetchProjectGit(projectID);
    panel.innerHTML = renderProjectGitSection(projectID, this.currentProjectGit);
  }

  async refreshProjectGit(event: Event) {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Refreshing git status…", "busy");
    try {
      await this.refreshProjectGitPanel(id);
      this.actionOk("Git status refreshed");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async renderDetail(id: number, options: { preserveStatus?: boolean; showLoading?: boolean } = {}): Promise<boolean> {
    const showLoading = options.showLoading ?? true;
    const request = this.nextDetailSignal();
    if (!options.preserveStatus) {
      this.setStatus("Loading project…", "busy");
    }
    if (showLoading) setGlobalLoading(true);
    try {
      const needsSettings = this.activeTab === "settings";
      const needsMap = this.activeTab === "map";
      const needsGit = this.activeTab === "git";
      const [{ project, modelConfig }, agentPayload, projectMap, projectGit] = await Promise.all([
        fetchProject(id, request.signal),
        needsSettings ? fetchAgentDefaults(id, undefined, request.signal).catch((error) => (this.isAbortError(error) ? Promise.reject(error) : null)) : Promise.resolve(null),
        needsMap ? fetchProjectMap(id, request.signal).catch((error) => (this.isAbortError(error) ? Promise.reject(error) : null)) : Promise.resolve(this.currentProjectMap),
        needsGit ? fetchProjectGit(id, request.signal).catch((error) => (this.isAbortError(error) ? Promise.reject(error) : null)) : Promise.resolve(this.currentProjectGit),
      ]);
      this.currentModelConfig = modelConfig ?? null;
      this.currentAgentConfig = agentPayload?.resolved ?? null;
      this.currentProjectMap = projectMap;
      if (projectGit) {
        this.currentProjectGit = projectGit;
      }
      this.detailTarget.innerHTML = renderProjectDetail(
        project,
        this.recipes,
        modelConfig?.fields ?? [],
        modelConfig?.source ?? "env",
        agentPayload?.resolved?.fields ?? agentPayload?.fields ?? [],
        agentPayload?.resolved?.source ?? "env",
        agentPayload?.resolved?.system ?? "omnidex",
        projectMap,
        this.currentProjectGit,
        this.activeTab,
      );
      this.applyTabState();
      document.dispatchEvent(
        new CustomEvent("omni:project-tab", {
          detail: { tab: this.activeTab, project_id: id },
        }),
      );
      this.detailTarget.classList.remove("hidden");
      this.detailTarget.classList.add("flex");
      this.listTarget.classList.add("hidden");
      this.dispatchProjectOpened(project);
      if (!options.preserveStatus) {
        this.setStatus(project.name, "ok");
      }
      return true;
    } catch (error) {
      if (this.isAbortError(error)) {
        return false;
      }
      this.actionFail(error);
      return false;
    } finally {
      if (showLoading) setGlobalLoading(false);
      request.cleanup();
    }
  }

  backToList() {
    this.selectedProjectID = null;
    this.setActiveProjectTab("scrum", false);
    this.currentProjectGit = null;
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
    const autoReviewEnabled = Boolean(
      (this.detailTarget.querySelector('[data-projects-field="autoReviewEnabled"]') as HTMLInputElement | null)?.checked,
    );
    const autoWorkEnabled = Boolean(
      (this.detailTarget.querySelector('[data-projects-field="autoWorkEnabled"]') as HTMLInputElement | null)?.checked,
    );
    const createTicketEnabled = Boolean(
      (this.detailTarget.querySelector('[data-projects-field="createTicketEnabled"]') as HTMLInputElement | null)?.checked,
    );
    const sourceColumns = Array.from(this.detailTarget.querySelectorAll('[data-projects-field="autoWorkColumn"]'))
      .filter((node): node is HTMLInputElement => node instanceof HTMLInputElement && node.checked)
      .map((node) => node.dataset.autoWorkColumn?.trim() || "")
      .filter(Boolean);
    const bounceColumn =
      (this.detailTarget.querySelector('[data-projects-field="autoReviewBounce"]') as HTMLSelectElement | null)?.value?.trim() ||
      "assigned";
    const createTicketColumn =
      (this.detailTarget.querySelector('[data-projects-field="createTicketColumn"]') as HTMLSelectElement | null)?.value?.trim() ||
      "backlog";
    this.setStatus("Saving scrum automation…", "busy");
    try {
      await patchScrumAutomation(
        {
          auto_work: { enabled: autoWorkEnabled, source_columns: sourceColumns },
          auto_review: { enabled: autoReviewEnabled, bounce_column: bounceColumn },
          create_ticket: { enabled: createTicketEnabled, column: createTicketColumn },
        },
        id,
      );
      await this.renderDetail(id);
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: id } }));
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

  async startAutoWork(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Starting auto-work…", "busy");
    try {
      const payload = await startProjectAutoWork(id);
      await this.load();
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: id } }));
      this.actionOk(payload.message || "Auto-work started");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async pauseAutoWork(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const id = Number((event.currentTarget as HTMLElement).dataset.projectId || 0);
    if (!id) return;
    this.setStatus("Pausing auto-work…", "busy");
    try {
      const payload = await pauseProjectAutoWork(id);
      await this.load();
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: id } }));
      this.actionOk(payload.message || "Auto-work paused");
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
    await this.debugger.open(event);
  }

  closeDebuggerModal() {
    this.debugger.close();
  }

  async runDebugger(event: Event) {
    await this.debugger.run(event);
  }
}
