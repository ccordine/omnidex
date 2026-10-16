import { jsonPut, jsonRequest, readJSON } from "./api";
import { exactRecord, exactString } from "./response_validation";

export const AGENT_ROLES = ["planner", "ledger_parser", "executor", "reviewer"] as const;
export type AgentRole = typeof AGENT_ROLES[number];
export type AgentRouteSettings = { provider: string; model: string; effort?: string; service_tier?: string };
export type AgentConfiguration = { mode: "agents" } & Record<AgentRole, AgentRouteSettings>;
export type AgentModel = {
  id: string; display_name: string; default_effort: string;
  description?: string; is_default?: boolean; hidden?: boolean; default_service_tier?: string | null;
  efforts: Array<{ value: string; description: string }>;
  service_tiers: Array<{ id: string; name: string; description: string }>;
};
export type AgentSettings = {
  configuration: AgentConfiguration; models: AgentModel[];
  installation: { version: string; can_update: boolean; update_method?: string };
  source: "saved" | "startup";
};

const endpoint = "/v1/coding-agents";

export function fetchAgentSettings(signal?: AbortSignal): Promise<AgentSettings> {
  return requestSettings(`${endpoint}/settings`, { signal });
}

export function saveAgentSettings(configuration: AgentConfiguration, signal?: AbortSignal): Promise<AgentSettings> {
  return requestSettings(`${endpoint}/settings`, { ...jsonPut({ configuration }), signal });
}

export function refreshAgentModels(signal?: AbortSignal): Promise<AgentSettings> {
  return requestSettings(`${endpoint}/refresh`, { ...jsonRequest({}), signal });
}

export function updateAgentInstallation(signal?: AbortSignal): Promise<AgentSettings> {
  return requestSettings(`${endpoint}/update`, { ...jsonRequest({}), signal });
}

async function requestSettings(url: string, options: RequestInit): Promise<AgentSettings> {
  return parseAgentSettings(await readJSON(await fetch(url, options)));
}

export function parseAgentSettings(value: unknown): AgentSettings {
  const root = exactRecord(value, "Agent settings", ["configuration", "models", "installation", "source"]);
  const config = exactRecord(root.configuration, "Agent configuration", ["mode", ...AGENT_ROLES]);
  if (config.mode !== "agents") throw new Error("Agent settings changed the execution mode.");
  const configuration = { mode: "agents" } as AgentConfiguration;
  for (const role of AGENT_ROLES) {
    const route = exactRecord(config[role], `${role} route`, ["provider", "model", "effort", "service_tier"], ["provider", "model"]);
    configuration[role] = {
      provider: token(route.provider, `${role} provider`), model: token(route.model, `${role} model`),
      ...(route.effort === undefined ? {} : { effort: token(route.effort, `${role} effort`, true) }),
      ...(route.service_tier === undefined ? {} : { service_tier: token(route.service_tier, `${role} service tier`, true) }),
    };
  }
  const models = array(root.models, "Agent models", 512).map((raw) => {
    const model = exactRecord(raw, "Agent model", ["id", "display_name", "default_effort", "efforts", "service_tiers", "description", "is_default", "hidden", "default_service_tier"], ["id", "display_name", "default_effort", "efforts", "service_tiers"]);
    const efforts = array(model.efforts, "Model efforts", 32).map((raw) => {
      const effort = exactRecord(raw, "Model effort", ["value", "description"]);
      return { value: token(effort.value, "Model effort value"), description: description(effort.description) };
    });
    const service_tiers = array(model.service_tiers, "Model service tiers", 32).map((raw) => {
      const tier = exactRecord(raw, "Model service tier", ["id", "name", "description"]);
      return { id: token(tier.id, "Model service tier id"), name: token(tier.name, "Model service tier name"), description: description(tier.description) };
    });
    unique(efforts.map((effort) => effort.value), "Model efforts");
    unique(service_tiers.map((tier) => tier.id), "Model service tiers");
    for (const field of ["is_default", "hidden"]) {
      if (model[field] !== undefined && typeof model[field] !== "boolean") throw new Error(`Model ${field} must be boolean.`);
    }
    return {
      id: token(model.id, "Model id"), display_name: token(model.display_name, "Model display name"), default_effort: token(model.default_effort, "Model default effort", true), efforts, service_tiers,
      ...(model.description === undefined ? {} : { description: description(model.description) }),
      ...(model.is_default === undefined ? {} : { is_default: model.is_default as boolean }),
      ...(model.hidden === undefined ? {} : { hidden: model.hidden as boolean }),
      ...(model.default_service_tier === undefined ? {} : { default_service_tier: model.default_service_tier === null ? null : token(model.default_service_tier, "Model default service tier") }),
    };
  });
  unique(models.map((model) => model.id), "Agent models");
  const installation = exactRecord(root.installation, "Codex installation", ["version", "can_update", "update_method"], ["version", "can_update"]);
  if (typeof installation.can_update !== "boolean") throw new Error("Codex installation update support must be boolean.");
  if (root.source !== "saved" && root.source !== "startup") throw new Error("Agent settings has an unknown source.");
  return {
    configuration, models,
    installation: {
      version: token(installation.version, "Codex version", true), can_update: installation.can_update,
      ...(installation.update_method === undefined ? {} : { update_method: token(installation.update_method, "Codex update method") }),
    },
    source: root.source,
  };
}

function token(value: unknown, label: string, empty = false): string {
  return exactString(value, label, { maxBytes: 512, canonical: true, nonblank: !empty });
}

function description(value: unknown): string { return exactString(value, "Model description", { maxBytes: 8192 }); }

function array(value: unknown, label: string, maximum: number): unknown[] {
  if (!Array.isArray(value) || value.length > maximum) throw new Error(`${label} exceeds its item bound.`);
  return value;
}

function unique(values: string[], label: string): void {
  if (new Set(values).size !== values.length) throw new Error(`${label} repeats a catalog identity.`);
}
