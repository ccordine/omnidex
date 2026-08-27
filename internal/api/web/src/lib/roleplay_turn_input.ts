export type RoleplayTurnPartKind = "action" | "message" | "event";

export interface RoleplayTurnPart {
  kind: RoleplayTurnPartKind;
  text: string;
}

type CharacterContribution = "dialogue" | "action" | "action_dialogue" | "structured_turn";
type NarratorContribution = "narration" | "direction" | "narration_direction";
type RoleplayPersonaKind = "character" | "narrator";

export type RoleplayTurnInput =
  | {
      persona_kind: "character";
      character_id: string;
      contribution_kind: CharacterContribution;
      parts: RoleplayTurnPart[];
    }
  | {
      persona_kind: "narrator";
      contribution_kind: NarratorContribution;
      parts: RoleplayTurnPart[];
    }
  | {
      persona_kind: "narrator";
      contribution_kind: "command";
    };

export interface RoleplayTurnSubmission {
  prompt: string;
  turn: RoleplayTurnInput;
}

const MAX_PARTS = 16;
const MAX_PROMPT_BYTES = 4 * 1024;

export function roleplayTurnSubmission(
  draft: string,
  persona: HTMLSelectElement,
  queuedParts: readonly RoleplayTurnPart[],
): RoleplayTurnSubmission {
  const exactDraft = exactPartText(draft);
  if (exactDraft.startsWith("/")) {
    if (queuedParts.length !== 0) throw new Error("A slash command cannot be mixed with queued story parts.");
    return {
      prompt: exactDraft,
      turn: { persona_kind: "narrator", contribution_kind: "command" },
    };
  }

  const parts = queuedParts.map((part, index) => validatePart(part, index));
  if (exactDraft) parts.push({ kind: "message", text: exactDraft });
  if (parts.length < 1) throw new Error("Add a message, action, or event before sending.");
  if (parts.length > MAX_PARTS) throw new Error(`A roleplay turn can contain at most ${MAX_PARTS} ordered parts.`);

  const prompt = composeRoleplayTurn(parts);
  if (new TextEncoder().encode(prompt).byteLength > MAX_PROMPT_BYTES) {
    throw new Error(`The composed roleplay turn exceeds ${MAX_PROMPT_BYTES} UTF-8 bytes.`);
  }

  const personaKind = selectedPersonaKind(persona);
  if (personaKind === "character") {
    if (!/^rpc_[0-9a-f]{32}$/.test(persona.value)) {
      throw new Error("Selected roleplay identity has no canonical character ID.");
    }
    return {
      prompt,
      turn: {
        persona_kind: "character",
        character_id: persona.value,
      contribution_kind: characterContribution(parts),
        parts,
      },
    };
  }
  return {
    prompt,
    turn: {
      persona_kind: "narrator",
      contribution_kind: narratorContribution(parts),
      parts,
    },
  };
}

export function restoredRoleplayTurnParts(
  prompt: string,
  personaKind: RoleplayPersonaKind,
  contributionKind: string,
  encodedParts: string,
): RoleplayTurnPart[] | null {
  let candidate: unknown;
  try {
    candidate = JSON.parse(encodedParts);
  } catch (error) {
    throw new Error("Failed roleplay turn recovery has invalid persisted parts JSON.", { cause: error });
  }
  if (!Array.isArray(candidate)) {
    throw new Error("Failed roleplay turn recovery requires one persisted ordered parts array.");
  }
  if (contributionKind === "command") {
    if (personaKind !== "narrator" || candidate.length !== 0 || !prompt.startsWith("/") || exactPartText(prompt) !== prompt) {
      throw new Error("Failed roleplay command recovery has contradictory persisted authority.");
    }
    return null;
  }
  if (candidate.length === 0) {
    throw new Error("This historical failed turn has no exact ordered parts, so its original modality cannot be restored safely.");
  }
  if (candidate.length > MAX_PARTS) {
    throw new Error(`Failed roleplay turn recovery exceeds ${MAX_PARTS} persisted parts.`);
  }
  const parts = candidate.map((part, index) => persistedPart(part, index));
  if (composeRoleplayTurn(parts) !== prompt) {
    throw new Error("Failed roleplay turn recovery parts differ from its exact prompt bytes.");
  }
  if (roleplayProseContribution(personaKind, parts) !== contributionKind) {
    throw new Error("Failed roleplay turn recovery has contradictory contribution authority.");
  }
  return parts;
}

function roleplayProseContribution(
  personaKind: RoleplayPersonaKind,
  parts: readonly RoleplayTurnPart[],
): CharacterContribution | NarratorContribution {
  return personaKind === "character" ? characterContribution(parts) : narratorContribution(parts);
}

function characterContribution(parts: readonly RoleplayTurnPart[]): CharacterContribution {
  if (parts.some((part) => part.kind === "event")) return "structured_turn";
  const action = parts.some((part) => part.kind === "action");
  const message = parts.some((part) => part.kind === "message");
  if (action && message) return "action_dialogue";
  return action ? "action" : "dialogue";
}

function narratorContribution(parts: readonly RoleplayTurnPart[]): NarratorContribution {
  const narration = parts.some((part) => part.kind === "action" || part.kind === "event");
  const direction = parts.some((part) => part.kind === "message");
  if (narration && direction) return "narration_direction";
  return narration ? "narration" : "direction";
}

function validatePart(part: RoleplayTurnPart, index: number): RoleplayTurnPart {
  if (part.kind !== "action" && part.kind !== "message" && part.kind !== "event") {
    throw new Error(`Queued roleplay part ${index + 1} has an unsupported type.`);
  }
  const text = exactPartText(part.text);
  if (!text) throw new Error(`Queued ${part.kind} ${index + 1} is blank.`);
  return { kind: part.kind, text };
}

function persistedPart(value: unknown, index: number): RoleplayTurnPart {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`Persisted roleplay part ${index + 1} is not an object.`);
  }
  const keys = Object.keys(value);
  if (keys.length !== 2 || !keys.includes("kind") || !keys.includes("text")) {
    throw new Error(`Persisted roleplay part ${index + 1} has unknown fields.`);
  }
  const part = value as Record<string, unknown>;
  if (typeof part.kind !== "string" || typeof part.text !== "string") {
    throw new Error(`Persisted roleplay part ${index + 1} has invalid field types.`);
  }
  return validatePart({ kind: part.kind as RoleplayTurnPartKind, text: part.text }, index);
}

function composeRoleplayTurn(parts: readonly RoleplayTurnPart[]): string {
  return parts.map((part) => `[${partLabel(part.kind)}]\n${part.text}`).join("\n\n");
}

function exactPartText(value: string): string {
  if (value.includes("\0")) throw new Error("Roleplay input contains NUL.");
  return value.trim();
}

function partLabel(kind: RoleplayTurnPartKind): string {
  return kind.charAt(0).toUpperCase() + kind.slice(1);
}

function selectedPersonaKind(persona: HTMLSelectElement): "character" | "narrator" {
  const option = persona.selectedOptions[0];
  const kind = option?.dataset.personaKind;
  if (kind !== "character" && kind !== "narrator") {
    throw new Error("Acting-as selection is missing exact server authority.");
  }
  if (kind === "narrator" && persona.value !== "narrator") {
    throw new Error("Narrator selection has contradictory identity authority.");
  }
  return kind;
}
