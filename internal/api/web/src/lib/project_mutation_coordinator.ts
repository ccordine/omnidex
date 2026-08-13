import {
  deleteProject,
  pauseProjectAutoWork,
  scanProjectMap,
  startProjectAutoWork,
  surveyProject,
  updateProject,
} from "./project_api";
import { patchScrumAutoWork } from "./scrum_api";
import { clearConfigValues, collectConfigValues } from "./config_form_values";
import { setGlobalLoading } from "./loading";

export type ProjectMutationHost = {
  detailRoot(): HTMLElement;
  selectedProjectID(): number | null;
  reloadDetail(): Promise<void>;
  reloadList(): Promise<void>;
  projectDeleted(): void;
  setStatus(message: string, tone?: "idle" | "busy" | "error" | "ok"): void;
  success(message: string): void;
  failure(error: unknown): void;
};

export class ProjectMutationCoordinator {
  constructor(private readonly host: ProjectMutationHost) {}

  private id(event?: Event): number {
    const eventID = Number((event?.currentTarget as HTMLElement | null)?.dataset.projectId || 0);
    const id = eventID || this.host.selectedProjectID() || 0;
    if (!id) throw new Error("Project action requires a project id.");
    return id;
  }

  private field(name: string): string {
    const node = this.host.detailRoot().querySelector(`[data-projects-field='${name}']`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    return node?.value?.trim() ?? "";
  }

  async saveProject(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    await this.run("Saving project…", "Project saved", async () => {
      await updateProject(id, { name: this.field("name"), location: this.field("location"), description: this.field("description") });
      await this.host.reloadDetail();
    });
  }

  async saveRecipe(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    let recipe: Record<string, unknown>;
    try { recipe = JSON.parse(this.field("recipeJson")) as Record<string, unknown>; }
    catch { return this.host.failure(new Error("Recipe JSON is invalid.")); }
    await this.run("Saving recipe…", "Recipe saved", async () => {
      await updateProject(id, { recipe_id: this.field("recipeId"), recipe });
      await this.host.reloadDetail();
    });
  }

  async saveModelConfig(event: Event): Promise<void> {
    event.preventDefault();
    await this.saveConfig("project-model", "model_config", "Model settings saved");
  }

  async clearModelConfig(event: Event): Promise<void> {
    event.preventDefault();
    clearConfigValues(this.host.detailRoot(), "project-model");
    await this.saveConfig("project-model", "model_config", "Model overrides cleared");
  }

  async saveAgentConfig(event: Event): Promise<void> {
    event.preventDefault();
    await this.saveConfig("project-agent", "agent_config", "Agent settings saved");
  }

  async clearAgentConfig(event: Event): Promise<void> {
    event.preventDefault();
    clearConfigValues(this.host.detailRoot(), "project-agent");
    await this.saveConfig("project-agent", "agent_config", "Agent overrides cleared");
  }

  private async saveConfig(scope: string, key: "model_config" | "agent_config", message: string): Promise<void> {
    const id = this.id();
    const values = collectConfigValues(this.host.detailRoot(), scope);
    await this.run("Saving configuration…", message, async () => {
      await updateProject(id, { [key]: values });
      await this.host.reloadDetail();
    });
  }

  async saveScrumAutomation(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    const enabled = Boolean((this.host.detailRoot().querySelector("[data-projects-field='autoWorkEnabled']") as HTMLInputElement | null)?.checked);
    const source_columns = [...this.host.detailRoot().querySelectorAll<HTMLInputElement>("[data-projects-field='autoWorkColumn']:checked")]
      .map((node) => node.dataset.autoWorkColumn?.trim() ?? "").filter(Boolean);
    await this.run("Saving automation…", "Scrum automation saved", async () => {
      await patchScrumAutoWork({ enabled, source_columns }, id);
      await this.host.reloadDetail();
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: id } }));
    });
  }

  async rescanProject(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    await this.run("Detecting project stack…", "Project stack detected", async () => { await surveyProject(id); await this.host.reloadDetail(); });
  }

  async scanProjectMap(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    await this.run("Scanning project…", "Codebase map updated", async () => { await scanProjectMap(id); await this.host.reloadDetail(); });
  }

  async refreshProjectGit(event: Event): Promise<void> {
    event.preventDefault();
    this.id(event);
    await this.run("Refreshing Git status…", "Git status refreshed", () => this.host.reloadDetail());
  }

  async startAutoWork(event: Event): Promise<void> {
    event.preventDefault(); event.stopPropagation();
    const id = this.id(event);
    await this.run("Starting auto-work…", "Auto-work started", async () => { await startProjectAutoWork(id); await this.host.reloadList(); });
  }

  async pauseAutoWork(event: Event): Promise<void> {
    event.preventDefault(); event.stopPropagation();
    const id = this.id(event);
    await this.run("Pausing auto-work…", "Auto-work paused", async () => { await pauseProjectAutoWork(id); await this.host.reloadList(); });
  }

  async deleteProject(event: Event): Promise<void> {
    event.preventDefault();
    const id = this.id(event);
    if (!window.confirm("Delete this project and its Scrum cards?")) return;
    await this.run("Deleting project…", "Project deleted", async () => { await deleteProject(id); this.host.projectDeleted(); await this.host.reloadList(); });
  }

  private async run(working: string, success: string, action: () => Promise<void>): Promise<void> {
    this.host.setStatus(working, "busy");
    setGlobalLoading(true);
    try { await action(); this.host.success(success); }
    catch (error) { this.host.failure(error); }
    finally { setGlobalLoading(false); }
  }
}
