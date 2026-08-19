import type {
  MeterDeltaInput,
  RoleplayComponentResponse,
  RoleplayPageState,
  SceneInput,
} from "./roleplay_api";

export function requireRoleplayComponent(payload: Record<string, unknown>): RoleplayComponentResponse {
  const channelID = requireID(payload.channel_id, "response channel", /^[a-z0-9][a-z0-9_.:-]{0,95}$/);
  const worldID = roleplayID(payload.world_id, "response world", "rpw");
  if (typeof payload.configured !== "boolean") throw new Error("Roleplay response configured must be boolean.");
  const html = requireRecord(payload.html, "Roleplay response HTML");
  const bundle = requireText(html.bundle, "Roleplay response bundle", 2_000_000, false);
  const draftRevision = requireInteger(payload.scene_draft_revision, "Scene draft revision", 0, Number.MAX_SAFE_INTEGER);
  const result: RoleplayComponentResponse = {
    channel_id: channelID,
    world_id: worldID,
    configured: payload.configured,
    scene_draft_revision: draftRevision,
    html: { bundle },
  };
  if (payload.configured) {
    result.scene_revision = requireInteger(payload.scene_revision, "Scene revision", 1, Number.MAX_SAFE_INTEGER);
  } else if (payload.scene_revision !== undefined) {
    throw new Error("Unconfigured roleplay response cannot carry a scene revision.");
  }
  return result;
}

export function requireScene(input: SceneInput): SceneInput {
  const ids = input.participant_ids.map((id) => roleplayID(id, "scene participant", "rpc"));
  if (ids.length < 1 || ids.length > 16 || new Set(ids).size !== ids.length) {
    throw new Error("Scene participants must contain 1 to 16 unique character ids.");
  }
  return {
    title: requireText(input.title, "Scene title", 256, true),
    description: requireText(input.description, "Scene description", 1024, true),
    participant_ids: ids,
  };
}

export function requireEffects(effects: MeterDeltaInput[]): MeterDeltaInput[] {
  if (!Array.isArray(effects) || effects.length < 1 || effects.length > 8) {
    throw new Error("Effects must contain 1 to 8 meter deltas.");
  }
  const result = effects.map((effect) => ({
    meter_key: requireKey(effect.meter_key, "Effect meter"),
    delta: requireInteger(effect.delta, "Effect delta", -100_000, 100_000),
  }));
  if (result.some((effect) => effect.delta === 0) ||
      new Set(result.map((effect) => effect.meter_key)).size !== result.length) {
    throw new Error("Effects require unique meters and nonzero deltas.");
  }
  return result;
}

export function requirePage(page: RoleplayPageState): RoleplayPageState {
  const result = { ...page };
  for (const [key, value] of Object.entries(result)) {
    const offset = requireInteger(value, `${key} offset`, 0, Number.MAX_SAFE_INTEGER);
    if (offset % 4 !== 0) throw new Error(`${key} offset must use the server page size.`);
  }
  return result;
}

export function requireTextList(values: string[], source: string): string[] {
  if (!Array.isArray(values) || values.length > 16) throw new Error(`${source} must have at most 16 entries.`);
  const result = values.map((value) => requireText(value, source, 256, true));
  if (new Set(result).size !== result.length) throw new Error(`${source} contains duplicates.`);
  return result;
}

export function requireText(value: unknown, source: string, maximum: number, required: boolean): string {
  if (typeof value !== "string" || value.includes("\0") ||
      new TextEncoder().encode(value).byteLength > maximum || value !== value.trim()) {
    throw new Error(`${source} must be exact trimmed text.`);
  }
  if (required && !value) throw new Error(`${source} is required.`);
  return value;
}

export function requireID(value: unknown, source: string, pattern: RegExp): string {
  if (typeof value !== "string" || !pattern.test(value)) throw new Error(`${source} identity is invalid.`);
  return value;
}

export function roleplayID(value: unknown, source: string, prefix: string): string {
  return requireID(value, source, new RegExp(`^${prefix}_[0-9a-f]{32}$`));
}

export function requireKey(value: unknown, source: string): string {
  return requireID(value, source, /^[a-z][a-z0-9-]{0,31}$/);
}

export function requireInteger(value: unknown, source: string, minimum: number, maximum: number): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum || value > maximum) {
    throw new Error(`${source} must be an integer between ${minimum} and ${maximum}.`);
  }
  return value;
}

export function requireDirection(value: unknown): "at_or_below" | "at_or_above" {
  if (value !== "at_or_below" && value !== "at_or_above") {
    throw new Error("Item trigger direction is invalid.");
  }
  return value;
}

function requireRecord(value: unknown, source: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`${source} must be an object.`);
  return value as Record<string, unknown>;
}
