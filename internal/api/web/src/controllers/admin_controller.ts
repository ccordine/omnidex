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
import { renderAPISecretsSettings, renderGlobalModelSettings, renderMindStats, renderNetworkSettings, renderOllamaModels } from "../lib/admin_render";

export default class AdminController extends Controller {
  static targets = [
    "mindStats",
    "networkAccess",
    "ollamaModels",
    "pullModel",
    "pullStatus",
    "globalModels",
    "globalAgents",
    "apiSecrets",
    "ingestStatus",
    "ingestFiles",
    "ingestStage",
    "ingestTags",
  ];

  declare readonly mindStatsTarget: HTMLElement;
  declare readonly networkAccessTarget: HTMLElement;
  declare readonly ollamaModelsTarget: HTMLElement;
  declare readonly pullModelTarget: HTMLInputElement;
  declare readonly pullStatusTarget: HTMLElement;
  declare readonly globalModelsTarget: HTMLElement;
  declare readonly globalAgentsTarget: HTMLElement;
  declare readonly apiSecretsTarget: HTMLElement;
  declare readonly ingestStatusTarget: HTMLElement;
  declare readonly ingestFilesTarget: HTMLInputElement;
  declare readonly ingestStageTarget: HTMLSelectElement;
  declare readonly ingestTagsTarget: HTMLInputElement;

  private panelShownHandler: ((event: Event) => void) | null = null;

  connect() {
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "admin") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
  }

  disconnect() {
    if (this.panelShownHandler) {
      document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    }
  }

  setPullStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.pullStatusTarget.textContent = message;
    this.pullStatusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  setIngestStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes = { idle: "text-zinc-400", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.ingestStatusTarget.textContent = message;
    this.ingestStatusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  async load() {
    await Promise.all([this.loadNetwork(), this.loadMind(), this.loadOllama(), this.loadAPISecrets(), this.loadGlobalModels(), this.loadGlobalAgents()]);
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
      this.setPullStatus("Enter a valid host and port", "error");
      return;
    }
    this.setPullStatus("Saving network URL…", "busy");
    try {
      await saveNetworkSettings({ host, port });
      await this.loadNetwork();
      this.setPullStatus("Network URL saved", "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
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
    this.setPullStatus("Saving API keys…", "busy");
    try {
      await saveAPISecrets(values);
      await this.loadAPISecrets();
      this.setPullStatus("API keys saved", "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async clearSecret(event: Event) {
    event.preventDefault();
    const key = (event.currentTarget as HTMLElement).dataset.secretKey || "";
    if (!key || !window.confirm(`Clear stored value for ${key}?`)) return;
    this.setPullStatus("Clearing stored API key…", "busy");
    try {
      await saveAPISecrets({}, [key]);
      await this.loadAPISecrets();
      this.setPullStatus("Stored API key cleared", "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async loadGlobalAgents() {
    try {
      const payload = await fetchGlobalAgentSettings();
      this.globalAgentsTarget.innerHTML = renderGlobalAgentSettings(payload.fields, payload.env_file);
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
    this.setPullStatus("Saving global agent settings…", "busy");
    try {
      await saveGlobalAgentSettings(values);
      await this.loadGlobalAgents();
      this.setPullStatus("Global agent settings saved", "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async pullModel(event: Event) {
    event.preventDefault();
    const model = this.pullModelTarget.value.trim();
    if (!model) return;
    this.setPullStatus(`Pulling ${model}…`, "busy");
    try {
      await pullOllamaModel(model);
      this.pullModelTarget.value = "";
      await this.loadOllama();
      this.setPullStatus(`Pulled ${model}`, "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async deleteOllamaModel(event: Event) {
    event.preventDefault();
    const name = (event.currentTarget as HTMLElement).dataset.modelName || "";
    if (!name || !window.confirm(`Remove Ollama model ${name}?`)) return;
    this.setPullStatus(`Removing ${name}…`, "busy");
    try {
      await deleteOllamaModel(name);
      await this.loadOllama();
      this.setPullStatus(`Removed ${name}`, "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
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
    this.setPullStatus("Saving global model settings…", "busy");
    try {
      await saveModelSettings(values);
      await this.loadGlobalModels();
      this.setPullStatus("Global model settings saved", "ok");
    } catch (error) {
      this.setPullStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async uploadDocuments(event: Event) {
    event.preventDefault();
    const files = this.ingestFilesTarget.files;
    if (!files?.length) {
      this.setIngestStatus("Choose one or more files first", "error");
      return;
    }
    this.setIngestStatus("Uploading and parsing documents…", "busy");
    try {
      const payload = await ingestDocuments(files, {
        stage: this.ingestStageTarget.value || "candidate",
        kind: "reference",
        tags: this.ingestTagsTarget.value.trim(),
      });
      this.ingestFilesTarget.value = "";
      this.setIngestStatus(payload.message, "ok");
      await this.loadMind();
      document.dispatchEvent(new CustomEvent("omni:memory-changed"));
    } catch (error) {
      this.setIngestStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }
}
