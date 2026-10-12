import { Controller } from "@hotwired/stimulus";
import {
  AGENT_ROLES, fetchAgentSettings, refreshAgentModels, saveAgentSettings, updateAgentInstallation,
  type AgentConfiguration, type AgentModel, type AgentRole, type AgentSettings,
} from "../lib/coding_agent_settings_api";

export default class CodingAgentSettingsController extends Controller<HTMLElement> {
  static targets = ["status", "version", "save", "refresh", "update"];
  declare readonly statusTarget: HTMLElement;
  declare readonly versionTarget: HTMLElement;
  declare readonly saveTarget: HTMLButtonElement;
  declare readonly refreshTarget: HTMLButtonElement;
  declare readonly updateTarget: HTMLButtonElement;
  private settings?: AgentSettings;
  private busy = false;
  private mounted = false;
  private request?: AbortController;

  connect(): void {
    this.mounted = true;
    void this.run("Loading…", "Ready", fetchAgentSettings);
  }

  disconnect(): void {
    this.mounted = false;
    this.request?.abort();
  }

  async save(event?: Event): Promise<void> {
    event?.preventDefault();
    if (this.busy) return;
    try {
      const configuration = this.draft();
      this.validateDraft(configuration);
      await this.run("Saving…", "Saved", (signal) => saveAgentSettings(configuration, signal));
    } catch (error) { this.status(errorMessage(error), "error"); }
  }

  async refresh(event?: Event): Promise<void> {
    event?.preventDefault();
    await this.run("Refreshing…", "Models refreshed", refreshAgentModels, true);
  }

  async update(event?: Event): Promise<void> {
    event?.preventDefault();
    if (!this.settings?.installation.can_update) return;
    await this.run("Updating…", "Codex updated", updateAgentInstallation, true);
  }

  modelChanged(event: Event): void {
    const role = (event.currentTarget as HTMLSelectElement).dataset.agentRole as AgentRole;
    if (!AGENT_ROLES.includes(role)) throw new Error("Agent model change has an unknown role.");
    const model = this.model(this.field(role, "model").value);
    if (model) {
      this.options(this.field(role, "effort"), [{ value: "", label: "Default" }, ...model.efforts.map((effort) => ({ value: effort.value, label: effort.value, title: effort.description }))], model.default_effort);
      this.options(this.field(role, "service_tier"), this.tierOptions(model), "default");
    }
    this.availability();
    this.status("Unsaved", "idle");
  }

  changed(): void { this.availability(); this.status("Unsaved", "idle"); }

  private async run(working: string, success: string, action: (signal?: AbortSignal) => Promise<AgentSettings>, preserveDraft = false): Promise<void> {
    if (this.busy) return;
    const draft = preserveDraft && this.settings ? this.draft() : undefined;
    this.busy = true;
    this.request = new AbortController();
    this.availability();
    this.status(working, "busy");
    try {
      const settings = await action(this.request.signal);
      if (!this.mounted) return;
      this.settings = settings;
      this.render(draft ?? settings.configuration);
      const issue = this.configurationIssue();
      this.status(issue ?? success, issue ? "error" : success === "Saved" ? "ok" : "idle");
    } catch (error) {
      if (this.mounted) this.status(errorMessage(error), "error");
    } finally {
      this.busy = false;
      if (this.mounted) this.availability();
    }
  }

  private render(configuration: AgentConfiguration): void {
    for (const role of AGENT_ROLES) {
      const route = configuration[role];
      this.options(this.field(role, "model"), this.settings!.models.map((model) => ({ value: model.id, label: model.display_name })), route.model);
      const model = this.model(route.model);
      this.options(this.field(role, "effort"), [{ value: "", label: "Default" }, ...(model?.efforts ?? []).map((effort) => ({ value: effort.value, label: effort.value, title: effort.description }))], route.effort ?? "");
      this.options(this.field(role, "service_tier"), model ? this.tierOptions(model) : [{ value: "default", label: "Default" }], route.service_tier || "default");
    }
    this.versionTarget.textContent = this.settings!.installation.version ? `Codex ${this.settings!.installation.version}` : "Codex version unavailable";
  }

  private tierOptions(model: AgentModel): Array<{ value: string; label: string; title?: string }> {
    const tiers = model.service_tiers.map((tier) => ({ value: tier.id, label: tier.name, title: tier.description }));
    return tiers.some((tier) => tier.value === "default") ? tiers : [{ value: "default", label: "Default" }, ...tiers];
  }

  private draft(): AgentConfiguration {
    if (!this.settings) throw new Error("Load Agent settings before saving.");
    const configuration = { mode: "agents" } as AgentConfiguration;
    for (const role of AGENT_ROLES) {
      configuration[role] = {
        provider: this.settings.configuration[role].provider, model: this.field(role, "model").value,
        effort: this.field(role, "effort").value, service_tier: this.field(role, "service_tier").value,
      };
    }
    return configuration;
  }

  private validateDraft(configuration: AgentConfiguration): void {
    for (const role of AGENT_ROLES) {
      const route = configuration[role];
      if (route.provider !== "codex") throw new Error(`${role} provider ${route.provider} is not supported by these Codex settings.`);
      const model = this.model(route.model);
      if (!model) throw new Error(`${role} model ${route.model} is unavailable. Refresh or choose an available model.`);
      if (route.effort && !model.efforts.some((effort) => effort.value === route.effort)) throw new Error(`${role} effort ${route.effort} is unavailable for this model.`);
      if (route.service_tier !== "default" && !model.service_tiers.some((tier) => tier.id === route.service_tier)) throw new Error(`${role} speed ${route.service_tier} is unavailable for this model.`);
    }
  }

  private configurationIssue(): string | undefined {
    try { this.validateDraft(this.draft()); return undefined; }
    catch (error) { return errorMessage(error); }
  }

  private availability(): void {
    for (const role of AGENT_ROLES) {
      const disabled = this.busy || !this.settings || this.settings.configuration[role].provider !== "codex";
      for (const field of ["model", "effort", "service_tier"] as const) this.field(role, field).disabled = disabled;
    }
    this.saveTarget.disabled = this.busy || !this.settings || this.configurationIssue() !== undefined;
    this.refreshTarget.disabled = this.busy;
    this.updateTarget.disabled = this.busy || !this.settings?.installation.can_update;
    this.element.setAttribute("aria-busy", String(this.busy));
  }

  private field(role: AgentRole, name: string): HTMLSelectElement {
    const field = this.element.querySelector(`[data-agent-role="${role}"][data-agent-field="${name}"]`);
    if (!(field instanceof HTMLSelectElement)) throw new Error(`Agent settings has no ${role} ${name} field.`);
    return field;
  }

  private model(id: string): AgentModel | undefined { return this.settings?.models.find((model) => model.id === id); }

  private options(select: HTMLSelectElement, options: Array<{ value: string; label: string; title?: string }>, selected: string): void {
    const nodes = options.map((option) => { const node = new Option(option.label, option.value); node.title = option.title ?? ""; return node; });
    if (!options.some((option) => option.value === selected)) nodes.unshift(new Option(`${selected} (unavailable)`, selected));
    select.replaceChildren(...nodes);
    select.value = selected;
  }

  private status(message: string, tone: "idle" | "busy" | "error" | "ok"): void {
    this.statusTarget.textContent = message;
    this.statusTarget.classList.toggle("text-rose-300", tone === "error");
    this.dispatch("status", { detail: { message, tone } });
  }
}

function errorMessage(error: unknown): string { return error instanceof Error ? error.message : String(error); }
