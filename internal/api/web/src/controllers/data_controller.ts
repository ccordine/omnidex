import { Controller } from "@hotwired/stimulus";
import {
  createDataSourceChannel,
  fetchDataSourceChannelMessages,
  fetchDataSourceChannels,
  fetchDataSourcesPublic,
  fetchJobRecord,
  sendDataSourceChannelMessage,
  type JobRecord,
} from "../lib/data_api";
import { mountDataCharts } from "../lib/data_chart";
import { emptyDataPanelState, renderDataPanel, type DataPanelState } from "../lib/data_render";
import { panelHref, parseDataChannelFromLocation, parseDataSourceFromLocation } from "../lib/panel_routing";
import type RecyclrController from "./recyclr_controller";
import { observeRealtimeJob, type RealtimeJobObservation } from "../lib/realtime_job_observer";

export default class DataController extends Controller {
  static targets = ["panel"];

  declare readonly panelTarget: HTMLElement;

  private state: DataPanelState = emptyDataPanelState();
  private panelShownHandler: ((event: Event) => void) | null = null;
  private jobObservation: RealtimeJobObservation<{ job: JobRecord }> | null = null;

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
    this.stopJobObservation("Data controller disconnected.");
  }

  private recyclrController(): RecyclrController {
    const controller = this.application.getControllerForElementAndIdentifier(document.body, "recyclr") as RecyclrController | null;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  private pushRoute() {
    this.recyclrController().pushRoute(
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
    this.stopJobObservation("Data source changed.");
    this.pushRoute();
    void this.loadChannels();
  }

  selectChannel(event: Event) {
    event.preventDefault();
    const id = (event.currentTarget as HTMLElement).dataset.channelId || "";
    if (!id || id === this.state.selectedChannelId) return;
    this.state.selectedChannelId = id;
    this.state.pendingJobId = null;
    this.stopJobObservation("Data channel changed.");
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
    if (!sourceID || !channelID) {
      this.state.status = "Select a data source and channel first.";
      this.render();
      return;
    }
    if (!prompt) {
      this.state.status = "Enter a question first.";
      this.render();
      return;
    }
    this.state.status = "Sending…";
    this.render();
    try {
      const result = await sendDataSourceChannelMessage(sourceID, channelID, prompt);
      this.state.pendingJobId = result.job.id;
      this.state.status = `Running job #${result.job.id}…`;
      this.restorePrompt("");
      await this.loadMessages(false);
      this.render(true);
      this.observeJob(result.job.id);
    } catch (error) {
      this.state.status = error instanceof Error ? error.message : String(error);
      this.render();
    }
  }

  private observeJob(jobID: number) {
    this.stopJobObservation("A newer data job replaced this observation.");
    const observation = observeRealtimeJob({
      jobID,
      load: async () => {
        const details = await fetchJobRecord(jobID);
        if (!details.job || details.job.id !== jobID) {
          throw new Error(`Authoritative job response did not include job #${jobID}.`);
        }
        return { status: details.job.status, error: details.job.error, data: details };
      },
      onUpdate: async ({ status }) => {
        if (this.jobObservation !== observation) return;
        this.state.status = status === "completed" ? "Finalizing results…" : `Running job #${jobID} · ${status}…`;
        await this.loadMessages(false);
        this.render(true);
      },
    });
    this.jobObservation = observation;
    void observation.completion
      .then(async () => {
        if (this.jobObservation !== observation) return;
        this.state.pendingJobId = null;
        this.state.status = "Ready";
        await this.loadMessages(false);
        this.render(true);
      })
      .catch((error) => {
        if (this.jobObservation !== observation) return;
        this.state.pendingJobId = null;
        this.state.status = error instanceof Error ? error.message : String(error);
        this.render();
      })
      .finally(() => {
        if (this.jobObservation === observation) this.jobObservation = null;
      });
  }

  private stopJobObservation(reason: string) {
    const observation = this.jobObservation;
    this.jobObservation = null;
    observation?.cancel(reason);
  }
}
