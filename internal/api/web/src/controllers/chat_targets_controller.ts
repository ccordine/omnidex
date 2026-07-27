import { Controller } from "@hotwired/stimulus";

export abstract class ChatTargetsController extends Controller {
  static targets = [
    "messages", "timeline", "input", "send", "status", "transport", "networkUrl", "job", "liveBadge", "eventCount", "panel",
    "jobFilter", "jobsList", "jobDetails", "memoryCandidates", "memoryList", "memoryKind", "memoryKindFilter", "memoryTags", "memoryContent",
    "personaMode", "personaModel", "personaSystem", "personaPrompt", "personaOutput", "statusOutput", "researchStatusOutput", "hostBridgeStatusOutput",
    "metricsOutput", "progress", "progressState", "spinner", "modal", "modalPanel", "channelSelect",
  ];

  declare readonly messagesTarget: HTMLElement;
  declare readonly timelineTarget: HTMLElement;
  declare readonly inputTarget: HTMLTextAreaElement;
  declare readonly sendTarget: HTMLButtonElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly transportTarget: HTMLElement;
  declare readonly networkUrlTarget: HTMLElement;
  declare readonly jobTarget: HTMLElement;
  declare readonly liveBadgeTarget: HTMLElement;
  declare readonly eventCountTarget: HTMLElement;
  declare readonly panelTargets: HTMLElement[];
  declare readonly jobFilterTarget: HTMLSelectElement;
  declare readonly jobsListTarget: HTMLElement;
  declare readonly jobDetailsTarget: HTMLElement;
  declare readonly memoryCandidatesTarget: HTMLElement;
  declare readonly memoryListTarget: HTMLElement;
  declare readonly memoryKindTarget: HTMLSelectElement;
  declare readonly memoryKindFilterTarget: HTMLSelectElement;
  declare readonly memoryTagsTarget: HTMLInputElement;
  declare readonly memoryContentTarget: HTMLTextAreaElement;
  declare readonly personaModeTarget: HTMLSelectElement;
  declare readonly personaModelTarget: HTMLInputElement;
  declare readonly personaSystemTarget: HTMLTextAreaElement;
  declare readonly personaPromptTarget: HTMLTextAreaElement;
  declare readonly personaOutputTarget: HTMLElement;
  declare readonly statusOutputTarget: HTMLElement;
  declare readonly researchStatusOutputTarget: HTMLElement;
  declare readonly hostBridgeStatusOutputTarget: HTMLElement;
  declare readonly metricsOutputTarget: HTMLElement;
  declare readonly progressTarget: HTMLElement;
  declare readonly progressStateTarget: HTMLElement;
  declare readonly spinnerTarget: HTMLElement;
  declare readonly modalTarget: HTMLElement;
  declare readonly modalPanelTarget: HTMLElement;
  declare readonly hasMemoryListTarget: boolean;
  declare readonly hasStatusOutputTarget: boolean;
  declare readonly hasResearchStatusOutputTarget: boolean;
  declare readonly hasHostBridgeStatusOutputTarget: boolean;
  declare readonly hasMetricsOutputTarget: boolean;
  declare readonly hasProgressStateTarget: boolean;
  declare readonly hasModalTarget: boolean;
  declare readonly hasSpinnerTarget: boolean;
  declare readonly hasNetworkUrlTarget: boolean;
  declare readonly hasChannelSelectTarget: boolean;
  declare readonly hasJobsListTarget: boolean;
  declare readonly hasJobDetailsTarget: boolean;
  declare readonly hasMessagesTarget: boolean;
  declare readonly hasInputTarget: boolean;
  declare readonly hasSendTarget: boolean;
  declare readonly hasStatusTarget: boolean;
  declare readonly hasLiveBadgeTarget: boolean;
  declare readonly hasTransportTarget: boolean;
  declare readonly hasJobTarget: boolean;
  declare readonly hasEventCountTarget: boolean;
  declare readonly channelSelectTarget: HTMLSelectElement;
}
