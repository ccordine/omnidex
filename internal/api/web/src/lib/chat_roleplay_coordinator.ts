import {
  createRoleplayScene,
  createRoleplayUserPersona,
  emptyRoleplayPage,
  fetchRoleplayComponent,
  registerRoleplayInteraction,
  registerRoleplayItem,
  registerRoleplayMeter,
  setRoleplayMeter,
	updateRoleplayScene,
	updateRoleplayResponders,
	writeRoleplaySceneDraftParticipant,
  type RoleplayComponentResponse,
  type RoleplayPageState,
} from "./roleplay_api";
import {
  interactionDefinitionInput,
  itemDefinitionInput,
  meterDefinitionInput,
  meterValueInput,
  requiredDataset,
	requiredDatasetInteger,
	sceneCreateInput,
	sceneDraftParticipantInput,
	sceneUpdateInput,
} from "./roleplay_form_input";
import { HTTPResponseError } from "./api";
import { pullOllamaModel } from "./ollama_model_api";
import {
  pageFromRoleplayButton,
  roleplayErrorMessage,
  withRoleplayControlFeedback,
  withRoleplayFormFeedback,
  type ChatRoleplayHost,
} from "./chat_roleplay_support";

export type { ChatRoleplayHost } from "./chat_roleplay_support";

export class ChatRoleplayCoordinator {
  private channelID = "";
	private configured = true;
	private responseGeneration = 0;
	private pendingRequests = 0;
	private renderGate: Promise<void> = Promise.resolve();

  constructor(private readonly host: ChatRoleplayHost) {}

  isConfigured(): boolean {
    return this.configured;
  }

	async activate(channelID: string, mode: "assistant" | "roleplay"): Promise<void> {
		this.responseGeneration += 1;
    if (mode === "assistant") {
      this.channelID = "";
      this.configured = true;
      if (this.host.hasPanel()) this.host.panel().classList.add("hidden");
      this.host.setComposerAvailable(true);
      return;
    }
    this.channelID = channelID;
    this.configured = false;
    if (!this.host.hasPanel()) throw new Error("The roleplay simulation panel is unavailable.");
    this.host.panel().classList.remove("hidden");
    this.host.setComposerAvailable(false);
    await this.load(emptyRoleplayPage);
  }

  async refresh(): Promise<void> {
    if (!this.channelID) return;
    await this.load(emptyRoleplayPage);
  }

  async loadPage(event: Event): Promise<void> {
    const button = event.currentTarget as HTMLButtonElement;
    const page = pageFromRoleplayButton(button);
    await withRoleplayControlFeedback(button, () => this.load(page), (error) => {
      this.host.addEvent("roleplay_page_failed", { error: roleplayErrorMessage(error) });
      this.host.reportError(error);
    });
  }

  useCommand(event: Event): void {
    const button = event.currentTarget as HTMLButtonElement;
    const exact = button.dataset.roleplayCommand;
    if (typeof exact !== "string" || !exact || exact !== exact.trim()) {
      throw new Error("The server-rendered roleplay command is missing or inexact.");
    }
    this.host.setComposerText(exact);
    this.host.focusComposer();
  }

  async createUserPersona(name: string): Promise<string> {
    const requestedChannel = this.requireChannel();
    const generation = ++this.responseGeneration;
    this.beginRequest();
    try {
      const component = await createRoleplayUserPersona(requestedChannel, name);
      if (!this.isCurrentResponse(requestedChannel, generation)) {
        throw new Error("The selected world changed while creating the identity.");
      }
      const characterID = component.composer_persona_character_id;
      if (!characterID) throw new Error("Created identity response omitted its character authority.");
      if (!await this.applyComponent(component, requestedChannel, generation)) {
        throw new Error("Created identity was not applied to the selected world.");
      }
      this.host.setStatus("identity added", "ready");
      this.host.addEvent("roleplay_user_persona_created", {
        channel_id: requestedChannel,
        character_id: characterID,
      });
      return characterID;
    } catch (error) {
      this.reportMutationFailure(error);
      throw error;
    } finally {
      this.endRequest();
    }
  }

  async updateResponders(characterIDs: string[], expectedRevision: number): Promise<void> {
    const requestedChannel = this.requireChannel();
    const generation = ++this.responseGeneration;
    this.beginRequest();
    try {
      const component = await updateRoleplayResponders(requestedChannel, {
        expected_revision: expectedRevision,
        character_ids: characterIDs,
      });
      if (!this.isCurrentResponse(requestedChannel, generation)) return;
      if (!await this.applyComponent(component, requestedChannel, generation)) return;
      this.host.setStatus("responders updated", "ready");
      this.host.addEvent("roleplay_responders_updated", {
        channel_id: requestedChannel,
        character_ids: characterIDs,
      });
    } catch (error) {
      if (error instanceof HTTPResponseError && error.status === 409 &&
          this.isCurrentResponse(requestedChannel, generation)) {
        this.reportMutationFailure(error);
        await this.load(emptyRoleplayPage);
        return;
      }
      if (this.isCurrentResponse(requestedChannel, generation)) this.reportMutationFailure(error);
    } finally {
      this.endRequest();
    }
  }

	async createScene(event: Event): Promise<void> {
		await this.mutate(event, (form) => createRoleplayScene(this.requireChannel(), sceneCreateInput(form)), "scene_created");
	}

	async updateScene(event: Event): Promise<void> {
		await this.mutate(event, (form) => updateRoleplayScene(
			this.requireChannel(),
			requiredDatasetInteger(form, "sceneRevision"),
			sceneUpdateInput(form),
		), "scene_updated");
	}

	async saveSceneDraftParticipant(event: Event): Promise<void> {
		await this.mutate(event, (form) => writeRoleplaySceneDraftParticipant(
			this.requireChannel(),
			requiredDataset(form, "characterId"),
			sceneDraftParticipantInput(form),
		), "scene_draft_saved");
  }

  async registerMeter(event: Event): Promise<void> {
    await this.mutate(event, (form) => registerRoleplayMeter(
      this.requireChannel(), meterDefinitionInput(form),
    ), "meter_registered");
  }

  async setMeter(event: Event): Promise<void> {
    await this.mutate(event, (form) => setRoleplayMeter(
      this.requireChannel(),
      requiredDataset(form, "characterId"),
      requiredDataset(form, "meterKey"),
      meterValueInput(form),
    ), "meter_saved");
  }

  async downloadModel(event: Event): Promise<void> {
    event.preventDefault();
    const form = event.currentTarget;
    if (!(form instanceof HTMLFormElement)) throw new Error("Roleplay model download form is invalid.");
    const control = form.elements.namedItem("model");
    if (!(control instanceof HTMLInputElement)) throw new Error("Roleplay model download input is missing.");
    const model = control.value.trim();
    if (!model || model.length > 256 || !/^[A-Za-z0-9._:/@-]+$/.test(model)) {
      this.host.setStatus("Enter a valid Ollama model tag.", "error");
      control.focus();
      return;
    }
    await withRoleplayFormFeedback(form, async () => {
      try {
        await pullOllamaModel(model);
        control.value = "";
        this.host.setStatus(`Downloading ${model}…`, "active");
        this.host.addEvent("roleplay_model_download_queued", {
          channel_id: this.requireChannel(),
          model,
        });
      } catch (error) {
        this.reportMutationFailure(error);
      }
    });
  }

  async registerInteraction(event: Event): Promise<void> {
    await this.mutate(event, (form) => registerRoleplayInteraction(
      this.requireChannel(), interactionDefinitionInput(form),
    ), "interaction_registered");
  }

  async registerItem(event: Event): Promise<void> {
    await this.mutate(event, (form) => registerRoleplayItem(
      this.requireChannel(), itemDefinitionInput(form),
    ), "item_registered");
  }

	private async load(page: RoleplayPageState): Promise<void> {
		const requestedChannel = this.requireChannel();
		const generation = ++this.responseGeneration;
		this.beginRequest();
		try {
			const component = await fetchRoleplayComponent(requestedChannel, page);
			if (!this.isCurrentResponse(requestedChannel, generation)) return;
			if (!await this.applyComponent(component, requestedChannel, generation)) return;
      this.host.addEvent("roleplay_loaded", {
        channel_id: requestedChannel,
        configured: component.configured,
        scene_revision: component.scene_revision ?? 0,
      });
    } catch (error) {
			if (this.isCurrentResponse(requestedChannel, generation)) {
        this.configured = false;
        this.host.setComposerAvailable(false);
        this.host.setStatus("roleplay unavailable", "error");
			}
			if (this.isCurrentResponse(requestedChannel, generation)) throw error;
		} finally {
			this.endRequest();
    }
  }

  private async mutate(
    event: Event,
    operation: (form: HTMLFormElement) => Promise<RoleplayComponentResponse>,
    eventName: string,
  ): Promise<void> {
		event.preventDefault();
		const form = event.currentTarget as HTMLFormElement;
		const requestedChannel = this.requireChannel();
		const generation = ++this.responseGeneration;
		let pending: Promise<RoleplayComponentResponse>;
    try {
      pending = operation(form);
    } catch (error) {
      this.reportMutationFailure(error);
      return;
    }
		await withRoleplayFormFeedback(form, async () => {
			this.beginRequest();
			try {
				const component = await pending;
				if (!this.isCurrentResponse(requestedChannel, generation)) return;
				if (!await this.applyComponent(component, requestedChannel, generation)) return;
				this.host.setStatus(component.configured ? "ready" : "setup required", component.configured ? "ready" : "active");
				this.host.addEvent(eventName, { channel_id: component.channel_id, scene_revision: component.scene_revision ?? 0 });
				await this.refreshSlashCommands();
			} catch (error) {
				if (!this.isCurrentResponse(requestedChannel, generation)) return;
				if (error instanceof HTTPResponseError && error.status === 409) {
					this.reportMutationFailure(error);
					await this.load(emptyRoleplayPage);
					if (this.channelID !== requestedChannel) return;
					this.host.setStatus("roleplay state refreshed", "active");
					this.host.addEvent("roleplay_conflict_rehydrated", { channel_id: requestedChannel });
					return;
				}
				this.reportMutationFailure(error);
			} finally {
				this.endRequest();
			}
    });
  }

  private reportMutationFailure(error: unknown): void {
    this.host.setStatus("roleplay update failed", "error");
    this.host.addEvent("roleplay_mutation_failed", { error: roleplayErrorMessage(error) });
    this.host.reportError(error);
  }

	private async refreshSlashCommands(): Promise<void> {
		if (!this.configured) return;
		try {
			await this.host.refreshSlashCommands();
		} catch (error) {
			this.host.setStatus("command hints unavailable", "error");
			this.host.addEvent("slash_commands_refresh_failed", { error: roleplayErrorMessage(error) });
			this.host.reportError(error);
		}
	}

	private async applyComponent(
		component: RoleplayComponentResponse,
		requestedChannel: string,
		generation: number,
	): Promise<boolean> {
		if (component.channel_id !== requestedChannel) {
			throw new Error("Roleplay component changed the active channel identity.");
		}
		const predecessor = this.renderGate;
		let release!: () => void;
		this.renderGate = new Promise<void>((resolve) => { release = resolve; });
		await predecessor;
		try {
			if (!this.isCurrentResponse(requestedChannel, generation)) return false;
			await this.host.renderComponentBundle(component.html.bundle);
			if (!this.isCurrentResponse(requestedChannel, generation)) return false;
			this.configured = component.configured;
			this.host.setComposerAvailable(component.configured);
			return true;
		} finally {
			release();
		}
  }

	private setLoading(loading: boolean): void {
    if (this.host.hasLoading()) this.host.loading().classList.toggle("hidden", !loading);
    if (this.host.hasPanel()) this.host.panel().setAttribute("aria-busy", String(loading));
	}

	private beginRequest(): void {
		this.pendingRequests += 1;
		this.setLoading(true);
	}

	private endRequest(): void {
		this.pendingRequests -= 1;
		if (this.pendingRequests < 0) throw new Error("Roleplay request accounting underflowed.");
		this.setLoading(this.pendingRequests > 0);
	}

	private isCurrentResponse(channelID: string, generation: number): boolean {
		return this.channelID === channelID && this.responseGeneration === generation;
	}

  private requireChannel(): string {
    if (!this.channelID) throw new Error("A roleplay channel must be selected.");
    return this.channelID;
  }
}
