import { Controller } from "@hotwired/stimulus";
import {
  askDataSource as askDataSourceAPI,
  createDataSource,
  deleteDataSource as deleteDataSourceAPI,
  exploreDataSource as exploreDataSourceAPI,
  fetchDataSourceCatalog,
  fetchDataSourceSchema,
  fetchDataSources,
  fetchJobDetails,
  runDataSourceQuery as runDataSourceQueryAPI,
  testDataSource as testDataSourceAPI,
  updateDataSource,
  type JobRecord,
} from "../lib/admin_api";
import { parseDataSourceJobResult } from "../lib/data_source_job_result";
import {
  emptyDataSourcesViewState,
  renderDataSourcesPanel,
  renderSourceForm,
  type DataSourcesViewState,
} from "../lib/data_sources_render";
import { escapeHTML } from "../lib/dom";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { observeRealtimeJob, type RealtimeJobObservation } from "../lib/realtime_job_observer";

type StatusTone = "idle" | "busy" | "error" | "ok";

type DataSourceForm = {
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
};

export default class AdminDataSourcesController extends Controller<HTMLElement> {
  private state: DataSourcesViewState = emptyDataSourcesViewState();
  private readonly jobObservations = new Map<number, RealtimeJobObservation<{ job: JobRecord }>>();

  connect(): void {
    void this.loadDataSources();
  }

  disconnect(): void {
    for (const observation of this.jobObservations.values()) {
      observation.cancel("Admin data-source controller disconnected.");
    }
    this.jobObservations.clear();
  }

  private setStatus(message: string, tone: StatusTone): void {
    this.dispatch("status", { detail: { message, tone } });
  }

  private actionOk(message: string): void {
    reportOk(this.setStatus.bind(this), message);
  }

  private actionFail(error: unknown): void {
    reportError(this.setStatus.bind(this), error);
  }

  private actionFailMessage(message: string): void {
    reportErrorMessage(this.setStatus.bind(this), message);
  }

  private preserveQueryForms(): { sql: string; question: string } {
    const sql = (this.element.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null)?.value ?? "";
    const question = (this.element.querySelector("[data-ds-field='question']") as HTMLInputElement | null)?.value ?? "";
    return { sql, question };
  }

  private restoreQueryForms(values: { sql: string; question: string }): void {
    const sql = this.element.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null;
    const question = this.element.querySelector("[data-ds-field='question']") as HTMLInputElement | null;
    if (sql) sql.value = values.sql;
    if (question) question.value = values.question;
  }

  private render(preserveForms = false): void {
    const preserved = preserveForms ? this.preserveQueryForms() : { sql: "", question: "" };
    this.element.innerHTML = renderDataSourcesPanel(this.state);
    if (preserveForms) this.restoreQueryForms(preserved);
  }

  toggleDataSourceDSNPanel(): void {
    const useDSN = (this.element.querySelector("[data-ds-field='use_dsn']") as HTMLInputElement | null)?.checked;
    if (useDSN === undefined) throw new Error("Data-source DSN toggle is unavailable.");
    const form = this.element.querySelector("[data-ds-source-form]");
    if (!form) throw new Error("Data-source form is unavailable.");
    const current = this.readForm();
    current.use_dsn = useDSN;
    form.outerHTML = renderSourceForm(current, current.id || this.state.editingId);
  }

  async loadDataSources(): Promise<void> {
    try {
      const sources = await fetchDataSources();
      const selectedId = this.state.selectedId && sources.some((source) => source.id === this.state.selectedId)
        ? this.state.selectedId
        : sources[0]?.id ?? null;
      this.state = { ...this.state, sources, selectedId };
      this.render(true);
      this.setStatus("Data sources ready", "idle");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.element.innerHTML = `<p class="text-sm text-rose-300">${escapeHTML(message)}</p>`;
      this.actionFail(error);
    }
  }

  private value(field: string): string {
    const input = this.element.querySelector(`[data-ds-field='${field}']`) as HTMLInputElement | HTMLSelectElement | null;
    if (!input) throw new Error(`Data-source form field ${JSON.stringify(field)} is unavailable.`);
    return input.value.trim();
  }

  private optionalValue(field: string): string {
    return (this.element.querySelector(`[data-ds-field='${field}']`) as HTMLInputElement | HTMLSelectElement | null)?.value.trim() ?? "";
  }

  private readForm(): DataSourceForm {
    const context = this.element.querySelector("[data-ds-field='context_prompt']") as HTMLTextAreaElement | null;
    const useDSN = this.element.querySelector("[data-ds-field='use_dsn']") as HTMLInputElement | null;
    if (!context || !useDSN) throw new Error("Data-source form is incomplete.");
    const port = Number.parseInt(this.optionalValue("port") || "5432", 10);
    if (!Number.isSafeInteger(port) || port <= 0) throw new Error("Data-source port must be a positive integer.");
    return {
      id: this.value("id"),
      name: this.value("name"),
      driver: this.value("driver"),
      domain: this.value("domain"),
      context_prompt: context.value.trim(),
      privacy_mode: this.value("privacy_mode"),
      use_dsn: useDSN.checked,
      dsn: this.optionalValue("dsn"),
      host: this.optionalValue("host"),
      port,
      database_name: this.optionalValue("database_name"),
      username: this.optionalValue("username"),
      password: this.optionalValue("password"),
      ssl_mode: this.optionalValue("ssl_mode") || "prefer",
    };
  }

  async saveDataSource(event: Event): Promise<void> {
    event.preventDefault();
    try {
      const form = this.readForm();
      if (!form.name) throw new Error("Data-source name is required.");
      if (form.use_dsn && !form.dsn && !form.id) throw new Error("Data-source DSN is required.");
      if (!form.use_dsn && (!form.host || !form.database_name || !form.username)) {
        throw new Error("Data-source host, database, and username are required.");
      }
      this.setStatus(form.id ? "Saving data source…" : "Adding data source…", "busy");
      const payload = { ...form, read_only: true };
      const source = form.id ? await updateDataSource(form.id, payload) : await createDataSource(payload);
      this.state.editingId = null;
      this.state.selectedId = source.id;
      await this.loadDataSources();
      this.actionOk(form.id ? "Data source saved" : "Data source added");
    } catch (error) {
      this.actionFail(error);
    }
  }

  editDataSource(event: Event): void {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim();
    if (!id) throw new Error("Edit data-source action requires a source id.");
    this.state.editingId = id;
    this.state.selectedId = id;
    this.render(true);
  }

  cancelEditDataSource(event: Event): void {
    event.preventDefault();
    this.state.editingId = null;
    this.render(true);
  }

  selectDataSource(event: Event): void {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim();
    if (!id) throw new Error("Select data-source action requires a source id.");
    this.state.selectedId = id;
    this.state.schema = null;
    this.state.catalog = null;
    this.state.catalogReady = false;
    this.state.queryResult = null;
    this.render(true);
  }

  async deleteDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim();
    const source = this.state.sources.find((item) => item.id === id);
    if (!id) throw new Error("Delete data-source action requires a source id.");
    if (!window.confirm(`Remove data source ${source?.name || id}?`)) return;
    this.setStatus("Removing data source…", "busy");
    try {
      await deleteDataSourceAPI(id);
      if (this.state.selectedId === id) {
        this.state.selectedId = null;
        this.state.schema = null;
        this.state.queryResult = null;
      }
      if (this.state.editingId === id) this.state.editingId = null;
      await this.loadDataSources();
      this.actionOk("Data source removed");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async testDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim();
    if (!id) throw new Error("Test data-source action requires a source id.");
    this.setStatus("Testing connection…", "busy");
    try {
      const result = await testDataSourceAPI(id);
      this.state.selectedId = id;
      await this.loadDataSources();
      this.actionOk(result.message || `Connection ${result.status}`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async loadDataSourceSchema(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.state.selectedId;
    if (!id) throw new Error("Load schema action requires a selected data source.");
    this.setStatus("Loading schema…", "busy");
    try {
      const schema = await fetchDataSourceSchema(id);
      this.state.selectedId = id;
      this.state.schema = schema;
      this.render(true);
      this.actionOk(`Loaded ${schema.length} tables`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async loadDataSourceCatalog(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.state.selectedId;
    if (!id) throw new Error("Load schema map action requires a selected data source.");
    this.setStatus("Loading schema map…", "busy");
    try {
      const { catalog, ready } = await fetchDataSourceCatalog(id);
      this.state.selectedId = id;
      this.state.catalog = catalog;
      this.state.catalogReady = ready;
      this.render(true);
      this.actionOk(ready ? `Schema map ready (${catalog.tables.length} tables)` : "No schema map yet — run Explore first");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async exploreDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.state.selectedId;
    if (!id) throw new Error("Explore action requires a selected data source.");
    this.setStatus("Queueing schema exploration…", "busy");
    try {
      const queued = await exploreDataSourceAPI(id);
      const jobID = queued.job?.id;
      if (!jobID) throw new Error("Explore job was not created.");
      this.setStatus(`Exploring schema (job #${jobID})…`, "busy");
      await this.waitForJob(jobID);
      const { catalog, ready } = await fetchDataSourceCatalog(id);
      this.state.catalog = catalog;
      this.state.catalogReady = ready;
      await this.loadDataSources();
      this.render(true);
      this.actionOk(ready ? `Schema map built (${catalog.tables.length} tables)` : "Exploration finished");
    } catch (error) {
      this.actionFail(error);
    }
  }

  insertSchemaQuery(event: Event): void {
    event.preventDefault();
    const table = (event.currentTarget as HTMLElement).dataset.tableName?.trim();
    if (!table) throw new Error("Insert schema query action requires a table name.");
    const field = this.element.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null;
    if (!field) throw new Error("Data-source SQL editor is unavailable.");
    field.value = `SELECT * FROM ${table} LIMIT 20`;
    field.focus();
  }

  async runDataSourceQuery(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || this.state.selectedId;
    const sql = (this.element.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null)?.value.trim();
    if (!id || !sql) {
      this.actionFailMessage("Select a source and enter a SQL query first.");
      return;
    }
    this.setStatus("Running query…", "busy");
    try {
      const result = await runDataSourceQueryAPI(id, sql);
      this.applyQueryResult(result);
      this.actionOk(`${result.count} row${result.count === 1 ? "" : "s"} returned`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  async askDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.state.selectedId;
    const question = (this.element.querySelector("[data-ds-field='question']") as HTMLInputElement | null)?.value.trim();
    if (!id || !question) {
      this.actionFailMessage("Select a source and enter a question.");
      return;
    }
    this.setStatus("Queueing data query job…", "busy");
    try {
      const queued = await askDataSourceAPI(id, question);
      const jobID = queued.job?.id;
      if (!jobID) throw new Error("Data query job was not created.");
      const job = await this.waitForJob(jobID);
      const result = parseDataSourceJobResult(job.result || "");
      this.applyQueryResult(result);
      this.actionOk(result.answer || `Job #${jobID} completed`);
    } catch (error) {
      this.actionFail(error);
    }
  }

  updateDataSourceChart(): void {
    const label = (this.element.querySelector("[data-ds-field='chart_label']") as HTMLSelectElement | null)?.value;
    const value = (this.element.querySelector("[data-ds-field='chart_value']") as HTMLSelectElement | null)?.value;
    if (label === undefined || value === undefined) throw new Error("Data-source chart controls are unavailable.");
    this.state.chartLabelCol = label;
    this.state.chartValueCol = value;
    this.render(true);
  }

  private applyQueryResult(result: DataSourcesViewState["queryResult"]): void {
    if (!result) throw new Error("Data-source query result is required.");
    this.state.queryResult = result;
    this.state.chartLabelCol = result.columns[0] || "";
    this.state.chartValueCol = result.columns.find((column) =>
      result.rows.some((row) => typeof row[column] === "number" ||
        (typeof row[column] === "string" && row[column] !== "" && Number.isFinite(Number(row[column])))),
    ) || "";
    this.render(true);
  }

  private async waitForJob(jobID: number): Promise<JobRecord> {
    if (this.jobObservations.has(jobID)) throw new Error(`Data source job #${jobID} is already being observed.`);
    const observation = observeRealtimeJob({
      jobID,
      load: async () => {
        const details = await fetchJobDetails(jobID);
        if (!details.job || details.job.id !== jobID) {
          throw new Error(`Authoritative job response did not include job #${jobID}.`);
        }
        return { status: details.job.status, error: details.job.error, data: { job: details.job } };
      },
      onUpdate: ({ status }) => {
        const label = status === "completed" ? "finalizing results" : status;
        this.setStatus(`Running job #${jobID} · ${label}…`, "busy");
      },
    });
    this.jobObservations.set(jobID, observation);
    try {
      return (await observation.completion).data.job;
    } finally {
      this.jobObservations.delete(jobID);
    }
  }
}
