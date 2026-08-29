export type RoleplayWorkspaceDialog = "worlds" | "characters" | "setup" | "character-editor";
export type RoleplaySetupSection = "scene" | "cast" | "state" | "actions";

const setupSections: readonly RoleplaySetupSection[] = ["scene", "cast", "state", "actions"];

export interface RoleplayWorkspaceDialogHost {
  worldDialog(): HTMLDialogElement;
  characterDialog(): HTMLDialogElement;
  setupDialog(): HTMLDialogElement;
  characterEditorDialog(): HTMLDialogElement;
}

export class RoleplayWorkspaceDialogs {
  private activeSetupSection: RoleplaySetupSection | null = null;

  constructor(private readonly host: RoleplayWorkspaceDialogHost) {}

  open(kind: RoleplayWorkspaceDialog): void {
    const dialog = this.dialog(kind);
    if (kind === "setup") this.restoreSetupSection();
    for (const other of this.dialogs()) {
      if (other !== dialog && other.open) other.close();
    }
    if (dialog.open) return;
    if (typeof dialog.showModal !== "function") {
      throw new Error("Roleplay workspace dialogs require native modal support.");
    }
    dialog.showModal();
  }

  openSetup(section?: RoleplaySetupSection): void {
    if (section !== undefined) this.activeSetupSection = this.requireSetupSection(section);
    this.open("setup");
  }

  close(kind: RoleplayWorkspaceDialog): void {
    const dialog = this.dialog(kind);
    if (dialog.open) dialog.close();
  }

  closeFromBackdrop(kind: RoleplayWorkspaceDialog, event: Event): void {
    const dialog = this.dialog(kind);
    if (event.currentTarget !== dialog) {
      throw new Error("Roleplay dialog backdrop event lacks exact dialog authority.");
    }
    if (event.target === dialog) this.close(kind);
  }

  selectSetupSection(event: Event): void {
    const target = event.currentTarget;
    if (!(target instanceof HTMLButtonElement)) {
      throw new Error("Roleplay setup selection requires a server-rendered button.");
    }
    const section = this.requireSetupSection(target.dataset.roleplaySetupTab ?? "");
    this.activeSetupSection = section;
    this.applySetupSection(section, true);
  }

  restoreSetupSection(): void {
    const navigation = this.setupNavigation();
    const retained = this.activeSetupSection;
    const selected = retained && navigation.tabs.has(retained)
      ? retained
      : this.serverSelectedSection(navigation.tabs);
    this.activeSetupSection = selected;
    this.applySetupSection(selected, false, navigation);
  }

  private applySetupSection(
    section: RoleplaySetupSection,
    focus: boolean,
    navigation = this.setupNavigation(),
  ): void {
    const selectedTab = navigation.tabs.get(section);
    if (!selectedTab || !navigation.panels.has(section)) {
      throw new Error(`Roleplay setup section ${JSON.stringify(section)} is unavailable.`);
    }
    for (const [key, tab] of navigation.tabs) {
      const selected = key === section;
      tab.setAttribute("aria-selected", String(selected));
      tab.tabIndex = selected ? 0 : -1;
      navigation.panels.get(key)!.hidden = !selected;
    }
    if (focus) selectedTab.focus();
  }

  private setupNavigation(): {
    tabs: Map<RoleplaySetupSection, HTMLButtonElement>;
    panels: Map<RoleplaySetupSection, HTMLElement>;
  } {
    const dialog = this.dialog("setup");
    const tabs = this.setupElements<HTMLButtonElement>(dialog, "tab");
    const panels = this.setupElements<HTMLElement>(dialog, "panel");
    if (tabs.size === 0 || tabs.size !== panels.size) {
      throw new Error("Roleplay setup navigation is incomplete.");
    }
    for (const [section, tab] of tabs) {
      const panel = panels.get(section);
      if (!panel || !panel.id || tab.getAttribute("aria-controls") !== panel.id) {
        throw new Error(`Roleplay setup navigation for ${JSON.stringify(section)} is inconsistent.`);
      }
    }
    return { tabs, panels };
  }

  private setupElements<T extends HTMLElement>(
    dialog: HTMLDialogElement,
    kind: "tab" | "panel",
  ): Map<RoleplaySetupSection, T> {
    const attribute = `data-roleplay-setup-${kind}`;
    const result = new Map<RoleplaySetupSection, T>();
    for (const element of dialog.querySelectorAll<T>(`[${attribute}]`)) {
      const section = this.requireSetupSection(element.getAttribute(attribute) ?? "");
      if (result.has(section)) throw new Error(`Roleplay setup duplicates section ${JSON.stringify(section)}.`);
      result.set(section, element);
    }
    return result;
  }

  private serverSelectedSection(
    tabs: Map<RoleplaySetupSection, HTMLButtonElement>,
  ): RoleplaySetupSection {
    const selected = [...tabs].filter(([, tab]) => tab.getAttribute("aria-selected") === "true");
    if (selected.length !== 1) throw new Error("Roleplay setup must have exactly one server-selected section.");
    return selected[0][0];
  }

  private requireSetupSection(value: string): RoleplaySetupSection {
    if (!setupSections.includes(value as RoleplaySetupSection)) {
      throw new Error(`Unregistered roleplay setup section ${JSON.stringify(value)}.`);
    }
    return value as RoleplaySetupSection;
  }

  private dialogs(): HTMLDialogElement[] {
    return [
      this.dialog("worlds"),
      this.dialog("characters"),
      this.dialog("setup"),
      this.dialog("character-editor"),
    ];
  }

  private dialog(kind: RoleplayWorkspaceDialog): HTMLDialogElement {
    const dialog = kind === "worlds"
      ? this.host.worldDialog()
      : kind === "characters"
        ? this.host.characterDialog()
        : kind === "setup"
          ? this.host.setupDialog()
          : this.host.characterEditorDialog();
    if (dialog.dataset.roleplayDialog !== kind) {
      throw new Error(`Roleplay ${kind} dialog authority does not match its target.`);
    }
    return dialog;
  }
}
