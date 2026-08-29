import { Controller } from "@hotwired/stimulus";
import {
  ingestDocuments,
  saveAPISecrets,
  saveModelSettings,
  saveNetworkSettings,
} from "../lib/admin_api";
import { deleteOllamaModel, pullOllamaModel } from "../lib/ollama_model_api";
import { fetchAdminComponent } from "../lib/operational_component_api";
import { renderServerBundle } from "../lib/server_component_api";
import type RecyclrController from "./recyclr_controller";
import type ChatController from "./chat_controller";
import { panelHref, parseAdminTabFromLocation } from "../lib/panel_routing";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { setGlobalLoading } from "../lib/loading";

const ADMIN_TABS = new Set(["overview", "ai", "datasources", "health", "advanced"]);

export default class AdminController extends Controller {
  static targets = ["tabNav", "tabPanel", "adminStatus", "pullModel", "catalogQuery", "ingestFiles", "ingestStage", "ingestTags"];

  declare readonly tabNavTarget: HTMLElement;
  declare readonly adminStatusTarget: HTMLElement;
  declare readonly pullModelTarget: HTMLInputElement;
  declare readonly hasPullModelTarget: boolean;
  declare readonly catalogQueryTarget: HTMLInputElement;
  declare readonly hasCatalogQueryTarget: boolean;
  declare readonly ingestFilesTarget: HTMLInputElement;
  declare readonly hasIngestFilesTarget: boolean;
  declare readonly ingestStageTarget: HTMLSelectElement;
  declare readonly hasIngestStageTarget: boolean;
  declare readonly ingestTagsTarget: HTMLInputElement;
  declare readonly hasIngestTagsTarget: boolean;

  private activeTab = "overview";
  private modelOffset = 0;
  private catalogQuery = "";
  private catalogPage = 1;
  private downloadOffset = 0;
  private panelShownHandler: ((event: Event) => void) | null = null;
  private ollamaDownloadHandler: ((event: Event) => void) | null = null;
  private ollamaRefreshTimer: number | null = null;

  connect(): void {
    const tab = parseAdminTabFromLocation();
    if (tab && ADMIN_TABS.has(tab)) this.activeTab = tab;
    this.panelShownHandler = (event) => {
      if ((event as CustomEvent<{ panel?: string }>).detail?.panel === "admin") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
    this.ollamaDownloadHandler = (event) => {
      if (this.activeTab !== "ai") return;
      const summary = String((event as CustomEvent<Record<string, unknown>>).detail?.summary ?? "").trim();
      if (summary) this.setAdminStatus(summary, "busy");
      if (this.ollamaRefreshTimer != null) return;
      this.ollamaRefreshTimer = window.setTimeout(() => {
        this.ollamaRefreshTimer = null;
        void this.load();
      }, 300);
    };
    document.addEventListener("omni:ollama-download", this.ollamaDownloadHandler);
    this.applyTabState();
    void this.load();
  }

  disconnect(): void {
    if (this.panelShownHandler) document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    if (this.ollamaDownloadHandler) document.removeEventListener("omni:ollama-download", this.ollamaDownloadHandler);
    if (this.ollamaRefreshTimer != null) window.clearTimeout(this.ollamaRefreshTimer);
    this.ollamaRefreshTimer = null;
  }

  private recyclrController(): RecyclrController {
    const controller = this.application.getControllerForElementAndIdentifier(document.body, "recyclr") as RecyclrController | null;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  private chatController(): ChatController | null {
    return this.application.getControllerForElementAndIdentifier(document.body, "chat") as ChatController | null;
  }

  setAdminStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle"): void {
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.adminStatusTarget.textContent = message;
    this.adminStatusTarget.className = `text-xs ${classes[tone]}`;
  }

  receiveStatus(event: Event): void {
    const detail = (event as CustomEvent<{ message?: string; tone?: "idle" | "busy" | "error" | "ok" }>).detail;
    if (!detail?.message?.trim()) throw new Error("Admin child status event requires a message.");
    this.setAdminStatus(detail.message, detail.tone ?? "idle");
  }

  private applyTabState(): void {
    this.tabNavTarget.querySelectorAll("[data-admin-tab]").forEach((button) => {
      const active = button.getAttribute("data-admin-tab") === this.activeTab;
      button.classList.toggle("border-cyan-300/40", active);
      button.classList.toggle("bg-cyan-300/10", active);
      button.classList.toggle("text-cyan-100", active);
      button.classList.toggle("border-white/10", !active);
      button.classList.toggle("text-zinc-400", !active);
    });
  }

  showTab(event: Event): void {
    event.preventDefault();
    const tab = (event.currentTarget as HTMLElement).dataset.adminTab ?? "";
    if (!ADMIN_TABS.has(tab)) throw new Error(`Unsupported admin tab ${JSON.stringify(tab)}.`);
    this.activeTab = tab;
    this.applyTabState();
    this.recyclrController().pushRoute(panelHref("admin", window.location, { admin_tab: tab }));
    void this.load();
  }

  async load(): Promise<void> {
    this.setAdminStatus("Loading admin settings…", "busy");
    try {
      const payload = await fetchAdminComponent(this.activeTab, {
        modelOffset: this.modelOffset,
        catalogQuery: this.catalogQuery,
        catalogPage: this.catalogPage,
        downloadOffset: this.downloadOffset,
      });
      await renderServerBundle(this.recyclrController(), payload, "Admin component");
      if (this.activeTab === "health") await this.chatController()?.loadStatus();
      this.setAdminStatus("Ready", "idle");
    } catch (error) {
      reportError(this.setAdminStatus.bind(this), error);
    }
  }

  async loadHealth(event?: Event): Promise<void> {
    event?.preventDefault();
    this.activeTab = "health";
    this.applyTabState();
    await this.load();
  }

  loadModelPage(event: Event): void {
    this.modelOffset = this.pageOffset(event, "model");
    void this.load();
  }

  loadDownloadPage(event: Event): void {
    this.downloadOffset = this.pageOffset(event, "download");
    void this.load();
  }

  private pageOffset(event: Event, kind: string): number {
    event.preventDefault();
    const target = event.currentTarget as HTMLElement | null;
    const offset = Number(target?.dataset.pageOffset ?? -1);
    if (target?.dataset.pageKind !== kind || !Number.isSafeInteger(offset) || offset < 0) {
      throw new Error(`Ollama ${kind} page control is invalid.`);
    }
    return offset;
  }

  searchOllamaCatalog(event: Event): void {
    event.preventDefault();
    const query = this.hasCatalogQueryTarget ? this.catalogQueryTarget.value.trim() : "";
    if (!query) return reportErrorMessage(this.setAdminStatus.bind(this), "Enter a catalog search");
    this.catalogQuery = query;
    this.catalogPage = 1;
    this.catalogQueryTarget.value = query;
    void this.load();
  }

  loadCatalogPage(event: Event): void {
    event.preventDefault();
    const page = Number((event.currentTarget as HTMLElement | null)?.dataset.catalogPage ?? -1);
    if (!Number.isSafeInteger(page) || page < 1 || page > 100 || !this.catalogQuery) {
      throw new Error("Ollama catalog page control is invalid.");
    }
    this.catalogPage = page;
    void this.load();
  }

  private field(root: ParentNode, name: string): HTMLInputElement | HTMLSelectElement | null {
    return root.querySelector(`[data-admin-field='${name}']`);
  }

  private collect(prefix: string): Record<string, string> {
    const values: Record<string, string> = {};
    this.element.querySelectorAll(`[data-admin-field^='${prefix}']`).forEach((node) => {
      const input = node as HTMLInputElement | HTMLSelectElement;
      const key = input.dataset.adminField?.slice(prefix.length) ?? "";
      if (key) values[key] = input.value.trim();
    });
    return values;
  }

  async saveNetwork(event: Event): Promise<void> {
    event.preventDefault();
    const host = this.field(this.element, "networkHost")?.value.trim() ?? "";
    const port = Number.parseInt(this.field(this.element, "networkPort")?.value ?? "", 10);
    if (!host || !Number.isSafeInteger(port) || port < 1) return reportErrorMessage(this.setAdminStatus.bind(this), "Enter a valid host and port");
    await this.mutate("Saving network URL…", "Network URL saved", () => saveNetworkSettings({ host, port }));
  }

  async saveGlobalModels(event: Event): Promise<void> {
    event.preventDefault();
    await this.mutate("Saving model settings…", "Model settings saved", () => saveModelSettings(this.collect("model_")));
  }

  async saveAPISecrets(event: Event): Promise<void> {
    event.preventDefault();
    const values = Object.fromEntries(Object.entries(this.collect("secret_")).filter(([, value]) => value));
    await this.mutate("Saving API keys…", "API keys saved; restart core to activate provider changes", () => saveAPISecrets(values));
  }

  async clearSecret(event: Event): Promise<void> {
    event.preventDefault();
    const key = (event.currentTarget as HTMLElement).dataset.secretKey ?? "";
    if (!key || !window.confirm(`Clear stored value for ${key}?`)) return;
    await this.mutate("Clearing API key…", "Stored API key cleared", () => saveAPISecrets({}, [key]));
  }

  async pullModel(event: Event): Promise<void> {
    event.preventDefault();
    const model = this.hasPullModelTarget ? this.pullModelTarget.value.trim() : "";
    if (!model) return;
    this.pullModelTarget.value = "";
    await this.mutate(`Queueing ${model}…`, `Download queued for ${model}`, () => pullOllamaModel(model));
  }

  async downloadCatalogModel(event: Event): Promise<void> {
    event.preventDefault();
    const model = (event.currentTarget as HTMLElement | null)?.dataset.modelName?.trim() ?? "";
    if (!model) throw new Error("Catalog download control has no model authority.");
    await this.mutate(`Queueing ${model}…`, `Download queued for ${model}`, () => pullOllamaModel(model));
  }

  async deleteOllamaModel(event: Event): Promise<void> {
    event.preventDefault();
    const name = (event.currentTarget as HTMLElement).dataset.modelName ?? "";
    if (!name || !window.confirm(`Remove Ollama model ${name}?`)) return;
    await this.mutate(`Removing ${name}…`, `Removed ${name}`, () => deleteOllamaModel(name));
  }

  async uploadDocuments(event: Event): Promise<void> {
    event.preventDefault();
    const files = this.hasIngestFilesTarget ? this.ingestFilesTarget.files : null;
    if (!files?.length || !this.hasIngestStageTarget || !this.hasIngestTagsTarget) {
      return reportErrorMessage(this.setAdminStatus.bind(this), "Choose one or more files first");
    }
    await this.mutate("Uploading documents…", "Documents ingested", async () => {
      await ingestDocuments(files, { stage: this.ingestStageTarget.value, kind: "reference", tags: this.ingestTagsTarget.value.trim() });
      document.dispatchEvent(new CustomEvent("omni:memory-changed"));
    });
  }

  private async mutate(working: string, success: string, action: () => Promise<unknown>): Promise<void> {
    this.setAdminStatus(working, "busy");
    setGlobalLoading(true);
    try {
      await action();
      await this.load();
      reportOk(this.setAdminStatus.bind(this), success);
    } catch (error) {
      reportError(this.setAdminStatus.bind(this), error);
    } finally {
      setGlobalLoading(false);
    }
  }
}
