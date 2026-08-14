import { Controller } from "@hotwired/stimulus";
import { fetchProjectDetailComponent, fetchProjectsComponent } from "../lib/operational_component_api";
import { renderServerBundle, type ServerComponent } from "../lib/server_component_api";
import type RecyclrController from "./recyclr_controller";
import { ProjectBrowserCoordinator } from "../lib/project_browser_coordinator";
import { ProjectMutationCoordinator } from "../lib/project_mutation_coordinator";
import { closeModalShell, openModalShell } from "../lib/modal";
import { reportError, reportOk } from "../lib/feedback";

const PROJECT_TABS = new Set(["scrum", "terminal", "screen", "settings", "map", "git"]);

export default class ProjectsController extends Controller {
  static targets = ["list", "detail", "status", "viewingBadge"];
  declare readonly listTarget: HTMLElement;
  declare readonly detailTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly viewingBadgeTarget: HTMLElement;
  declare readonly hasListTarget: boolean;
  declare readonly hasDetailTarget: boolean;
  declare readonly hasStatusTarget: boolean;
  declare readonly hasViewingBadgeTarget: boolean;

  private selectedID: number | null = null;
  private activeTab = "scrum";
  private pageOffset = 0;
  private browser!: ProjectBrowserCoordinator;
  private mutations!: ProjectMutationCoordinator;
  private panelShownHandler: ((event: Event) => void) | null = null;
  private targetObserver: MutationObserver | null = null;
  private detailRequest: AbortController | null = null;

  connect(): void {
    this.browser = new ProjectBrowserCoordinator({
      detailRoot: () => this.detailTarget,
      modalPanel: () => document.querySelector('[data-chat-target="modalPanel"]'),
      openModal: (component) => this.openModal(component),
      closeModal: () => this.closeBrowse(),
      setStatus: (message, tone) => this.setStatus(message, tone),
      projectCreated: async (project) => {
        this.selectedID = project.id;
        this.setActiveTab("scrum");
        await this.loadDetail();
      },
      reloadProjects: () => this.loadListAuthority(),
    });
    this.mutations = new ProjectMutationCoordinator({
      detailRoot: () => this.detailTarget,
      reloadDetail: () => this.loadDetail(),
      reloadList: () => this.loadListAuthority(),
      projectDeleted: () => this.backToList(),
      setStatus: (message, tone) => this.setStatus(message, tone),
      success: (message) => reportOk(this.setStatus.bind(this), message),
      failure: (error) => reportError(this.setStatus.bind(this), error),
    });
    this.panelShownHandler = (event) => {
      if ((event as CustomEvent<{ panel?: string }>).detail?.panel === "projects") this.loadWhenMounted();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
  }

  disconnect(): void {
    if (this.panelShownHandler) document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    this.targetObserver?.disconnect();
    this.detailRequest?.abort();
  }

  private loadWhenMounted(): void {
    if (this.hasListTarget && this.hasDetailTarget && this.hasStatusTarget) { void this.load(); return; }
    this.targetObserver?.disconnect();
    this.targetObserver = new MutationObserver(() => {
      if (!this.hasListTarget || !this.hasDetailTarget || !this.hasStatusTarget) return;
      this.targetObserver?.disconnect();
      this.targetObserver = null;
      void this.load();
    });
    this.targetObserver.observe(this.element, { childList: true, subtree: true });
  }

  private recyclrController(): RecyclrController {
    const controller = this.application.getControllerForElementAndIdentifier(document.body, "recyclr") as RecyclrController | null;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  async openModal(component: ServerComponent): Promise<void> {
    await renderServerBundle(this.recyclrController(), component, "Project modal");
    openModalShell({ wide: true });
  }

  closeBrowse(): void { closeModalShell(); }
  closeCreateModal(): void { this.closeBrowse(); }

  setStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle"): void {
    if (!this.hasStatusTarget) throw new Error("Projects status target is unavailable.");
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.statusTarget.textContent = message;
    this.statusTarget.className = `text-xs ${classes[tone]}`;
  }

  async load(): Promise<void> {
    this.setStatus("Loading projects…", "busy");
    try {
      await this.loadListAuthority();
      if (this.selectedID) await this.loadDetail();
    } catch (error) { reportError(this.setStatus.bind(this), error); }
  }

  private async loadListAuthority(): Promise<void> {
    const payload = await fetchProjectsComponent(this.pageOffset);
    await renderServerBundle(this.recyclrController(), payload, "Projects component");
    this.setStatus(`${payload.count} projects on this page`, "ok");
  }

  loadProjectPage(event: Event): void {
    event.preventDefault();
    const offset = this.requiredDatasetInteger(event, "pageOffset", "Project page offset", true);
    if (!Number.isSafeInteger(offset) || offset < 0) throw new Error("Project page offset is invalid.");
    this.pageOffset = offset;
    void this.load();
  }

  async openProject(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.requiredDatasetInteger(event, "projectId", "Open project", false);
    this.selectedID = id;
    this.setActiveTab(this.resolveTab(id));
    await this.loadDetail();
  }

  async showTab(event: Event): Promise<void> {
    event.preventDefault();
    this.setActiveTab(this.requiredDatasetString(event, "projectTab", "Project tab"));
    await this.loadDetail();
  }

  private setActiveTab(tab: string): void {
    if (!PROJECT_TABS.has(tab)) throw new Error(`Unsupported project tab ${JSON.stringify(tab)}.`);
    this.activeTab = tab;
    if (this.selectedID) sessionStorage.setItem(`omni.project.tab.${this.selectedID}`, tab);
    const url = new URL(window.location.href);
    url.searchParams.set("project_tab", tab);
    history.replaceState(null, document.title, `${url.pathname}${url.search}${url.hash}`);
  }

  private resolveTab(id: number): string {
    const query = new URLSearchParams(window.location.search);
    const queryTabs = query.getAll("project_tab");
    if (queryTabs.length > 1) throw new Error("Project tab query must not be duplicated.");
    const fromURL = queryTabs.length === 1 ? queryTabs[0] : null;
    const stored = sessionStorage.getItem(`omni.project.tab.${id}`);
    const tab = fromURL !== null ? fromURL : stored !== null ? stored : "scrum";
    if (!PROJECT_TABS.has(tab)) throw new Error(`Unsupported stored project tab ${JSON.stringify(tab)}.`);
    return tab;
  }

  async loadDetail(): Promise<void> {
    if (!this.selectedID) return;
    this.detailRequest?.abort();
    this.detailRequest = new AbortController();
		const payload = await fetchProjectDetailComponent(this.selectedID, this.activeTab, this.detailRequest.signal);
    await renderServerBundle(this.recyclrController(), payload, "Project detail");
    this.detailTarget.classList.remove("hidden"); this.detailTarget.classList.add("flex");
    this.listTarget.classList.add("hidden");
    this.setStatus(payload.project_name, "ok");
    if (this.hasViewingBadgeTarget) this.viewingBadgeTarget.textContent = payload.project_name;
    document.dispatchEvent(new CustomEvent("omni:project-opened", { detail: { project_id: payload.project_id, name: payload.project_name, location: payload.project_location } }));
    document.dispatchEvent(new CustomEvent("omni:project-tab", { detail: { tab: payload.tab, project_id: payload.project_id } }));
  }

  backToList(): void {
    this.selectedID = null;
    if (this.hasDetailTarget) { this.detailTarget.classList.add("hidden"); this.detailTarget.classList.remove("flex"); }
    if (this.hasListTarget) this.listTarget.classList.remove("hidden");
    if (this.hasViewingBadgeTarget) this.viewingBadgeTarget.textContent = "None selected";
    document.dispatchEvent(new CustomEvent("omni:project-closed"));
  }

  private requiredDatasetString(event: Event, key: string, label: string): string {
    const target = event.currentTarget;
    if (!(target instanceof HTMLElement)) throw new Error(`${label} requires one server-rendered control.`);
    const value = target.dataset[key];
    if (value === undefined || value === "" || value !== value.trim() || value.includes("\0")) {
      throw new Error(`${label} is missing exact server authority.`);
    }
    return value;
  }

  private requiredDatasetInteger(event: Event, key: string, label: string, allowZero: boolean): number {
    const raw = this.requiredDatasetString(event, key, label);
    if (!/^(0|[1-9][0-9]*)$/.test(raw) || (!allowZero && raw === "0")) {
      throw new Error(`${label} is not a canonical integer.`);
    }
    const value = Number(raw);
    if (!Number.isSafeInteger(value)) throw new Error(`${label} exceeds the safe integer bound.`);
    return value;
  }

  showCreateModal(): Promise<void> { return this.browser.showCreateModal(); }
  openBrowse(event: Event): Promise<void> { return this.browser.openBrowse(event); }
  browseForEdit(event: Event): Promise<void> { return this.browser.browseForEdit(event); }
  enterBrowseDir(event: Event): Promise<void> { return this.browser.enterBrowseDir(event); }
  loadBrowsePage(event: Event): Promise<void> { return this.browser.loadBrowsePage(event); }
  selectBrowseDir(event: Event): Promise<void> { return this.browser.selectBrowseDir(event); }
  createBrowseFolder(event: Event): Promise<void> { return this.browser.createBrowseFolder(event); }
  confirmBrowse(event: Event): Promise<void> { return this.browser.confirmBrowse(event); }
  submitCreate(event: Event): Promise<void> { return this.browser.submitCreate(event); }
  saveProject(event: Event): Promise<void> { return this.mutations.saveProject(event); }
  saveModelConfig(event: Event): Promise<void> { return this.mutations.saveModelConfig(event); }
  clearModelConfig(event: Event): Promise<void> { return this.mutations.clearModelConfig(event); }
  saveScrumAutomation(event: Event): Promise<void> { return this.mutations.saveScrumAutomation(event); }
  rescanProject(event: Event): Promise<void> { return this.mutations.rescanProject(event); }
  scanProjectMap(event: Event): Promise<void> { return this.mutations.scanProjectMap(event); }
  refreshProjectGit(event: Event): Promise<void> { return this.mutations.refreshProjectGit(event); }
  startAutoWork(event: Event): Promise<void> { return this.mutations.startAutoWork(event); }
  pauseAutoWork(event: Event): Promise<void> { return this.mutations.pauseAutoWork(event); }
  deleteProject(event: Event): Promise<void> { return this.mutations.deleteProject(event); }
}
