import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchAgentSettings, parseAgentSettings, refreshAgentModels, saveAgentSettings, updateAgentInstallation } from "./coding_agent_settings_api";

function payload() {
  const route = { provider: "codex", model: "future-model", effort: "ultra", service_tier: "fast" };
  return {
    configuration: { mode: "agents", planner: route, ledger_parser: route, executor: route, reviewer: route },
    models: [{ provider: "codex", id: "future-model", display_name: "Future model", default_effort: "ultra", efforts: [{ value: "ultra", description: "Most reasoning" }], service_tiers: [{ id: "fast", name: "Fast", description: "Priority processing" }, { id: "new-tier", name: "New tier", description: "Account supplied tier" }] }],
    installation: { version: "codex 0.999.0", can_update: true }, source: "saved",
  };
}

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("Agent settings API", () => {
  it("retains distinct provider identities when local and native model names match", () => {
    const value = payload();
    value.models.push({ provider: "ollama", id: "future-model", display_name: "Local parser", default_effort: "", efforts: [], service_tiers: [] });
    const settings = parseAgentSettings(value);
    expect(settings.models.map((model) => [model.provider, model.id])).toEqual([["codex", "future-model"], ["ollama", "future-model"]]);
  });

  it("accepts future advertised models, efforts and service tiers without a hardcoded inventory", () => {
    const settings = parseAgentSettings(payload());
    expect(settings.models[0].efforts[0].value).toBe("ultra");
    expect(settings.models[0].service_tiers[1].id).toBe("new-tier");
  });

  it("preserves omitted route defaults and an unavailable saved model for explicit presentation", () => {
    const value = payload();
    Object.assign(value.configuration.planner, { effort: undefined, service_tier: undefined, model: "no-longer-listed" });
    const settings = parseAgentSettings(value);
    expect(settings.configuration.planner).toEqual({ provider: "codex", model: "no-longer-listed" });
  });

  it("accepts the installed native catalog and installation metadata DTO", () => {
    const value = payload();
    Object.assign(value.models[0], { description: "Native account model", is_default: true, hidden: false, default_service_tier: null });
    Object.assign(value.installation, { update_method: "native" });
    const settings = parseAgentSettings(value);
    expect(settings.models[0]).toMatchObject({ description: "Native account model", is_default: true, hidden: false, default_service_tier: null });
    expect(settings.installation.update_method).toBe("native");
    Object.assign(value.models[0], { default_service_tier: "new-tier" });
    expect(parseAgentSettings(value).models[0].default_service_tier).toBe("new-tier");
  });

  it.each([
    { hidden: "false" }, { is_default: null }, { default_service_tier: {} }, { description: 1 },
  ])("rejects malformed native model metadata", (metadata) => {
    const value = payload(); Object.assign(value.models[0], metadata);
    expect(() => parseAgentSettings(value)).toThrow();
  });

  it.each([
    (value: ReturnType<typeof payload>) => { value.configuration.mode = "normal"; },
    (value: ReturnType<typeof payload>) => { value.models.push(value.models[0]); },
    (value: ReturnType<typeof payload>) => { value.models[0].efforts.push(value.models[0].efforts[0]); },
    (value: ReturnType<typeof payload>) => { value.models[0].service_tiers.push(value.models[0].service_tiers[0]); },
    (value: ReturnType<typeof payload>) => { (value.installation.can_update as unknown) = "yes"; },
    (value: ReturnType<typeof payload>) => { (value.configuration.planner.effort as unknown) = null; },
    (value: ReturnType<typeof payload>) => { Object.assign(value, { access_token: "ungranted" }); },
  ])("rejects malformed or ambiguous catalog authority", (change) => {
    const value = payload(); change(value);
    expect(() => parseAgentSettings(value)).toThrow();
  });

  it("uses the exact GET/PUT/refresh/update contracts and owned abort signal", async () => {
    const value = payload();
    const fetchMock = vi.fn(async (_url: string, _options: RequestInit) => new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const signal = new AbortController().signal;
    const settings = await fetchAgentSettings(signal);
    await saveAgentSettings(settings.configuration, signal);
    await refreshAgentModels(signal);
    await updateAgentInstallation(signal);
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(["/v1/coding-agents/settings", "/v1/coding-agents/settings", "/v1/coding-agents/refresh", "/v1/coding-agents/update"]);
    const options = fetchMock.mock.calls.map(([, init]) => init as RequestInit);
    expect(options[0]).toEqual({ signal });
    expect(options[1].method).toBe("PUT");
    expect(JSON.parse(String(options[1].body))).toEqual({ configuration: settings.configuration });
    expect(options[2].method).toBe("POST"); expect(options[3].method).toBe("POST");
    expect(options.every((option) => option.signal === signal)).toBe(true);
  });
});
