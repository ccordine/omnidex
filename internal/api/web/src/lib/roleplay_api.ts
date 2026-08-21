import { jsonPut, jsonRequest, readJSON } from "./api";
import {
  requireDirection,
  requireEffects,
  requireID,
  requireInteger,
  requireKey,
  requirePage,
  requireRoleplayComponent,
  requireScene,
  requireText,
  requireTextList,
  roleplayID,
} from "./roleplay_api_validation";

export interface RoleplayPageState {
  characters: number;
  personas: number;
  turn_order: number;
  meters: number;
  inventory: number;
  interactions: number;
  item_templates: number;
}

export interface RoleplayComponentResponse {
  channel_id: string;
  world_id: string;
  configured: boolean;
  scene_revision?: number;
  scene_draft_revision: number;
  composer_persona_character_id?: string;
  html: { bundle: string };
}

export function createRoleplayUserPersona(
  channelID: string,
  name: string,
): Promise<RoleplayComponentResponse> {
  return mutate(channelID, "/user-personas", jsonRequest({
    name: requireText(name, "Identity name", 256, true),
  }), 201);
}

export interface PersonaInput {
  expected_revision: number;
  summary: string;
  voice: string;
  traits: string[];
  goals: string[];
}

export interface SceneInput {
  title: string;
  description: string;
  participant_ids: string[];
}

export interface SceneCreateInput extends SceneInput {
  expected_draft_revision: number;
}

export interface SceneUpdateInput extends SceneInput {
  expected_draft_revision: number;
}

export interface SceneDraftParticipantInput {
  expected_revision: number;
  selected: boolean;
  characters_offset: number;
}

export interface ResponderOrderInput {
  expected_revision: number;
  character_ids: string[];
}

export interface MeterDefinitionInput {
  key: string;
  name: string;
  minimum: number;
  maximum: number;
  initial_value: number;
}

export interface MeterValueInput {
  expected_revision: number;
  value: number;
}

export interface ResearchCapabilityInput {
  enabled: boolean;
}

export interface RoleplayGenerationInput {
  expected_revision: number;
  narrative_model: string;
}

export interface MeterDeltaInput {
  meter_key: string;
  delta: number;
}

export interface InteractionDefinitionInput {
  key: string;
  name: string;
  description: string;
  argument_mode: "none" | "required";
  effects: MeterDeltaInput[];
}

export interface ItemDefinitionInput {
  name: string;
  description: string;
  use_policy: "finite" | "infinite";
  initial_uses: number;
  trigger: null | { meter_key: string; direction: "at_or_below" | "at_or_above"; threshold: number };
  priority: number;
  effects: MeterDeltaInput[];
}

export const emptyRoleplayPage: RoleplayPageState = {
  characters: 0,
  personas: 0,
  turn_order: 0,
  meters: 0,
  inventory: 0,
  interactions: 0,
  item_templates: 0,
};

export async function fetchRoleplayComponent(
  channelID: string,
  page: RoleplayPageState = emptyRoleplayPage,
): Promise<RoleplayComponentResponse> {
  const id = requireID(channelID, "channel", /^[a-z0-9][a-z0-9_.:-]{0,95}$/);
  const query = new URLSearchParams({ channel_id: id });
  for (const [key, value] of Object.entries(requirePage(page))) query.set(`${key}_offset`, String(value));
  return requestRoleplayComponent(`/v1/ui/chat/roleplay?${query}`, undefined, 200, id);
}

export function placeRoleplayLibraryCharacter(
  channelID: string,
  libraryCharacterID: string,
): Promise<RoleplayComponentResponse> {
  return mutate(
    channelID,
    `/library/${roleplayID(libraryCharacterID, "library character", "rpl")}`,
    { method: "POST" },
    201,
  );
}

export function writeRoleplayPersona(
  channelID: string,
  characterID: string,
  input: PersonaInput,
): Promise<RoleplayComponentResponse> {
  const body = {
    expected_revision: requireInteger(input.expected_revision, "Persona revision", 0, Number.MAX_SAFE_INTEGER),
    summary: requireText(input.summary, "Persona summary", 1024, true),
    voice: requireText(input.voice, "Persona voice", 1024, false),
    traits: requireTextList(input.traits, "Persona traits"),
    goals: requireTextList(input.goals, "Persona goals"),
  };
  return mutate(channelID, `/personas/${roleplayID(characterID, "character", "rpc")}`, jsonPut(body), 200);
}

export function createRoleplayScene(channelID: string, input: SceneCreateInput): Promise<RoleplayComponentResponse> {
	return mutate(channelID, "/scene", jsonRequest({
		expected_draft_revision: requireInteger(input.expected_draft_revision, "Scene draft revision", 0, Number.MAX_SAFE_INTEGER),
		...requireScene(input),
	}), 201);
}

export function updateRoleplayScene(
  channelID: string,
  expectedRevision: number,
  input: SceneUpdateInput,
): Promise<RoleplayComponentResponse> {
  return mutate(channelID, "/scene", jsonPut({
    expected_revision: requireInteger(expectedRevision, "Scene revision", 1, Number.MAX_SAFE_INTEGER),
		expected_draft_revision: requireInteger(input.expected_draft_revision, "Scene draft revision", 0, Number.MAX_SAFE_INTEGER),
    ...requireScene(input),
  }), 200);
}

export function updateRoleplayResponders(
  channelID: string,
  input: ResponderOrderInput,
): Promise<RoleplayComponentResponse> {
  const characterIDs = input.character_ids.map((id) => roleplayID(id, "responder", "rpc"));
  if (characterIDs.length < 1 || characterIDs.length > 16 ||
      new Set(characterIDs).size !== characterIDs.length) {
    throw new Error("Responders must contain 1 to 16 unique characters.");
  }
  return mutate(channelID, "/responders", jsonPut({
    expected_revision: requireInteger(input.expected_revision, "Scene revision", 1, Number.MAX_SAFE_INTEGER),
    character_ids: characterIDs,
  }), 200);
}

export function writeRoleplaySceneDraftParticipant(
  channelID: string,
  characterID: string,
  input: SceneDraftParticipantInput,
): Promise<RoleplayComponentResponse> {
  if (typeof input.selected !== "boolean") throw new Error("Scene draft selection must be an exact boolean.");
  const offset = requireInteger(input.characters_offset, "Character page offset", 0, Number.MAX_SAFE_INTEGER);
  if (offset % 4 !== 0) throw new Error("Character page offset must use the server page size.");
  return mutate(
    channelID,
    `/scene-draft/participants/${roleplayID(characterID, "character", "rpc")}`,
    jsonPut({
      expected_revision: requireInteger(input.expected_revision, "Scene draft revision", 0, Number.MAX_SAFE_INTEGER),
      selected: input.selected,
      characters_offset: offset,
    }),
    200,
  );
}

export function registerRoleplayMeter(
  channelID: string,
  input: MeterDefinitionInput,
): Promise<RoleplayComponentResponse> {
  const minimum = requireInteger(input.minimum, "Meter minimum", -1_000_000, 1_000_000);
  const maximum = requireInteger(input.maximum, "Meter maximum", -1_000_000, 1_000_000);
  const initial = requireInteger(input.initial_value, "Meter initial value", minimum, maximum);
  if (minimum >= maximum) throw new Error("Meter minimum must be below its maximum.");
  return mutate(channelID, "/meters", jsonRequest({
    key: requireKey(input.key, "Meter key"), name: requireText(input.name, "Meter name", 128, true),
    minimum, maximum, initial_value: initial,
  }), 201);
}

export function setRoleplayMeter(
  channelID: string,
  characterID: string,
  meterKey: string,
  input: MeterValueInput,
): Promise<RoleplayComponentResponse> {
  return mutate(channelID, `/meters/${roleplayID(characterID, "character", "rpc")}/${requireKey(meterKey, "Meter key")}`, jsonPut({
    expected_revision: requireInteger(input.expected_revision, "Meter revision", 1, Number.MAX_SAFE_INTEGER),
    value: requireInteger(input.value, "Meter value", -1_000_000, 1_000_000),
  }), 200);
}

export function configureRoleplayResearch(
  channelID: string,
  characterID: string,
  input: ResearchCapabilityInput,
): Promise<RoleplayComponentResponse> {
  if (typeof input.enabled !== "boolean") throw new Error("Research access must be an exact boolean.");
  return mutate(
    channelID,
    `/capabilities/${roleplayID(characterID, "character", "rpc")}/web-research`,
    jsonPut({ enabled: input.enabled }),
    200,
  );
}

export function writeRoleplayGeneration(
  channelID: string,
  characterID: string,
  input: RoleplayGenerationInput,
): Promise<RoleplayComponentResponse> {
  const narrative = requireOllamaModel(input.narrative_model, false);
  return mutate(
    channelID,
    `/generation/${roleplayID(characterID, "character", "rpc")}`,
    jsonPut({
      expected_revision: requireInteger(input.expected_revision, "Character generation revision", 1, Number.MAX_SAFE_INTEGER),
      narrative_model: narrative,
    }),
    200,
  );
}

function requireOllamaModel(value: string, required: boolean): string {
  const model = requireText(value, "Ollama model", 256, required);
  if (model && !/^[A-Za-z0-9._:/@-]+$/.test(model)) throw new Error("Ollama model contains unsupported characters.");
  return model;
}

export function registerRoleplayInteraction(
  channelID: string,
  input: InteractionDefinitionInput,
): Promise<RoleplayComponentResponse> {
  if (input.argument_mode !== "none" && input.argument_mode !== "required") throw new Error("Interaction argument mode is invalid.");
  return mutate(channelID, "/interactions", jsonRequest({
    key: requireKey(input.key, "Interaction key"), name: requireText(input.name, "Interaction name", 128, true),
    description: requireText(input.description, "Interaction description", 512, true),
    argument_mode: input.argument_mode, effects: requireEffects(input.effects),
  }), 201);
}

export function registerRoleplayItem(
  channelID: string,
  input: ItemDefinitionInput,
): Promise<RoleplayComponentResponse> {
  if (input.use_policy !== "finite" && input.use_policy !== "infinite") throw new Error("Item use policy is invalid.");
  const initialUses = requireInteger(input.initial_uses, "Item initial uses", 0, 1000);
  if ((input.use_policy === "finite") !== (initialUses > 0)) throw new Error("Finite items require uses; infinite items require zero uses.");
  const trigger = input.trigger === null ? null : {
    meter_key: requireKey(input.trigger.meter_key, "Item trigger meter"),
    direction: requireDirection(input.trigger.direction),
    threshold: requireInteger(input.trigger.threshold, "Item trigger threshold", -1_000_000, 1_000_000),
  };
  return mutate(channelID, "/items", jsonRequest({
    name: requireText(input.name, "Item name", 256, true),
    description: requireText(input.description, "Item description", 512, true),
    use_policy: input.use_policy, initial_uses: initialUses, trigger,
    priority: requireInteger(input.priority, "Item priority", -1000, 1000), effects: requireEffects(input.effects),
  }), 201);
}

function mutate(channelID: string, suffix: string, init: RequestInit, status: number): Promise<RoleplayComponentResponse> {
  const id = requireID(channelID, "channel", /^[a-z0-9][a-z0-9_.:-]{0,95}$/);
  return requestRoleplayComponent(`/v1/channels/${encodeURIComponent(id)}/roleplay${suffix}`, init, status, id);
}

async function requestRoleplayComponent(
  url: string,
  init: RequestInit | undefined,
  status: number,
  channelID: string,
): Promise<RoleplayComponentResponse> {
  const response = await fetch(url, init);
  const payload = await readJSON<Record<string, unknown>>(response);
  if (response.status !== status) throw new Error(`Roleplay request expected HTTP ${status}, received HTTP ${response.status}.`);
  const result = requireRoleplayComponent(payload);
  if (result.channel_id !== channelID) throw new Error("Roleplay response changed the requested channel identity.");
  return result;
}
