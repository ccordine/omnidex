import { Controller } from "@hotwired/stimulus";
import {
  createDataSource,
  deleteDataSource as deleteDataSourceAPI,
  testDataSource as testDataSourceAPI,
  updateDataSource,
} from "../lib/admin_api";
import {
  fetchAdminDataSourcesComponent,
} from "../lib/operational_component_api";
import { fetchServerComponent, renderServerBundle } from "../lib/server_component_api";
import type RecyclrController from "./recyclr_controller";
import { jsonRequest } from "../lib/api";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { setGlobalLoading } from "../lib/loading";

type StatusTone = "idle" | "busy" | "error" | "ok";

export default class AdminDataSourcesController extends Controller<HTMLElement> {
  private selectedID = "";
  private editingID = "";
  private offset = 0;

  connect(): void {
    void this.refresh();
  }

  private recyclrController(): RecyclrController {
    const controller = this.application.getControllerForElementAndIdentifier(document.body, "recyclr") as RecyclrController | null;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  private setStatus(message: string, tone: StatusTone): void {
    this.dispatch("status", { detail: { message, tone } });
  }

  private async refresh(): Promise<void> {
    const payload = await fetchAdminDataSourcesComponent(this.editingID, this.selectedID, this.offset);
    this.selectedID = payload.selected_source_id ?? "";
    this.offset = payload.offset ?? 0;
    await renderServerBundle(this.recyclrController(), payload, "Admin data sources");
  }

  private value(field: string): string {
    const input = this.element.querySelector(`[data-ds-field='${field}']`) as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null;
    if (!input) throw new Error(`Data-source field ${JSON.stringify(field)} is unavailable.`);
    return input.value.trim();
  }

  private optionalValue(field: string): string {
    return (this.element.querySelector(`[data-ds-field='${field}']`) as HTMLInputElement | HTMLSelectElement | null)?.value.trim() ?? "";
  }

  toggleDataSourceDSNPanel(): void {
    const useDSN = this.element.querySelector("[data-ds-field='use_dsn']") as HTMLInputElement | null;
    const fields = this.element.querySelector("[data-ds-connection-fields]");
    if (!useDSN || !fields) throw new Error("Data-source connection fields are unavailable.");
    fields.querySelectorAll("[data-ds-field='dsn']").forEach((node) => node.classList.toggle("ring-1", useDSN.checked));
  }

  toggleDataSourceExecutionPanel(): void {
    const mode = this.value("execution_mode");
    const direct = this.element.querySelector("[data-ds-direct-fields]");
    const delegated = this.element.querySelector("[data-ds-delegated-fields]");
    if (!direct || !delegated) throw new Error("Data-source execution fields are unavailable.");
    direct.classList.toggle("hidden", mode !== "direct");
    delegated.classList.toggle("hidden", mode !== "delegated");
  }

  async saveDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const executionMode = this.value("execution_mode") as "direct" | "delegated";
    const useDSN = executionMode === "direct" && ((this.element.querySelector("[data-ds-field='use_dsn']") as HTMLInputElement | null)?.checked ?? false);
    const port = executionMode === "direct" ? Number.parseInt(this.optionalValue("port"), 10) : 0;
    if (executionMode === "direct" && (!Number.isSafeInteger(port) || port < 1)) return reportErrorMessage(this.setStatus.bind(this), "Port must be a positive integer.");
    const input = {
      name: this.value("name"), driver: this.value("driver"), execution_mode: executionMode, use_dsn: useDSN,
      dsn: executionMode === "direct" ? this.optionalValue("dsn") : "",
      host: executionMode === "direct" ? this.optionalValue("host") : "", port,
      database_name: executionMode === "direct" ? this.optionalValue("database_name") : "",
      username: executionMode === "direct" ? this.optionalValue("username") : "",
      password: executionMode === "direct" ? this.optionalValue("password") : "",
      ssl_mode: executionMode === "direct" ? this.optionalValue("ssl_mode") : "",
      authority_url: executionMode === "delegated" ? this.optionalValue("authority_url") : "",
      credential_env: executionMode === "delegated" ? this.optionalValue("credential_env") : "",
    };
    if (!input.name) return reportErrorMessage(this.setStatus.bind(this), "Data-source name is required.");
    await this.mutate(this.editingID ? "Saving data source…" : "Adding data source…", async () => {
      const source = this.editingID ? await updateDataSource(this.editingID, input) : await createDataSource(input);
      this.selectedID = source.id;
      this.editingID = "";
    });
  }

  editDataSource(event: Event): void {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() ?? "";
    if (!id) throw new Error("Edit requires a data-source id.");
    this.editingID = id;
    this.selectedID = id;
    void this.refresh();
  }

  cancelEditDataSource(event: Event): void {
    event.preventDefault();
    this.editingID = "";
    void this.refresh();
  }

  selectDataSource(event: Event): void {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() ?? "";
    if (!id) throw new Error("Select requires a data-source id.");
    this.selectedID = id;
    void this.refresh();
  }

  loadDataSourcePage(event: Event): void {
    event.preventDefault();
    const offset = Number((event.currentTarget as HTMLElement).dataset.pageOffset ?? -1);
    if (!Number.isSafeInteger(offset) || offset < 0) throw new Error("Data-source page offset is invalid.");
    this.offset = offset;
    this.selectedID = "";
    this.editingID = "";
    void this.refresh();
  }

  async deleteDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() ?? "";
    if (!id || !window.confirm(`Remove data source ${id}?`)) return;
    await this.mutate("Removing data source…", async () => {
      await deleteDataSourceAPI(id);
      if (this.selectedID === id) this.selectedID = "";
      if (this.editingID === id) this.editingID = "";
    });
  }

  async testDataSource(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() ?? "";
    if (!id) throw new Error("Test requires a data-source id.");
    await this.mutate("Testing connection…", () => testDataSourceAPI(id));
  }

  async loadDataSourceSchema(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() || this.selectedID;
    if (!id) throw new Error("Load schema requires a selected data source.");
    try {
      const payload = await fetchServerComponent(`/v1/ui/admin/data-sources/schema?id=${encodeURIComponent(id)}`);
      await renderServerBundle(this.recyclrController(), payload, "Data-source schema");
      reportOk(this.setStatus.bind(this), "Schema loaded");
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    }
  }

  insertSchemaQuery(event: Event): void {
    event.preventDefault();
    const table = (event.currentTarget as HTMLElement).dataset.tableName?.trim() ?? "";
    const editor = this.element.querySelector("[data-ds-field='sql']") as HTMLTextAreaElement | null;
    if (!table || !editor) throw new Error("Schema query insertion target is unavailable.");
    editor.value = `SELECT * FROM ${table} LIMIT 20`;
    editor.focus();
  }

  async runDataSourceQuery(event: Event): Promise<void> {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() || this.selectedID;
    const sql = this.optionalValue("sql");
    if (!id || !sql) return reportErrorMessage(this.setStatus.bind(this), "Select a source and enter SQL.");
    this.setStatus("Running query…", "busy");
    try {
      const payload = await fetchServerComponent(`/v1/ui/admin/data-sources/query?id=${encodeURIComponent(id)}`, jsonRequest({ sql }));
      await renderServerBundle(this.recyclrController(), payload, "Data-source query result");
      reportOk(this.setStatus.bind(this), "Query completed");
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    }
  }

  private async mutate(message: string, action: () => Promise<unknown>): Promise<void> {
    this.setStatus(message, "busy");
    setGlobalLoading(true);
    try {
      await action();
      await this.refresh();
      reportOk(this.setStatus.bind(this), "Data sources ready");
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    } finally {
      setGlobalLoading(false);
    }
  }
}
