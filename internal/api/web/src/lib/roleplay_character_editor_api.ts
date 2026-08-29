import { readJSON } from "./api";
import { requireID, roleplayID } from "./roleplay_api_validation";
import { requireServerBundle } from "./server_component_api";

export interface RoleplayCharacterEditorResponse {
  channel_id: string;
  world_id: string;
  character_id: string;
  html: { bundle: string };
}

export async function fetchRoleplayCharacterEditor(
  channelID: string,
  characterID: string,
): Promise<RoleplayCharacterEditorResponse> {
  const channel = requireID(channelID, "channel", /^[a-z0-9][a-z0-9_.:-]{0,95}$/);
  const character = roleplayID(characterID, "character", "rpc");
  const query = new URLSearchParams({ channel_id: channel, character_id: character });
  const response = await fetch(`/v1/ui/roleplay/character?${query}`);
  const payload = await readJSON<Record<string, unknown>>(response);
  if (response.status !== 200) {
    throw new Error(`Roleplay character editor expected HTTP 200, received HTTP ${response.status}.`);
  }
  const responseChannel = requireID(payload.channel_id, "response channel", /^[a-z0-9][a-z0-9_.:-]{0,95}$/);
  const worldID = roleplayID(payload.world_id, "response world", "rpw");
  const responseCharacter = roleplayID(payload.character_id, "response character", "rpc");
  if (responseChannel !== channel || responseCharacter !== character) {
    throw new Error("Roleplay character editor response changed its requested authority.");
  }
  return {
    channel_id: responseChannel,
    world_id: worldID,
    character_id: responseCharacter,
    html: { bundle: requireServerBundle(payload, "Roleplay character editor") },
  };
}
