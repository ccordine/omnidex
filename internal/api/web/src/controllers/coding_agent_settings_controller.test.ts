import { Application } from "@hotwired/stimulus";
import { afterEach, describe, expect, it, vi } from "vitest";
import CodingAgentSettingsController from "./coding_agent_settings_controller";
import * as api from "../lib/coding_agent_settings_api";
import type { AgentRole, AgentSettings } from "../lib/coding_agent_settings_api";

vi.mock("../lib/coding_agent_settings_api", async (original) => ({
  ...await original<typeof api>(), fetchAgentSettings: vi.fn(), saveAgentSettings: vi.fn(), refreshAgentModels: vi.fn(), updateAgentInstallation: vi.fn(),
}));

function settings(): AgentSettings {
  const route = { provider: "codex", model: "future-model", effort: "ultra", service_tier: "fast" };
  return {
    configuration: { mode: "agents", planner: { ...route }, executor: { ...route }, reviewer: { ...route } },
    models: [{ id: "future-model", display_name: "Future <model>", default_effort: "max", efforts: ["low", "xhigh", "max", "ultra"].map((value) => ({ value, description: `${value} reasoning` })), service_tiers: ["default", "fast", "new-tier"].map((id) => ({ id, name: id, description: `${id} processing` })) }],
    installation: { version: "0.999.0", can_update: true }, source: "startup",
  };
}

let application: Application | undefined;
afterEach(() => { application?.stop(); application = undefined; document.body.innerHTML = ""; vi.clearAllMocks(); });

async function mount(payload = settings()) {
  vi.mocked(api.fetchAgentSettings).mockResolvedValue(payload);
  document.body.innerHTML = `<section data-controller="coding-agent-settings">
    ${api.AGENT_ROLES.map((role) => ["model", "effort", "service_tier"].map((field) => `<select data-agent-role="${role}" data-agent-field="${field}"></select>`).join("")).join("")}
    <p data-coding-agent-settings-target="status"></p><span data-coding-agent-settings-target="version"></span>
    ${["save", "refresh", "update"].map((target) => `<button data-coding-agent-settings-target="${target}">${target}</button>`).join("")}
  </section>`;
  const element = document.body.firstElementChild as HTMLElement;
  application = Application.start();
  application.register("coding-agent-settings", CodingAgentSettingsController);
  await vi.waitFor(() => expect(element.getAttribute("aria-busy")).toBe("false"));
  const controller = application.getControllerForElementAndIdentifier(element, "coding-agent-settings") as CodingAgentSettingsController;
  const field = (role: AgentRole, name: string) => element.querySelector(`[data-agent-role="${role}"][data-agent-field="${name}"]`) as HTMLSelectElement;
  const button = (name: string) => element.querySelector(`[data-coding-agent-settings-target="${name}"]`) as HTMLButtonElement;
  return { controller, element, field, button };
}

describe("Agent settings controller", () => {
  it("shows all advertised effort/tier options and safely presents saved values and CLI version", async () => {
    const { controller, field, button, element } = await mount();
    expect(Array.from(field("planner", "effort").options, (option) => option.value)).toEqual(["", "low", "xhigh", "max", "ultra"]);
    expect(Array.from(field("planner", "service_tier").options, (option) => option.value)).toEqual(["default", "fast", "new-tier"]);
    expect(field("planner", "model").value).toBe("future-model");
    expect(field("planner", "effort").value).toBe("ultra");
    expect(field("planner", "service_tier").value).toBe("fast");
    expect(element.querySelector("model")).toBeNull();
    expect(controller.versionTarget.textContent).toBe("Codex 0.999.0");
    expect(button("save").disabled).toBe(false);
  });

  it("saves every route's explicit model/effort/speed without changing the mode", async () => {
    const { controller, field } = await mount();
    field("planner", "effort").value = "xhigh";
    field("executor", "service_tier").value = "new-tier";
    field("reviewer", "service_tier").value = "default";
    vi.mocked(api.saveAgentSettings).mockImplementation(async (configuration) => ({ ...settings(), configuration, source: "saved" }));
    await controller.save();
    expect(api.saveAgentSettings).toHaveBeenCalledWith({
      mode: "agents", planner: { provider: "codex", model: "future-model", effort: "xhigh", service_tier: "fast" },
      executor: { provider: "codex", model: "future-model", effort: "ultra", service_tier: "new-tier" },
      reviewer: { provider: "codex", model: "future-model", effort: "ultra", service_tier: "default" },
    }, expect.any(AbortSignal));
    expect(controller.statusTarget.textContent).toBe("Saved");
  });

  it("refreshes new catalog models while preserving unsaved choices", async () => {
    const { controller, field } = await mount();
    field("planner", "effort").value = "max";
    const refreshed = settings(); refreshed.models.push({ ...refreshed.models[0], id: "just-released", display_name: "Just released" });
    vi.mocked(api.refreshAgentModels).mockResolvedValue(refreshed);
    await controller.refresh();
    expect(Array.from(field("planner", "model").options, (option) => option.value)).toContain("just-released");
    expect(field("planner", "effort").value).toBe("max");
    expect(controller.statusTarget.textContent).toBe("Models refreshed");
  });

  it("retains the exact choices on a catalog error and supports an explicit retry", async () => {
    const { controller, field } = await mount();
    field("executor", "effort").value = "xhigh";
    vi.mocked(api.refreshAgentModels).mockRejectedValueOnce(new Error("Codex model catalog unavailable")).mockResolvedValue(settings());
    await controller.refresh();
    expect(controller.statusTarget.textContent).toBe("Codex model catalog unavailable");
    expect(field("executor", "effort").value).toBe("xhigh");
    await controller.refresh();
    expect(field("executor", "effort").value).toBe("xhigh");
  });

  it("updates installed CLI and preserves the unsaved configuration", async () => {
    const { controller, field } = await mount();
    field("reviewer", "effort").value = "low";
    const updated = settings(); updated.installation.version = "1.0.0";
    vi.mocked(api.updateAgentInstallation).mockResolvedValue(updated);
    await controller.update();
    expect(api.updateAgentInstallation).toHaveBeenCalledWith(expect.any(AbortSignal));
    expect(controller.versionTarget.textContent).toBe("Codex 1.0.0");
    expect(field("reviewer", "effort").value).toBe("low");
  });

  it("keeps unsupported provider values visible and blocks edits and save without falling back", async () => {
    const payload = settings(); payload.configuration.executor.provider = "future-agent";
    const { controller, field, button } = await mount(payload);
    expect(field("executor", "model").value).toBe("future-model");
    expect(field("executor", "model").disabled).toBe(true);
    expect(button("save").disabled).toBe(true);
    expect(controller.statusTarget.textContent).toMatch(/future-agent.*not supported/i);
    await controller.save();
    expect(api.saveAgentSettings).not.toHaveBeenCalled();
  });

  it("keeps an unavailable saved model visible until the user chooses a listed model", async () => {
    const payload = settings(); payload.configuration.planner.model = "retired-model";
    const { controller, field, button } = await mount(payload);
    expect(field("planner", "model").value).toBe("retired-model");
    expect(button("save").disabled).toBe(true);
    field("planner", "model").value = "future-model";
    controller.modelChanged({ currentTarget: field("planner", "model") } as unknown as Event);
    expect(field("planner", "effort").value).toBe("max");
    expect(field("planner", "service_tier").value).toBe("default");
    expect(button("save").disabled).toBe(false);
  });

  it("disables Update when the installed CLI has no supported updater", async () => {
    const payload = settings(); payload.installation.can_update = false;
    const { controller, button } = await mount(payload);
    expect(button("update").disabled).toBe(true);
    await controller.update();
    expect(api.updateAgentInstallation).not.toHaveBeenCalled();
  });

  it("cancels its pending request and prevents a detached view from applying the result", async () => {
    const { controller } = await mount();
    let resolve!: (value: AgentSettings) => void;
    vi.mocked(api.refreshAgentModels).mockImplementation(() => new Promise((done) => { resolve = done; }));
    const refresh = controller.refresh();
    const signal = vi.mocked(api.refreshAgentModels).mock.calls.at(-1)![0]!;
    controller.disconnect();
    expect(signal.aborted).toBe(true);
    const updated = settings(); updated.installation.version = "detached";
    resolve(updated); await refresh;
    expect(controller.versionTarget.textContent).toBe("Codex 0.999.0");
  });
});
