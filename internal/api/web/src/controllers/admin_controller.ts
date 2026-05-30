import { Controller } from "@hotwired/stimulus";
import {
  askDataSource as askDataSourceAPI,
  createDataSource,
  deleteDataSource as deleteDataSourceAPI,
  deleteOllamaModel,
  exploreDataSource as exploreDataSourceAPI,
  fetchDataSourceCatalog,
  fetchDataSourceSchema,
  fetchDataSources,
  fetchJobDetails,
  fetchMindStats,
  fetchModelSettings,
  fetchOllamaModels,
  fetchAPISecrets,
  fetchNetworkSettings,
  ingestDocuments,
  pullOllamaModel,
  runDataSourceQuery as runDataSourceQueryAPI,
  saveModelSettings,
  saveAPISecrets,
  saveNetworkSettings,
  testDataSource as testDataSourceAPI,
  updateDataSource,
} from "../lib/admin_api";
import { fetchGlobalAgentSettings, saveGlobalAgentSettings } from "../lib/agent_config_api";
import { renderGlobalAgentSettings } from "../lib/agent_config_render";
import {
  emptyDataSourcesViewState,
  renderDataSourcesPanel,
  type DataSourcesViewState,
} from "../lib/data_sources_render";
import {
  renderAPISecretsSettings,
  renderGlobalModelSettings,
  renderMindStats,
  renderNetworkSettings,
  renderOllamaModels,
  type AdminTab,
} from "../lib/admin_render";
import type GxController from "./gx_controller";
import type ChatController from "./chat_controller";
import { panelHref, parseAdminTabFromLocation } from "../lib/panel_routing";
import { sleep } from "../lib/dom";

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
    "dataSourcesPanel",
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
  declare readonly dataSourcesPanelTarget: HTMLElement;

  private panelShownHandler: ((event: Event) => void) | null = null;
  private activeTab: AdminTab = "overview";
  private dataSourcesState: DataSourcesViewState = emptyDataSourcesViewState();

  connect() {
    const fromURL = parseAdminTabFromLocation();
    if (fromURL === "overview" || fromURL === "ai" || fromURL === "datasources" || fromURL === "health" || fromURL === "advanced") {
      this.activeTab = fromURL;
    }
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "admin") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
    this.applyTabState();
    if (this.activeTab === "health") void this.loadHealth();
    if (this.activeTab === "datasources") void this.loadDataSources();
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
    if (this.activeTab === "datasources") void this.loadDataSources();
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
        this.loadDataSources(),
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

  private preserveDataSourceFormValues(): { sql: string; question: string } {
    const sql = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null)?.value ?? "";
    const question = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='question']") as HTMLInputElement | null)?.value ?? "";
    return { sql, question };
  }

  private restoreDataSourceFormValues(values: { sql: string; question: string }) {
    const sqlField = this.dataSourcesPanelTarget.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null;
    const questionField = this.dataSourcesPanelTarget.querySelector("[data-ds-field='question']") as HTMLInputElement | null;
    if (sqlField) sqlField.value = values.sql;
    if (questionField) questionField.value = values.question;
  }

  private renderDataSources(preserveForms = false) {
    const preserved = preserveForms ? this.preserveDataSourceFormValues() : { sql: "", question: "" };
    this.dataSourcesPanelTarget.innerHTML = renderDataSourcesPanel(this.dataSourcesState);
    if (preserveForms) this.restoreDataSourceFormValues(preserved);
    this.dataSourcesPanelTarget.querySelectorAll("[data-ds-field='use_dsn']").forEach((input) => {
      input.addEventListener("change", () => this.toggleDataSourceDSNPanel());
    });
  }

  toggleDataSourceDSNPanel() {
    const useDSN = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='use_dsn']") as HTMLInputElement | null)?.checked ?? false;
    this.dataSourcesPanelTarget.querySelector("[data-ds-panel='dsn']")?.classList.toggle("hidden", !useDSN);
    this.dataSourcesPanelTarget.querySelector("[data-ds-panel='fields']")?.classList.toggle("hidden", useDSN);
  }

  async loadDataSources() {
    try {
      const sources = await fetchDataSources();
      const selectedId = this.dataSourcesState.selectedId && sources.some((s) => s.id === this.dataSourcesState.selectedId)
        ? this.dataSourcesState.selectedId
        : sources[0]?.id ?? null;
      this.dataSourcesState = {
        ...this.dataSourcesState,
        sources,
        selectedId,
      };
      this.renderDataSources(true);
    } catch (error) {
      this.dataSourcesPanelTarget.innerHTML = `<p class="text-sm text-rose-300">${error instanceof Error ? error.message : String(error)}</p>`;
    }
  }

  private readDataSourceForm(): {
    id: string;
    name: string;
    driver: string;
    domain: string;
    context_prompt: string;
    privacy_mode: string;
    use_dsn: boolean;
    dsn: string;
    host: string;
    port: number;
    database_name: string;
    username: string;
    password: string;
    ssl_mode: string;
  } {
    const root = this.dataSourcesPanelTarget;
    const read = (field: string) => (root.querySelector(`[data-ds-field='${field}']`) as HTMLInputElement | HTMLSelectElement | null)?.value.trim() ?? "";
    const useDSN = (root.querySelector("[data-ds-field='use_dsn']") as HTMLInputElement | null)?.checked ?? false;
    const port = Number.parseInt(read("port") || "5432", 10);
    return {
      id: read("id"),
      name: read("name"),
      driver: read("driver") || "postgres",
      domain: read("domain") || "generic",
      context_prompt: (root.querySelector("[data-ds-field='context_prompt']") as HTMLTextAreaElement | null)?.value.trim() ?? "",
      privacy_mode: read("privacy_mode") || "strict",
      use_dsn: useDSN,
      dsn: read("dsn"),
      host: read("host"),
      port: Number.isFinite(port) && port > 0 ? port : 5432,
      database_name: read("database_name"),
      username: read("username"),
      password: read("password"),
      ssl_mode: read("ssl_mode") || "prefer",
    };
  }

  async saveDataSource(event: Event) {
    event.preventDefault();
    const form = this.readDataSourceForm();
    if (!form.name) {
      this.actionFailMessage("Name is required");
      return;
    }
    if (form.use_dsn && !form.dsn && !form.id) {
      this.actionFailMessage("DSN is required");
      return;
    }
    if (!form.use_dsn && (!form.host || !form.database_name || !form.username)) {
      this.actionFailMessage("Host, database, and username are required");
      return;
    }
    this.setAdminStatus(form.id ? "Saving data source…" : "Adding data source…", "busy");
    try {
      const payload = {
        name: form.name,
        driver: form.driver,
        domain: form.domain,
        context_prompt: form.context_prompt,
        privacy_mode: form.privacy_mode,
        use_dsn: form.use_dsn,
        dsn: form.dsn,
        host: form.host,
        port: form.port,
        database_name: form.database_name,
        username: form.username,
        password: form.password,
        ssl_mode: form.ssl_mode,
        read_only: true,
      };
      const source = form.id ? await updateDataSource(form.id, payload) : await createDataSource(payload);
      this.dataSourcesState.editingId = null;
      this.dataSourcesState.selectedId = source.id;
      await this.loadDataSources();
      this.actionOk(form.id ? "Data source saved" : "Data source added");
    } catch (error) {
      this.actionFail(error);
    }
  }

  editDataSource(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || "";
    if (!id) return;
    this.dataSourcesState.editingId = id;
    this.dataSourcesState.selectedId = id;
    this.renderDataSources(true);
  }

  cancelEditDataSource(event: Event) {
    event.preventDefault();
    this.dataSourcesState.editingId = null;
    this.renderDataSources(true);
  }

  selectDataSource(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || "";
    if (!id) return;
    this.dataSourcesState.selectedId = id;
    this.dataSourcesState.schema = null;
    this.dataSourcesState.catalog = null;
    this.dataSourcesState.catalogReady = false;
    this.dataSourcesState.queryResult = null;
    this.renderDataSources(true);
  }

  async deleteDataSourceHandler(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || "";
    const source = this.dataSourcesState.sources.find((s) => s.id === id);
    if (!id || !window.confirm(`Remove data source ${source?.name || id}?`)) return;
    this.setAdminStatus("Removing data source…", "busy");
    try {
      await deleteDataSourceAPI(id);
      if (this.dataSourcesState.selectedId === id) {
        this.dataSourcesState.selectedId = null;
        this.dataSourcesState.schema = null;
        this.dataSourcesState.queryResult = null;
      }
      if (this.dataSourcesState.editingId === id) this.dataSourcesState.editingId = null;
      await this.loadDataSources();
      this.actionOk("Data source removed");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async deleteDataSource(event: Event) {
    await this.deleteDataSourceHandler(event);
  }

  async testDataSourceHandler(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || "";
    if (!id) return;
    this.setAdminStatus("Testing connection…", "busy");
    try {
      const result = await testDataSourceAPI(id);
      await this.loadDataSources();
      this.dataSourcesState.selectedId = id;
      this.actionOk(result.message || `Connection ${result.status}`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async testDataSource(event: Event) {
    await this.testDataSourceHandler(event);
  }

  async loadDataSourceSchema(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.dataSourcesState.selectedId || "";
    if (!id) return;
    this.setAdminStatus("Loading schema…", "busy");
    try {
      const schema = await fetchDataSourceSchema(id);
      this.dataSourcesState.selectedId = id;
      this.dataSourcesState.schema = schema;
      this.renderDataSources(true);
      this.actionOk(`Loaded ${schema.length} tables`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async loadDataSourceCatalog(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.dataSourcesState.selectedId || "";
    if (!id) return;
    this.setAdminStatus("Loading schema map…", "busy");
    try {
      const { catalog, ready } = await fetchDataSourceCatalog(id);
      this.dataSourcesState.selectedId = id;
      this.dataSourcesState.catalog = catalog;
      this.dataSourcesState.catalogReady = ready;
      this.renderDataSources(true);
      this.actionOk(ready ? `Schema map ready (${catalog.tables?.length ?? 0} tables)` : "No schema map yet — run Explore first");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async exploreDataSource(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.dataSourcesState.selectedId || "";
    if (!id) return;
    this.setAdminStatus("Queueing schema exploration…", "busy");
    try {
      const queued = await exploreDataSourceAPI(id);
      const jobID = queued.job?.id;
      if (!jobID) {
        throw new Error("Explore job was not created");
      }
      this.setAdminStatus(`Exploring schema (job #${jobID})…`, "busy");
      await this.pollDataSourceJob(jobID);
      const { catalog, ready } = await fetchDataSourceCatalog(id);
      this.dataSourcesState.catalog = catalog;
      this.dataSourcesState.catalogReady = ready;
      await this.loadDataSources();
      this.renderDataSources(true);
      this.actionOk(ready ? `Schema map built (${catalog.tables?.length ?? 0} tables)` : "Exploration finished");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async exploreDataSourceHandler(event: Event) {
    await this.exploreDataSource(event);
  }

  async loadDataSourceCatalogHandler(event: Event) {
    await this.loadDataSourceCatalog(event);
  }

  insertSchemaQuery(event: Event) {
    event.preventDefault();
    const table = (event.currentTarget as HTMLElement).dataset.tableName || "";
    if (!table) return;
    const field = this.dataSourcesPanelTarget.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null;
    if (!field) return;
    field.value = `SELECT * FROM ${table} LIMIT 20`;
    field.focus();
  }

  async runDataSourceQuery(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.dataSourcesState.selectedId || "";
    const sql = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null)?.value.trim() ?? "";
    if (!id || !sql) {
      this.actionFailMessage("Enter a SQL query first");
      return;
    }
    this.setAdminStatus("Running query…", "busy");
    try {
      const result = await runDataSourceQueryAPI(id, sql);
      this.dataSourcesState.queryResult = result;
      this.dataSourcesState.chartLabelCol = result.columns[0] || "";
      this.dataSourcesState.chartValueCol = result.columns.find((col) => result.rows.some((row) => typeof row[col] === "number" || (typeof row[col] === "string" && row[col] !== "" && Number.isFinite(Number(row[col]))))) || "";
      this.renderDataSources(true);
      this.actionOk(`${result.count} row${result.count === 1 ? "" : "s"} returned`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async askDataSource(event: Event) {
    event.preventDefault();
    const id = this.dataSourcesState.selectedId || "";
    const question = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='question']") as HTMLInputElement | null)?.value.trim() ?? "";
    if (!id || !question) {
      this.actionFailMessage("Select a source and enter a question");
      return;
    }
    this.setAdminStatus("Queueing data query job…", "busy");
    try {
      const queued = await askDataSourceAPI(id, question);
      const jobID = queued.job?.id;
      if (!jobID) {
        throw new Error("Job was not created");
      }
      this.setAdminStatus(`Running job #${jobID}…`, "busy");
      const result = await this.pollDataSourceJob(jobID);
      this.dataSourcesState.queryResult = result;
      this.dataSourcesState.chartLabelCol = result.columns[0] || "";
      this.dataSourcesState.chartValueCol =
        result.columns.find((col) => result.rows.some((row) => typeof row[col] === "number" || (typeof row[col] === "string" && row[col] !== "" && Number.isFinite(Number(row[col]))))) || "";
      this.renderDataSources(true);
      this.actionOk(result.answer || `Job #${jobID} completed`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  private async pollDataSourceJob(jobID: number): Promise<import("../lib/admin_api").DataSourceQueryResult> {
    for (;;) {
      await sleep(800);
      const details = await fetchJobDetails(jobID);
      const status = details.job?.status || "";
      if (status === "completed") {
        const parsed = this.parseDataSourceJobResult(details.job?.result || "");
        if (parsed) return parsed;
        throw new Error("Job completed without query results");
      }
      if (status === "failed" || status === "canceled") {
        throw new Error(details.job?.error || `Job ${status}`);
      }
      this.setAdminStatus(`Running job #${jobID} · ${status || "pending"}…`, "busy");
    }
  }

  private parseDataSourceJobResult(raw: string): import("../lib/admin_api").DataSourceQueryResult | null {
    const trimmed = raw.trim();
    if (!trimmed) return null;
    try {
      const parsed = JSON.parse(trimmed) as Record<string, unknown>;
      const nested = parsed.query as import("../lib/admin_api").DataSourceQueryResult | undefined;
      if (nested && Array.isArray(nested.columns)) {
        return {
          question: nested.question,
          sql: nested.sql,
          answer: nested.answer,
          columns: nested.columns || [],
          rows: nested.rows || [],
          count: nested.count ?? (nested.rows?.length || 0),
        };
      }
      const legacy = parsed as import("../lib/admin_api").DataSourceQueryResult;
      if (Array.isArray(legacy.columns)) {
        return {
          question: legacy.question,
          sql: legacy.sql,
          answer: legacy.answer,
          columns: legacy.columns || [],
          rows: legacy.rows || [],
          count: legacy.count ?? (legacy.rows?.length || 0),
        };
      }
      return null;
    } catch {
      return { answer: trimmed, columns: [], rows: [], count: 0 };
    }
  }

  updateDataSourceChart() {
    const label = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='chart_label']") as HTMLSelectElement | null)?.value ?? "";
    const value = (this.dataSourcesPanelTarget.querySelector("[data-ds-field='chart_value']") as HTMLSelectElement | null)?.value ?? "";
    this.dataSourcesState.chartLabelCol = label;
    this.dataSourcesState.chartValueCol = value;
    this.renderDataSources(true);
  }
}
