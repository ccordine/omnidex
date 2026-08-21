import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  RoleplayWorkspaceDialogs,
  type RoleplayWorkspaceDialog,
} from "./roleplay_workspace_dialogs";

function dialog(kind: RoleplayWorkspaceDialog): HTMLDialogElement {
  const element = document.createElement("dialog");
  element.dataset.roleplayDialog = kind;
  element.showModal = vi.fn(() => { element.open = true; });
  element.close = vi.fn(() => { element.open = false; });
  document.body.append(element);
  return element;
}

function setupFlow(element: HTMLDialogElement, active: "scene" | "cast" = "scene"): void {
  element.innerHTML = ["scene", "cast"].map((section) => `
    <button
      data-roleplay-setup-tab="${section}"
      aria-selected="${String(section === active)}"
      aria-controls="roleplay-setup-panel-${section}"
    >${section}</button>
    <section
      id="roleplay-setup-panel-${section}"
      data-roleplay-setup-panel="${section}"
      ${section === active ? "" : "hidden"}
    ></section>
  `).join("");
}

describe("RoleplayWorkspaceDialogs", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
  });

  it("opens only one typed workspace dialog and closes only from its backdrop", () => {
    const worlds = dialog("worlds");
    const characters = dialog("characters");
    const setup = dialog("setup");
    const editor = dialog("character-editor");
    setupFlow(setup);
    const coordinator = new RoleplayWorkspaceDialogs({
      worldDialog: () => worlds,
      characterDialog: () => characters,
      setupDialog: () => setup,
      characterEditorDialog: () => editor,
    });

    coordinator.open("worlds");
    expect(worlds.showModal).toHaveBeenCalledOnce();
    expect(worlds.open).toBe(true);

    coordinator.open("characters");
    expect(worlds.close).toHaveBeenCalledOnce();
    expect(characters.showModal).toHaveBeenCalledOnce();
    expect(characters.open).toBe(true);

    const interior = document.createElement("div");
    characters.append(interior);
    coordinator.closeFromBackdrop("characters", {
      target: interior,
      currentTarget: characters,
    } as unknown as Event);
    expect(characters.open).toBe(true);

    coordinator.closeFromBackdrop("characters", {
      target: characters,
      currentTarget: characters,
    } as unknown as Event);
    expect(characters.close).toHaveBeenCalledOnce();
    expect(characters.open).toBe(false);

    coordinator.open("characters");
    coordinator.open("character-editor");
    expect(characters.close).toHaveBeenCalledTimes(2);
    expect(editor.showModal).toHaveBeenCalledOnce();
    expect(editor.open).toBe(true);
    coordinator.open("worlds");
    expect(editor.close).toHaveBeenCalledOnce();
  });

  it("fails loudly when dialog authority does not match the requested collection", () => {
    const worlds = dialog("characters");
    const characters = dialog("characters");
    const setup = dialog("setup");
    const editor = dialog("character-editor");
    const coordinator = new RoleplayWorkspaceDialogs({
      worldDialog: () => worlds,
      characterDialog: () => characters,
      setupDialog: () => setup,
      characterEditorDialog: () => editor,
    });

    expect(() => coordinator.open("worlds")).toThrow(/dialog authority/);
    expect(worlds.showModal).not.toHaveBeenCalled();
  });

  it("organizes world setup into one selected section and preserves it after server replacement", () => {
    const worlds = dialog("worlds");
    const characters = dialog("characters");
    const setup = dialog("setup");
    const editor = dialog("character-editor");
    setupFlow(setup);
    const coordinator = new RoleplayWorkspaceDialogs({
      worldDialog: () => worlds,
      characterDialog: () => characters,
      setupDialog: () => setup,
      characterEditorDialog: () => editor,
    });

    coordinator.openSetup("cast");
    expect(setup.open).toBe(true);
    expect(setup.querySelector('[data-roleplay-setup-tab="cast"]')?.getAttribute("aria-selected")).toBe("true");
    expect((setup.querySelector('[data-roleplay-setup-panel="scene"]') as HTMLElement).hidden).toBe(true);
    expect((setup.querySelector('[data-roleplay-setup-panel="cast"]') as HTMLElement).hidden).toBe(false);

    setupFlow(setup, "scene");
    coordinator.restoreSetupSection();
    expect(setup.querySelector('[data-roleplay-setup-tab="cast"]')?.getAttribute("aria-selected")).toBe("true");
    expect((setup.querySelector('[data-roleplay-setup-panel="cast"]') as HTMLElement).hidden).toBe(false);
  });

  it("fails loudly when server-rendered setup navigation is incomplete", () => {
    const worlds = dialog("worlds");
    const characters = dialog("characters");
    const setup = dialog("setup");
    const editor = dialog("character-editor");
    setup.innerHTML = '<button data-roleplay-setup-tab="scene" aria-selected="true"></button>';
    const coordinator = new RoleplayWorkspaceDialogs({
      worldDialog: () => worlds,
      characterDialog: () => characters,
      setupDialog: () => setup,
      characterEditorDialog: () => editor,
    });

    expect(() => coordinator.openSetup()).toThrow(/setup navigation/);
    expect(setup.showModal).not.toHaveBeenCalled();
  });
});
