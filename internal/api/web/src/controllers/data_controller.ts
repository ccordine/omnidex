import { Controller } from "@hotwired/stimulus";
import {
  createDataSourceChannel,
  fetchDataSourceChannelMessages,
  fetchDataSourceChannels,
  fetchDataSourcesPublic,
  fetchJobRecord,
  sendDataSourceChannelMessage,
} from "../lib/data_api";
import { mountDataCharts } from "../lib/data_chart";
import { emptyDataPanelState, renderDataPanel, type DataPanelState } from "../lib/data_render";
import { panelHref, parseDataChannelFromLocation, parseDataSourceFromLocation } from "../lib/panel_routing";
import type GxController from "./gx_controller";

export default class DataController extends Controller {
  static targets = ["panel"];

  declare readonly panelTarget: HTMLElement;

  private state: DataPanelState = emptyDataPanelState();
  private panelShownHandler: ((event: Event) => void) | null = null;
  private pollTimer: number | null = null;

  connect() {
    const sourceID = parseDataSourceFromLocation();
    const channelID = parseDataChannelFromLocation();
    if (sourceID) this.state.selectedSourceId = sourceID;
    if (channelID) this.state.selectedChannelId = channelID;
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "data") void this.load();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);
    void this.load();
  }

  disconnect() {
    if (this.panelShownHandler) document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    this.stopPolling();
  }

  private gxController(): GxController | null {
    return this.application.getControllerForElementAndIdentifier(document.body, "gx") as GxController | null;
  }

  private pushRoute() {
    this.gxController()?.pushRoute(
      panelHref("data", window.location, {
        data_source: this.state.selectedSourceId || "",
        data_channel: this.state.selectedChannelId || "",
      }),
    );
  }

  private preservePrompt(): string {
    return (this.element.querySelector("[data-data-target='promptInput']") as HTMLInputElement | null)?.value ?? "";
  }

  private restorePrompt(value: string) {
    const input = this.element.querySelector("[data-data-target='promptInput']") as HTMLInputElement | null;
    if (input) input.value = value;
  }

  private render(scrollMessages = false) {
    const prompt = this.preservePrompt();
    this.panelTarget.innerHTML = renderDataPanel(this.state);
    this.restorePrompt(prompt);
    mountDataCharts(this.panelTarget);
    if (scrollMessages) {
      const list = this.element.querySelector("[data-data-target='messageList']");
      if (list) list.scrollTop = list.scrollHeight;
    }
  }

  async load() {
    this.state.status = "Loading databases…";
    this.render();
    try {
      const sources = await fetchDataSourcesPublic();
      this.state.sources = sources;
      if (!this.state.selectedSourceId || !sources.some((s) => s.id === this.state.selectedSourceId)) {
        this.state.selectedSourceId = sources[0]?.id ?? null;
      }
      await this.loadChannels(false);
      this.state.status = "Ready";
      this.render(true);
    } catch (error) {
      this.state.status = error instanceof Error ? error.message : String(error);
      this.render();
    }
  }

  async loadChannels(renderPanel = true) {
    const sourceID = this.state.selectedSourceId;
    if (!sourceID) {
      this.state.channels = [];
      this.state.selectedChannelId = null;
      this.state.messages = [];
      if (renderPanel) this.render();
      return;
    }
    this.state.channels = await fetchDataSourceChannels(sourceID);
    if (!this.state.selectedChannelId || !this.state.channels.some((c) => c.id === this.state.selectedChannelId)) {
      this.state.selectedChannelId = this.state.channels[0]?.id ?? null;
    }
    await this.loadMessages(false);
    if (renderPanel) this.render(true);
  }

  async loadMessages(renderPanel = true) {
    const sourceID = this.state.selectedSourceId;
    const channelID = this.state.selectedChannelId;
    if (!sourceID || !channelID) {
      this.state.messages = [];
      if (renderPanel) this.render();
      return;
    }
    this.state.messages = await fetchDataSourceChannelMessages(sourceID, channelID);
    if (renderPanel) this.render(true);
  }

  selectSource(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.sourceId || "";
    if (!id || id === this.state.selectedSourceId) return;
    this.state.selectedSourceId = id;
    this.state.selectedChannelId = null;
    this.state.messages = [];
    this.state.pendingJobId = null;
    this.stopPolling();
    this.pushRoute();
    void this.loadChannels();
  }

  selectChannel(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.channelId || "";
    if (!id || id === this.state.selectedChannelId) return;
    this.state.selectedChannelId = id;
    this.state.pendingJobId = null;
    this.stopPolling();
    this.pushRoute();
    void this.loadMessages();
  }

  async createChannel(event: Event) {
    event.preventDefault();
    const sourceID = this.state.selectedSourceId;
    if (!sourceID) return;
    const name = window.prompt("Channel name", "New analysis")?.trim();
    if (!name) return;
    this.state.status = "Creating channel…";
    this.render();
    try {
      const channel = await createDataSourceChannel(sourceID, name);
      await this.loadChannels(false);
      this.state.selectedChannelId = channel.id;
      this.state.status = "Ready";
      this.pushRoute();
      this.render(true);
    } catch (error) {
      this.state.status = error instanceof Error ? error.message : String(error);
      this.render();
    }
  }

  async sendMessage(event: Event) {
    event.preventDefault();
    const sourceID = this.state.selectedSourceId;
    const channelID = this.state.selectedChannelId;
    const prompt = this.preservePrompt().trim();
    if (!sourceID || !channelID || !prompt) return;
    this.state.status = "Sending…";
    this.render();
    try {
      const result = await sendDataSourceChannelMessage(sourceID, channelID, prompt);
      this.state.pendingJobId = result.job.id;
      this.state.status = `Running job #${result.job.id}…`;
      this.restorePrompt("");
      await this.loadMessages(false);
      this.render(true);
      this.startPolling(result.job.id);
    } catch (error) {
      this.state.status = error instanceof Error ? error.message : String(error);
      this.render();
    }
  }

  private startPolling(jobID: number) {
    this.stopPolling();
    const tick = async () => {
      try {
        const details = await fetchJobRecord(jobID);
        const status = details.job?.status || "";
        if (status === "completed" || status === "failed" || status === "canceled") {
          this.state.pendingJobId = null;
          this.state.status = status === "completed" ? "Ready" : details.job?.error || `Job ${status}`;
          await this.loadMessages(false);
          this.render(true);
          this.stopPolling();
          return;
        }
        this.state.status = `Running job #${jobID} · ${status}…`;
        await this.loadMessages(false);
        this.render(true);
      } catch {
        /* keep polling */
      }
    };
    void tick();
    this.pollTimer = window.setInterval(() => void tick(), 900);
  }

  private stopPolling() {
    if (this.pollTimer != null) {
      window.clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }
}
