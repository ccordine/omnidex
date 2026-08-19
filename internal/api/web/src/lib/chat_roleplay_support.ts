import type { RoleplayPageState } from "./roleplay_api";
import type { StatusTone } from "./types";

export interface ChatRoleplayHost {
  hasPanel(): boolean;
  panel(): HTMLElement;
  hasLoading(): boolean;
  loading(): HTMLElement;
  renderComponentBundle(bundle: string): Promise<void>;
  setComposerAvailable(available: boolean): void;
  setComposerText(value: string): void;
  focusComposer(): void;
  setStatus(text: string, tone: StatusTone): void;
  addEvent(type: string, details?: Record<string, unknown>): void;
  reportError(error: unknown): void;
  refreshSlashCommands(): Promise<void>;
}

export function pageFromRoleplayButton(button: HTMLButtonElement): RoleplayPageState {
  const section = button.dataset.roleplayPageSection;
  if (!section) throw new Error("The server-rendered roleplay page section is missing.");
  return {
    characters: datasetOffset(button, "charactersOffset"),
    personas: datasetOffset(button, "personasOffset"),
    turn_order: datasetOffset(button, "turnOrderOffset"),
    meters: datasetOffset(button, "metersOffset"),
    inventory: datasetOffset(button, "inventoryOffset"),
    interactions: datasetOffset(button, "interactionsOffset"),
    item_templates: datasetOffset(button, "itemTemplatesOffset"),
  };
}

function isRoleplayDisableable(
  control: Element,
): control is HTMLButtonElement | HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement {
  return control instanceof HTMLButtonElement || control instanceof HTMLInputElement ||
    control instanceof HTMLSelectElement || control instanceof HTMLTextAreaElement;
}

export function roleplayErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function withRoleplayFormFeedback(
  form: HTMLFormElement,
  operation: () => Promise<void>,
): Promise<void> {
  const controls = [...form.elements].filter(isRoleplayDisableable);
  const prior = controls.map((control) => control.disabled);
  controls.forEach((control) => { control.disabled = true; });
  form.setAttribute("aria-busy", "true");
  try {
    await operation();
  } finally {
    controls.forEach((control, index) => { control.disabled = prior[index] ?? false; });
    form.setAttribute("aria-busy", "false");
  }
}

export async function withRoleplayControlFeedback(
  control: HTMLButtonElement,
  operation: () => Promise<void>,
  onError: (error: unknown) => void,
): Promise<void> {
  const prior = control.disabled;
  control.disabled = true;
  control.setAttribute("aria-busy", "true");
  try {
    await operation();
  } catch (error) {
    onError(error);
  } finally {
    control.disabled = prior;
    control.setAttribute("aria-busy", "false");
  }
}

function datasetOffset(button: HTMLButtonElement, key: string): number {
  const raw = button.dataset[key];
  if (typeof raw !== "string" || !/^(0|[1-9][0-9]*)$/.test(raw)) {
    throw new Error(`Roleplay page cursor ${key} is invalid.`);
  }
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || value % 4 !== 0) {
    throw new Error(`Roleplay page cursor ${key} is invalid.`);
  }
  return value;
}
