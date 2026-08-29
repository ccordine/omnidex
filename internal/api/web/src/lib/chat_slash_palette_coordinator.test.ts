import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchSlashCommandComponent, type SlashCommandComponent } from "./chat_slash_palette_api";
import { ChatSlashPaletteCoordinator, type ChatSlashPaletteHost } from "./chat_slash_palette_coordinator";

vi.mock("./chat_slash_palette_api", () => ({ fetchSlashCommandComponent: vi.fn() }));

const channelID = "story-42";

type CommandFixture = {
  exact: string;
  cursor: number;
  prefix: string;
  key: string;
  kind: "interaction" | "give" | "take" | "research";
};

const commands: CommandFixture[] = [
  { exact: '/feed ""', cursor: 7, prefix: "/feed", key: "feed", kind: "interaction" },
  { exact: '/take "🍔"', cursor: 10, prefix: "/take", key: "take", kind: "take" },
  { exact: '/research ""', cursor: 11, prefix: "/research", key: "research", kind: "research" },
];

function component(items = commands, marker = "current", id = channelID): SlashCommandComponent {
  const options = items.map((command, order) => `<button id="slash-command-option-${marker}-${order}" type="button" role="option" aria-selected="false" tabindex="-1" data-chat-slash-option data-action="chat#chooseSlashCommand" data-slash-command="${escapeAttribute(command.exact)}" data-slash-command-cursor="${command.cursor}" data-slash-command-prefix="${command.prefix}" data-slash-command-kind="${command.kind}" data-slash-command-key="${command.key}" data-slash-command-order="${order}">${command.prefix}</button>`).join("");
  const empty = items.length === 0 ? "" : " hidden";
  return {
    channel_id: id,
    command_count: items.length,
    html: { bundle: `<template data-recyclr-target="slash-command-options"><div id="slash-command-list" role="listbox" data-slash-command-list data-slash-command-channel-id="${id}">${options}<p data-slash-command-no-match role="status"${empty}>No available slash commands match this prefix.</p></div></template>` },
  };
}

function createFixture() {
  const input = document.createElement("textarea");
  input.setAttribute("aria-expanded", "false");
  const palette = document.createElement("div");
  palette.classList.add("hidden");
  const options = document.createElement("div");
  palette.append(options);
  document.body.replaceChildren(input, palette);
  const host: ChatSlashPaletteHost = {
    input: () => input,
    palette: () => palette,
    options: () => options,
    renderComponentBundle: vi.fn(async (bundle: string) => {
      const template = document.createElement("template");
      template.innerHTML = bundle;
      const serverTemplate = template.content.querySelector("template");
      if (!(serverTemplate instanceof HTMLTemplateElement)) throw new Error("test bundle lacks template");
      options.replaceChildren(serverTemplate.content.cloneNode(true));
    }),
  };
  return { input, palette, options, host, coordinator: new ChatSlashPaletteCoordinator(host) };
}

function type(input: HTMLTextAreaElement, value: string): void {
  input.value = value;
  input.setSelectionRange(value.length, value.length);
}

function optionElements(options: HTMLElement): HTMLButtonElement[] {
  return [...options.querySelectorAll<HTMLButtonElement>("[data-chat-slash-option]")];
}

describe("ChatSlashPaletteCoordinator", () => {
  beforeEach(() => vi.resetAllMocks());

  it("shows all server options for slash, filters prefixes, and toggles the server no-match row", async () => {
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component());
    const fixture = createFixture();
    await fixture.coordinator.activate(channelID);

    type(fixture.input, "/");
    fixture.coordinator.inputChanged();
    expect(fixture.palette).not.toHaveClass("hidden");
    expect(optionElements(fixture.options).every((option) => !option.hidden)).toBe(true);
    expect(fixture.input).toHaveAttribute("aria-expanded", "true");

    type(fixture.input, "/ta");
    fixture.coordinator.inputChanged();
    expect(optionElements(fixture.options).map((option) => option.hidden)).toEqual([true, false, true]);

    type(fixture.input, "/zzz");
    fixture.coordinator.inputChanged();
    const noMatch = fixture.options.querySelector<HTMLElement>("[data-slash-command-no-match]")!;
    expect(fixture.palette).not.toHaveClass("hidden");
    expect(noMatch.hidden).toBe(false);
    expect(noMatch).toHaveAttribute("aria-hidden", "false");

    type(fixture.input, "/");
    fixture.coordinator.inputChanged();
    expect(noMatch.hidden).toBe(true);
    expect(noMatch).toHaveAttribute("aria-hidden", "true");
  });

  it("navigates with arrows, inserts on Tab, and closes on Escape", async () => {
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component());
    const fixture = createFixture();
    await fixture.coordinator.activate(channelID);
    type(fixture.input, "/");
    fixture.coordinator.inputChanged();

    const down = new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true });
    fixture.coordinator.keydown(down);
    expect(down.defaultPrevented).toBe(true);
    expect(fixture.input.getAttribute("aria-activedescendant")).toContain("-1");

    const tab = new KeyboardEvent("keydown", { key: "Tab", cancelable: true });
    fixture.coordinator.keydown(tab);
    expect(tab.defaultPrevented).toBe(true);
    expect(fixture.input.value).toBe('/take "🍔"');
    expect(fixture.input.selectionStart).toBe(10);
    expect(fixture.palette).toHaveClass("hidden");

    type(fixture.input, "/");
    fixture.coordinator.inputChanged();
    const escape = new KeyboardEvent("keydown", { key: "Escape", cancelable: true });
    fixture.coordinator.keydown(escape);
    expect(escape.defaultPrevented).toBe(true);
    expect(fixture.palette).toHaveClass("hidden");
  });

  it("inserts the exact server command and UTF-16 cursor on click", async () => {
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component());
    const fixture = createFixture();
    await fixture.coordinator.activate(channelID);
    type(fixture.input, "/ta");
    fixture.coordinator.inputChanged();
    const take = optionElements(fixture.options)[1];

    fixture.coordinator.choose({ currentTarget: take, preventDefault: vi.fn() } as unknown as Event);

    expect(fixture.input.value).toBe('/take "🍔"');
    expect(fixture.input.selectionStart).toBe(10);
    expect(fixture.input.selectionEnd).toBe(10);
  });

  it("does not consume modified Enter or alter the current composer bytes", async () => {
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component());
    const fixture = createFixture();
    await fixture.coordinator.activate(channelID);
    type(fixture.input, "/ta");
    fixture.coordinator.inputChanged();
    const exact = fixture.input.value;
    const submit = new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true, cancelable: true });

    fixture.coordinator.keydown(submit);

    expect(submit.defaultPrevented).toBe(false);
    expect(fixture.input.value).toBe(exact);
  });

  it("ignores composition Enter and Escape without selecting or dismissing", async () => {
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component());
    const fixture = createFixture();
    await fixture.coordinator.activate(channelID);
    type(fixture.input, "/ta");
    fixture.coordinator.inputChanged();
    const exact = fixture.input.value;

    for (const key of ["Enter", "Escape"]) {
      const event = new KeyboardEvent("keydown", { key, isComposing: true, cancelable: true });
      fixture.coordinator.keydown(event);
      expect(event.defaultPrevented).toBe(false);
      expect(fixture.input.value).toBe(exact);
      expect(fixture.palette).not.toHaveClass("hidden");
    }
  });

  it("shows the explicit server empty state for an assistant channel", async () => {
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component([], "assistant", "assistant-42"));
    const fixture = createFixture();
    await fixture.coordinator.activate("assistant-42");
    type(fixture.input, "/");
    fixture.coordinator.inputChanged();

    expect(fixture.palette).not.toHaveClass("hidden");
    expect(fixture.options.querySelector("[data-slash-command-no-match]")).not.toHaveAttribute("hidden");
    expect(optionElements(fixture.options)).toHaveLength(0);
  });

  it("generation-gates reversed responses for the same channel", async () => {
    let resolveOlder!: (value: SlashCommandComponent) => void;
    let resolveNewer!: (value: SlashCommandComponent) => void;
    vi.mocked(fetchSlashCommandComponent)
      .mockReturnValueOnce(new Promise((resolve) => { resolveOlder = resolve; }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveNewer = resolve; }));
    const fixture = createFixture();
    const older = fixture.coordinator.activate(channelID);
    const newer = fixture.coordinator.refresh();
    resolveNewer(component(commands, "newer"));
    await newer;
    resolveOlder(component(commands, "older"));
    await older;

    expect(fixture.options.querySelector("#slash-command-option-newer-0")).not.toBeNull();
    expect(fixture.options.querySelector("#slash-command-option-older-0")).toBeNull();
  });

  it("keeps same-channel stale options unusable during and after a failed refresh", async () => {
    let rejectRefresh!: (error: Error) => void;
    vi.mocked(fetchSlashCommandComponent)
      .mockResolvedValueOnce(component(commands, "old"))
      .mockReturnValueOnce(new Promise((_resolve, reject) => { rejectRefresh = reject; }));
    const fixture = createFixture();
    await fixture.coordinator.activate(channelID);
    type(fixture.input, "/");
    fixture.coordinator.inputChanged();
    expect(fixture.palette).not.toHaveClass("hidden");

    const refresh = fixture.coordinator.refresh();
    fixture.coordinator.inputChanged();
    expect(fixture.palette).toHaveClass("hidden");

    rejectRefresh(new Error("projection unavailable"));
    await expect(refresh).rejects.toThrow("projection unavailable");
    fixture.coordinator.inputChanged();
    expect(fixture.palette).toHaveClass("hidden");
  });

  it("rejects a server cursor that splits a UTF-16 surrogate pair", async () => {
    const invalid = [{ exact: '/take "🍔"', cursor: 8, prefix: "/take", key: "take", kind: "take" as const }];
    vi.mocked(fetchSlashCommandComponent).mockResolvedValueOnce(component(invalid));
    const fixture = createFixture();

    await expect(fixture.coordinator.activate(channelID)).rejects.toThrow("invalid server-rendered option");
  });
});

function escapeAttribute(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;");
}
