import { Controller } from "@hotwired/stimulus";
import {
  ingestDocuments,
} from "../lib/admin_api";
import { fetchAdminComponent } from "../lib/operational_component_api";
import { renderServerBundle } from "../lib/server_component_api";
import type RecyclrController from "./recyclr_controller";
import type ChatController from "./chat_controller";
import { panelHref, parseAdminTabFromLocation } from "../lib/panel_routing";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { setGlobalLoading } from "../lib/loading";

const ADMIN_TABS = new Set(["overview", "datasources", "health"]);

export default class AdminController extends Controller {
  static targets = ["tabNav", "tabPanel", "adminStatus", "ingestFiles", "ingestStage", "ingestTags"];

  declare readonly tabNavTarget: HTMLElement;
  declare readonly adminStatusTarget: HTMLElement;
  declare readonly ingestFilesTarget: HTMLInputElement;
  declare readonly hasIngestFilesTarget: boolean;
  declare readonly ingestStageTarget: HTMLSelectElement;
  declare readonly hasIngestStageTarget: boolean;
  declare readonly ingestTagsTarget: HTMLInputElement;
  declare readonly hasIngestTagsTarget: boolean;

  private activeTab = "overview";
  private panelShownHandler: ((event: Event) => void) | null = null;

  connect(): void {
    const tab = parseAdminTabFromLocation();
    if (tab && ADMIN_TABS.has(tab)) this.activeTab = tab;
    this.panelShownHandler = (event) => {
      if ((event as CustomEvent<{ panel?: string }>).detail?.panel === "admin") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
    this.applyTabState();
    void this.load();
  }

  disconnect(): void {
    if (this.panelShownHandler) document.removeEventListener("omni:panel-shown", this.panelShownHandler);
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
      const payload = await fetchAdminComponent(this.activeTab);
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
