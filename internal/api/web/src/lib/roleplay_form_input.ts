import type {
  InteractionDefinitionInput,
  ItemDefinitionInput,
  MeterDefinitionInput,
  MeterDeltaInput,
  MeterValueInput,
  PersonaInput,
  ResearchCapabilityInput,
	RoleplayGenerationInput,
  SceneInput,
	SceneCreateInput,
	SceneDraftParticipantInput,
	SceneUpdateInput,
} from "./roleplay_api";

export function personaInput(form: HTMLFormElement): PersonaInput {
  return {
    expected_revision: integerField(form, "expected_revision"),
    summary: exactField(form, "summary"),
    voice: exactField(form, "voice", false),
    traits: lineList(form, "traits", false),
    goals: lineList(form, "goals", false),
  };
}

export function sceneInput(form: HTMLFormElement): SceneInput {
  return {
    title: exactField(form, "title"),
    description: exactField(form, "description"),
    participant_ids: orderedSceneParticipants(form),
  };
}

export function sceneCreateInput(form: HTMLFormElement): SceneCreateInput {
	return {
		...sceneInput(form),
		expected_draft_revision: integerField(form, "expected_draft_revision"),
	};
}

export function sceneUpdateInput(form: HTMLFormElement): SceneUpdateInput {
	return {
		...sceneInput(form),
		expected_draft_revision: integerField(form, "expected_draft_revision"),
	};
}

export function sceneDraftParticipantInput(form: HTMLFormElement): SceneDraftParticipantInput {
	const control = form.elements.namedItem("selected");
	if (!(control instanceof HTMLInputElement) || control.type !== "checkbox") {
		throw new Error("The server-rendered scene draft selection is missing.");
	}
	return {
		expected_revision: requiredDatasetInteger(form, "draftRevision"),
		selected: control.checked,
		characters_offset: requiredDatasetInteger(form, "charactersOffset"),
	};
}

export function meterDefinitionInput(form: HTMLFormElement): MeterDefinitionInput {
  return {
    key: exactField(form, "key"),
    name: exactField(form, "name"),
    minimum: integerField(form, "minimum"),
    maximum: integerField(form, "maximum"),
    initial_value: integerField(form, "initial_value"),
  };
}

export function meterValueInput(form: HTMLFormElement): MeterValueInput {
  return {
    expected_revision: requiredDatasetInteger(form, "meterRevision"),
    value: integerField(form, "value"),
  };
}

export function researchCapabilityInput(form: HTMLFormElement): ResearchCapabilityInput {
  const control = form.elements.namedItem("enabled");
  if (!(control instanceof HTMLInputElement) || control.type !== "checkbox") {
    throw new Error("The server-rendered research access checkbox is missing.");
  }
  return {
    enabled: control.checked,
  };
}

export function roleplayGenerationInput(form: HTMLFormElement): RoleplayGenerationInput {
	return {
		expected_revision: integerField(form, "expected_revision"),
		narrative_model: exactField(form, "narrative_model", false),
	};
}

export function interactionDefinitionInput(form: HTMLFormElement): InteractionDefinitionInput {
  const mode = exactField(form, "argument_mode");
  if (mode !== "none" && mode !== "required") throw new Error("Interaction argument mode is invalid.");
  return {
    key: exactField(form, "key"),
    name: exactField(form, "name"),
    description: exactField(form, "description"),
    argument_mode: mode,
    effects: effectLines(form),
  };
}

export function itemDefinitionInput(form: HTMLFormElement): ItemDefinitionInput {
  const policy = exactField(form, "use_policy");
  if (policy !== "finite" && policy !== "infinite") throw new Error("Item use policy is invalid.");
  const triggerMeter = exactField(form, "trigger_meter_key", false);
  const triggerDirection = exactField(form, "trigger_direction", false);
  const triggerThreshold = exactField(form, "trigger_threshold", false);
  const hasTrigger = triggerMeter !== "" || triggerDirection !== "" || triggerThreshold !== "";
  let trigger: ItemDefinitionInput["trigger"] = null;
  if (hasTrigger) {
    if (!triggerMeter || !triggerThreshold || (triggerDirection !== "at_or_below" && triggerDirection !== "at_or_above")) {
      throw new Error("Item trigger requires its meter, direction, and threshold together.");
    }
    trigger = {
      meter_key: triggerMeter,
      direction: triggerDirection,
      threshold: canonicalInteger(triggerThreshold, "trigger_threshold"),
    };
  }
  return {
    name: exactField(form, "name"),
    description: exactField(form, "description"),
    use_policy: policy,
    initial_uses: integerField(form, "initial_uses"),
    trigger,
    priority: integerField(form, "priority"),
    effects: effectLines(form),
  };
}

export function requiredDataset(form: HTMLFormElement, key: string): string {
  const value = form.dataset[key];
  if (typeof value !== "string" || !value || value !== value.trim()) {
    throw new Error(`Server form data ${key} is missing or inexact.`);
  }
  return value;
}

export function requiredDatasetInteger(form: HTMLFormElement, key: string): number {
  return canonicalInteger(requiredDataset(form, key), key);
}

function effectLines(form: HTMLFormElement): MeterDeltaInput[] {
  return lineList(form, "effects", true).map((line, index) => {
    const parts = line.split(":");
    if (parts.length !== 2 || !parts[0] || !parts[1]) throw new Error(`Effect line ${index + 1} must be exactly key:delta.`);
    return { meter_key: parts[0], delta: canonicalInteger(parts[1], `effect line ${index + 1}`) };
  });
}

function lineList(form: HTMLFormElement, name: string, required: boolean): string[] {
  const raw = exactField(form, name, required);
  if (!raw) return [];
  const values = raw.split("\n");
  if (values.some((value) => !value || value !== value.trim() || value.includes("\r"))) {
    throw new Error(`${name} must contain one exact value per line.`);
  }
  return values;
}

function integerField(form: HTMLFormElement, name: string): number {
  return canonicalInteger(exactField(form, name), name);
}

function orderedSceneParticipants(form: HTMLFormElement): string[] {
	const values = new FormData(form).getAll("participant_id");
	if (values.length < 1 || values.length > 16 || values.some((value) => typeof value !== "string" || !/^rpc_[0-9a-f]{32}$/.test(value))) {
		throw new Error("The server-rendered scene draft must contain 1 to 16 exact character identities.");
	}
	const participants = values as string[];
	if (new Set(participants).size !== participants.length) {
		throw new Error("The server-rendered scene draft contains a duplicate participant.");
	}
	return participants;
}

function canonicalInteger(raw: string, source: string): number {
  if (!/^-?(0|[1-9][0-9]*)$/.test(raw)) throw new Error(`${source} must be one canonical integer.`);
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || String(value) !== raw) throw new Error(`${source} is outside the safe integer range.`);
  return value;
}

function exactField(form: HTMLFormElement, name: string, required = true): string {
  const value = new FormData(form).get(name);
  if (typeof value !== "string" || value.includes("\0") || value !== value.trim() || (required && !value)) {
    throw new Error(`${name} must contain exact${required ? " nonblank" : ""} text.`);
  }
  return value;
}
