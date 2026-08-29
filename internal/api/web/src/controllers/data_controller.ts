import { Controller } from "@hotwired/stimulus";
import { createDataSourceChannel } from "../lib/data_api";
import { fetchDataComponent } from "../lib/operational_component_api";
import { renderServerBundle } from "../lib/server_component_api";
import { panelHref, parseDataChannelFromLocation, parseDataSourceFromLocation } from "../lib/panel_routing";
import type RecyclrController from "./recyclr_controller";
import { setGlobalLoading } from "../lib/loading";
import { showToast } from "../lib/toast";

export default class DataController extends Controller {
  private selectedSourceID = "";
  private selectedChannelID = "";
  private sourceOffset = 0;
  private channelOffset = 0;
	private messageOffset = 0;
  private panelShownHandler: ((event: Event) => void) | null = null;

  connect(): void {
    this.selectedSourceID = parseDataSourceFromLocation() ?? "";
    this.selectedChannelID = parseDataChannelFromLocation() ?? "";
    this.panelShownHandler = (event) => {
      if ((event as CustomEvent<{ panel?: string }>).detail?.panel === "data") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
    void this.load();
  }

  disconnect(): void {
    if (this.panelShownHandler) document.removeEventListener("omni:panel-shown", this.panelShownHandler);
  }

  private recyclrController(): RecyclrController {
    const controller = this.application.getControllerForElementAndIdentifier(document.body, "recyclr") as RecyclrController | null;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  private pushRoute(): void {
    this.recyclrController().pushRoute(panelHref("data", window.location, {
      data_source: this.selectedSourceID,
      data_channel: this.selectedChannelID,
    }));
  }

  async load(): Promise<void> {
    try {
		const payload = await fetchDataComponent(this.selectedSourceID, this.selectedChannelID, this.sourceOffset, this.channelOffset, this.messageOffset);
      this.selectedSourceID = payload.selected_source_id ?? "";
      this.selectedChannelID = payload.selected_channel_id ?? "";
      this.sourceOffset = payload.source_offset ?? 0;
      this.channelOffset = payload.channel_offset ?? 0;
		this.messageOffset = payload.message_offset ?? 0;
      await renderServerBundle(this.recyclrController(), payload, "Data component");
      const messages = this.element.querySelector("[data-data-target='messageList']");
      if (messages) messages.scrollTop = messages.scrollHeight;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      console.error("Server data component failed", error);
      showToast(message, "error");
    }
  }

  selectSource(event: Event): void {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId?.trim() ?? "";
    if (!id || id === this.selectedSourceID) return;
    this.selectedSourceID = id;
    this.selectedChannelID = "";
    this.channelOffset = 0;
		this.messageOffset = 0;
    this.pushRoute();
    void this.load();
  }

  loadDataPage(event: Event): void {
    event.preventDefault();
    const node = event.currentTarget as HTMLElement;
    const offset = Number(node.dataset.pageOffset ?? -1);
    const kind = node.dataset.pageKind;
		if (!Number.isSafeInteger(offset) || offset < 0 || (kind !== "source" && kind !== "channel" && kind !== "message")) {
      throw new Error("Data page control is invalid.");
    }
    if (kind === "source") {
      this.sourceOffset = offset;
      this.channelOffset = 0;
      this.selectedSourceID = "";
      this.selectedChannelID = "";
			this.messageOffset = 0;
    } else {
			if (kind === "channel") {
				this.channelOffset = offset;
				this.selectedChannelID = "";
				this.messageOffset = 0;
			} else {
				this.messageOffset = offset;
			}
    }
    void this.load();
  }

  selectChannel(event: Event): void {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.channelId?.trim() ?? "";
    if (!id || id === this.selectedChannelID) return;
    this.selectedChannelID = id;
		this.messageOffset = 0;
    this.pushRoute();
    void this.load();
  }

  async createChannel(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.selectedSourceID) return;
    const name = window.prompt("Channel name", "New analysis")?.trim();
    if (!name) return;
    setGlobalLoading(true);
    try {
      const channel = await createDataSourceChannel(this.selectedSourceID, name);
      this.selectedChannelID = channel.id;
      this.channelOffset = 0;
			this.messageOffset = 0;
      this.pushRoute();
      await this.load();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      console.error("Create data channel failed", error);
      showToast(message, "error");
    } finally {
      setGlobalLoading(false);
    }
  }
}
