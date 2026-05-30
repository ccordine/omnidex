import { Controller } from "@hotwired/stimulus";
import {
  deleteOllamaModel,
  fetchMindStats,
  fetchModelSettings,
  fetchOllamaModels,
  fetchAPISecrets,
  fetchNetworkSettings,
  ingestDocuments,
  pullOllamaModel,
  saveModelSettings,
  saveAPISecrets,
  saveNetworkSettings,
} from "../lib/admin_api";
import { fetchGlobalAgentSettings, saveGlobalAgentSettings } from "../lib/agent_config_api";
import { renderGlobalAgentSettings } from "../lib/agent_config_render";
import {
  renderAPISecretsSettings,
  renderGlobalModelSettings,
  renderMindStats,
  renderNetworkSettings,
  renderOllamaModels,
  type AdminTab,
} from "../lib/admin_render";
import type ChatController from "./chat_controller";
import type GxController from "./gx_controller";
import { panelHref, parseAdminTabFromLocation } from "../lib/panel_routing";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";

export default class AdminController extends Controller {
  static targets = [
    "tabNav",
    "adminStatus",
    "mindStats",
    "networkAccess",
    "ollamaModels",
    "pullModel",
    "globalModels",
    "globalAgents",
    "apiSecrets",
    "ingestFiles",
    "ingestStage",
    "ingestTags",
  ];

  declare readonly tabNavTarget: HTMLElement;
  declare readonly adminStatusTarget: HTMLElement;
  declare readonly mindStatsTarget: HTMLElement;
  declare readonly networkAccessTarget: HTMLElement;
  declare readonly ollamaModelsTarget: HTMLElement;
  declare readonly pullModelTarget: HTMLInputElement;
  declare readonly globalModelsTarget: HTMLElement;
  declare readonly globalAgentsTarget: HTMLElement;
  declare readonly apiSecretsTarget: HTMLElement;
  declare readonly ingestFilesTarget: HTMLInputElement;
  declare readonly ingestStageTarget: HTMLSelectElement;
  declare readonly ingestTagsTarget: HTMLInputElement;

  private panelShownHandler: ((event: Event) => void) | null = null;
  private activeTab: AdminTab = "overview";

  connect() {
    const fromURL = parseAdminTabFromLocation();
    if (fromURL === "overview" || fromURL === "ai" || fromURL === "health" || fromURL === "advanced") {
      this.activeTab = fromURL;
    }
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "admin") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
    this.applyTabState();
    if (this.activeTab === "health") void this.loadHealth();
  }

  disconnect() {
    if (this.panelShownHandler) {
      document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    }
  }

  setAdminStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.adminStatusTarget.textContent = message;
    this.adminStatusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  private actionOk(message: string) {
    reportOk(this.setAdminStatus.bind(this), message);
  }

  private actionFail(error: unknown) {
    reportError(this.setAdminStatus.bind(this), error);
  }

  private actionFailMessage(message: string) {
    reportErrorMessage(this.setAdminStatus.bind(this), message);
  }

  showTab(event: Event) {
    event.preventDefault();
    this.activeTab = ((event.currentTarget as HTMLElement).dataset.adminTab as AdminTab) || "overview";
    this.applyTabState();
    this.pushAdminTabHistory();
    if (this.activeTab === "health") void this.loadHealth();
  }

  private gxController(): GxController | null {
    return this.application.getControllerForElementAndIdentifier(this.element, "gx") as GxController | null;
  }

  private pushAdminTabHistory() {
    this.gxController()?.pushRoute(panelHref("admin", window.location, { admin_tab: this.activeTab }));
  }

  private applyTabState() {
    this.element.querySelectorAll("[data-admin-tab-panel]").forEach((panel) => {
      panel.classList.toggle("hidden", panel.getAttribute("data-admin-tab-panel") !== this.activeTab);
    });
    this.tabNavTarget.querySelectorAll("[data-admin-tab]").forEach((button) => {
      const active = button.getAttribute("data-admin-tab") === this.activeTab;
      button.classList.toggle("border-cyan-300/40", active);
      button.classList.toggle("bg-cyan-300/10", active);
      button.classList.toggle("text-cyan-100", active);
      button.classList.toggle("border-white/10", !active);
      button.classList.toggle("text-zinc-400", !active);
    });
  }

  private chatController(): ChatController | null {
    return this.application.getControllerForElementAndIdentifier(this.element, "chat") as ChatController | null;
  }

  async loadHealth() {
    this.setAdminStatus("Refreshing health checks…", "busy");
    try {
      await this.chatController()?.loadStatus();
      this.actionOk("Health checks updated");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async load() {
    this.setAdminStatus("Loading admin settings…", "busy");
    try {
      await Promise.all([
        this.loadNetwork(),
        this.loadMind(),
        this.loadOllama(),
        this.loadAPISecrets(),
        this.loadGlobalModels(),
        this.loadGlobalAgents(),
      ]);
      if (this.activeTab === "health") {
        await this.chatController()?.loadStatus();
      }
      this.setAdminStatus("Ready", "idle");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async loadNetwork() {
    try {
      const payload = await fetchNetworkSettings();
      this.networkAccessTarget.innerHTML = renderNetworkSettings(payload);
      document.dispatchEvent(new CustomEvent("omni:network-settings", { detail: payload }));
    } catch (error) {
      this.networkAccessTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  async saveNetwork(event: Event) {
    event.preventDefault();
    const host = (this.networkAccessTarget.querySelector("[data-admin-field='networkHost']") as HTMLInputElement | null)?.value.trim() ?? "";
    const portRaw = (this.networkAccessTarget.querySelector("[data-admin-field='networkPort']") as HTMLInputElement | null)?.value.trim() ?? "";
    const port = Number.parseInt(portRaw, 10);
    if (!host || !Number.isFinite(port) || port <= 0) {
      this.actionFailMessage("Enter a valid host and port");
      return;
    }
    this.setAdminStatus("Saving network URL…", "busy");
    try {
      await saveNetworkSettings({ host, port });
      await this.loadNetwork();
      this.actionOk("Network URL saved");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async loadMind() {
    try {
      const stats = await fetchMindStats();
      this.mindStatsTarget.innerHTML = renderMindStats(stats);
    } catch (error) {
      this.mindStatsTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  async loadOllama() {
    try {
      const payload = await fetchOllamaModels();
      this.ollamaModelsTarget.innerHTML = renderOllamaModels(payload.endpoint, payload.models);
    } catch (error) {
      this.ollamaModelsTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  async loadGlobalModels() {
    try {
      const payload = await fetchModelSettings();
      this.globalModelsTarget.innerHTML = renderGlobalModelSettings(payload.fields, payload.env_file);
    } catch (error) {
      this.globalModelsTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  async loadAPISecrets() {
    try {
      const payload = await fetchAPISecrets();
      this.apiSecretsTarget.innerHTML = renderAPISecretsSettings(payload.fields);
    } catch (error) {
      this.apiSecretsTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  async saveAPISecrets(event: Event) {
    event.preventDefault();
    const values: Record<string, string> = {};
    for (const input of this.apiSecretsTarget.querySelectorAll("[data-admin-field^='secret_']")) {
      const element = input as HTMLInputElement;
      const key = element.dataset.adminField?.replace(/^secret_/, "") ?? "";
      const value = element.value.trim();
      if (key && value) values[key] = value;
    }
    this.setAdminStatus("Saving API keys…", "busy");
    try {
      await saveAPISecrets(values);
      await this.loadAPISecrets();
      this.actionOk("API keys saved");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async clearSecret(event: Event) {
    event.preventDefault();
    const key = (event.currentTarget as HTMLElement).dataset.secretKey || "";
    if (!key || !window.confirm(`Clear stored value for ${key}?`)) return;
    this.setAdminStatus("Clearing stored API key…", "busy");
    try {
      await saveAPISecrets({}, [key]);
      await this.loadAPISecrets();
      this.actionOk("Stored API key cleared");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async loadGlobalAgents() {
    try {
      const payload = await fetchGlobalAgentSettings();
      this.globalAgentsTarget.innerHTML = renderGlobalAgentSettings(payload.fields);
    } catch (error) {
      this.globalAgentsTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  async saveGlobalAgents(event: Event) {
    event.preventDefault();
    const values: Record<string, string> = {};
    for (const input of this.globalAgentsTarget.querySelectorAll("[data-admin-field^='agent_']")) {
      if (input instanceof HTMLInputElement && input.type === "radio") {
        const key = input.dataset.adminField?.replace(/^agent_/, "") ?? "";
        if (key && input.checked) values[key] = input.value.trim();
        continue;
      }
      if (input instanceof HTMLInputElement && input.type === "checkbox") {
        const key = input.dataset.adminField?.replace(/^agent_/, "") ?? "";
        if (key && input.checked) values[key] = "true";
        continue;
      }
      const element = input as HTMLSelectElement;
      const key = element.dataset.adminField?.replace(/^agent_/, "") ?? "";
      if (key) values[key] = element.value.trim();
    }
    this.setAdminStatus("Saving workspace agent settings…", "busy");
    try {
      await saveGlobalAgentSettings(values);
      await this.loadGlobalAgents();
      this.actionOk("Workspace agent settings saved");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async pullModel(event: Event) {
    event.preventDefault();
    const model = this.pullModelTarget.value.trim();
    if (!model) return;
    this.setAdminStatus(`Pulling ${model}…`, "busy");
    try {
      await pullOllamaModel(model);
      this.pullModelTarget.value = "";
      await this.loadOllama();
      this.actionOk(`Pulled ${model}`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async deleteOllamaModel(event: Event) {
    event.preventDefault();
    const name = (event.currentTarget as HTMLElement).dataset.modelName || "";
    if (!name || !window.confirm(`Remove Ollama model ${name}?`)) return;
    this.setAdminStatus(`Removing ${name}…`, "busy");
    try {
      await deleteOllamaModel(name);
      await this.loadOllama();
      this.actionOk(`Removed ${name}`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async saveGlobalModels(event: Event) {
    event.preventDefault();
    const values: Record<string, string> = {};
    for (const input of this.globalModelsTarget.querySelectorAll("[data-admin-field^='model_']")) {
      const element = input as HTMLInputElement;
      const key = element.dataset.adminField?.replace(/^model_/, "") ?? "";
      const value = element.value.trim();
      if (key) values[key] = value;
    }
    this.setAdminStatus("Saving global model settings…", "busy");
    try {
      await saveModelSettings(values);
      await this.loadGlobalModels();
      this.actionOk("Global model settings saved");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async uploadDocuments(event: Event) {
    event.preventDefault();
    const files = this.ingestFilesTarget.files;
    if (!files?.length) {
      this.actionFailMessage("Choose one or more files first");
      return;
    }
    this.setAdminStatus("Uploading and parsing documents…", "busy");
    try {
      const payload = await ingestDocuments(files, {
        stage: this.ingestStageTarget.value || "candidate",
        kind: "reference",
        tags: this.ingestTagsTarget.value.trim(),
      });
      this.ingestFilesTarget.value = "";
      this.actionOk(payload.message);
      await this.loadMind();
      document.dispatchEvent(new CustomEvent("omni:memory-changed"));
    } catch (error) {
      this.actionFail(error);
    }
  }
}
