import { withRoleplayControlFeedback, withRoleplayFormFeedback } from "./chat_roleplay_support";
import { placeRoleplayLibraryCharacter } from "./roleplay_api";
import {
  createRoleplayLibraryCharacter,
  fetchRoleplayLibraryPage,
  fetchRoleplayWorldsPage,
} from "./roleplay_workspace_api";
import type { StatusTone } from "./types";

export interface RoleplayWorkspaceHost {
  hasLoading(): boolean;
  loading(): HTMLElement;
  renderComponentBundle(bundle: string): Promise<void>;
  selectedChannelID(): string;
  firstChannelID(): string;
  selectChannelID(id: string): Promise<void>;
  createWorld(): Promise<boolean>;
  refreshRoleplay(): Promise<void>;
  setStatus(text: string, tone: StatusTone): void;
  addEvent(type: string, details?: Record<string, unknown>): void;
  reportError(error: unknown): void;
}

export class RoleplayWorkspaceCoordinator {
  private pending = 0;

  constructor(private readonly host: RoleplayWorkspaceHost) {}

  async activate(): Promise<void> {
    await this.withLoading(async () => {
      await this.loadWorlds(0);
      if (!this.host.selectedChannelID()) {
        const first = this.host.firstChannelID();
        if (first) await this.host.selectChannelID(first);
      }
      await this.loadCharacters(0);
    });
  }

  async refresh(): Promise<void> {
    await Promise.all([this.loadWorlds(0), this.loadCharacters(0)]);
  }

  async selectWorld(event: Event): Promise<boolean> {
    const button = event.currentTarget as HTMLButtonElement;
    const channelID = button.dataset.channelId;
    if (!channelID || !/^[a-z0-9][a-z0-9_.:-]{0,95}$/.test(channelID)) {
      throw new Error("World browser selection lacks a canonical channel identity.");
    }
    let selected = false;
    await withRoleplayControlFeedback(button, async () => {
      await this.host.selectChannelID(channelID);
      await this.loadCharacters(0);
      this.host.setStatus("world ready", "ready");
      this.host.addEvent("roleplay_world_selected", { channel_id: channelID });
      selected = true;
    }, (error) => this.report(error));
    return selected;
  }

  async createWorld(event: Event): Promise<boolean> {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    let created = false;
    try {
      await withRoleplayFormFeedback(form, async () => {
        if (!await this.host.createWorld()) return;
        form.reset();
        await this.refresh();
        this.host.setStatus("world created", "ready");
        this.host.addEvent("roleplay_world_created", { channel_id: this.host.selectedChannelID() });
        created = true;
      });
    } catch (error) {
      this.report(error);
    }
    return created;
  }

  async createCharacter(event: Event): Promise<void> {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    try {
      await withRoleplayFormFeedback(form, async () => {
        const input = form.elements.namedItem("name");
        if (!(input instanceof HTMLInputElement)) throw new Error("Character library form is missing its name field.");
        const page = await createRoleplayLibraryCharacter(input.value, this.host.selectedChannelID());
        await this.host.renderComponentBundle(page.html.bundle);
        input.value = "";
        this.host.setStatus("character created", "ready");
        this.host.addEvent("roleplay_library_character_created", {});
      });
    } catch (error) {
      this.report(error);
    }
  }

  async placeCharacter(event: Event): Promise<boolean> {
    const button = event.currentTarget as HTMLButtonElement;
    const libraryID = button.dataset.libraryCharacterId;
    if (!libraryID || !/^rpl_[0-9a-f]{32}$/.test(libraryID)) {
      throw new Error("Character placement lacks a canonical library identity.");
    }
    const channelID = this.host.selectedChannelID();
    if (!channelID) throw new Error("Select a world before adding a character.");
    let placed = false;
    await withRoleplayControlFeedback(button, async () => {
      const component = await placeRoleplayLibraryCharacter(channelID, libraryID);
      await this.host.renderComponentBundle(component.html.bundle);
      await this.refresh();
      await this.host.refreshRoleplay();
      this.host.setStatus("character added", "ready");
      this.host.addEvent("roleplay_character_placed", {
        channel_id: channelID,
        library_character_id: libraryID,
      });
      placed = true;
    }, (error) => this.report(error));
    return placed;
  }

  async loadMoreWorlds(event: Event): Promise<void> {
    await this.loadMore(event, "worlds", (offset) => this.loadWorlds(offset));
  }

  async loadMoreCharacters(event: Event): Promise<void> {
    await this.loadMore(event, "characters", (offset) => this.loadCharacters(offset));
  }

  private async loadMore(event: Event, section: string, load: (offset: number) => Promise<void>): Promise<void> {
    const button = event.currentTarget as HTMLButtonElement;
    const offset = Number(button.dataset.nextOffset ?? "");
    if (button.dataset.roleplayPage !== section || !Number.isSafeInteger(offset) || offset < 1) {
      throw new Error(`Roleplay ${section} pagination cursor is invalid.`);
    }
    await withRoleplayControlFeedback(button, () => load(offset), (error) => this.report(error));
  }

  private async loadWorlds(offset: number): Promise<void> {
    const page = await fetchRoleplayWorldsPage(offset);
    await this.host.renderComponentBundle(page.html.bundle);
  }

  private async loadCharacters(offset: number): Promise<void> {
    const page = await fetchRoleplayLibraryPage(this.host.selectedChannelID(), offset);
    await this.host.renderComponentBundle(page.html.bundle);
  }

  private async withLoading(operation: () => Promise<void>): Promise<void> {
    this.begin();
    try {
      await operation();
    } catch (error) {
      this.report(error);
      throw error;
    } finally {
      this.end();
    }
  }

  private begin(): void {
    this.pending += 1;
    if (this.host.hasLoading()) this.host.loading().classList.remove("hidden");
  }

  private end(): void {
    this.pending -= 1;
    if (this.pending < 0) throw new Error("Roleplay workspace request accounting underflowed.");
    if (this.pending === 0 && this.host.hasLoading()) this.host.loading().classList.add("hidden");
  }

  private report(error: unknown): void {
    this.host.setStatus("roleplay update failed", "error");
    this.host.addEvent("roleplay_workspace_failed", {
      error: error instanceof Error ? error.message : String(error),
    });
    this.host.reportError(error);
  }
}
