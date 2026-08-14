import { closeModalShell, getModalElements, openModalShell, resetModalPanelWidth } from "./modal";
import {
  clearScrumModalHref,
  isScrumCardTab,
  parseScrumCardFromLocation,
  parseScrumTabFromLocation,
  scrumModalHref,
  type ScrumCardTab,
} from "./panel_routing";

type ProjectIDProvider = () => number | null;
type CardExists = (cardID: string) => boolean;

export class ScrumCardModalHost {
  private cardID: string | null = null;
  private tab: ScrumCardTab = "card";

  constructor(
    private readonly projectID: ProjectIDProvider,
    private readonly cardExists: CardExists,
  ) {}

  activeCardID(): string | null {
    return this.cardID;
  }

  handleTabChanged(cardID: string | undefined, tab: string | undefined): void {
    if (cardID && cardID !== this.cardID) return;
		if (!tab) throw new Error("Scrum card modal tab change requires one exact registered tab.");
    if (!isScrumCardTab(tab)) throw new Error(`Unknown Scrum card modal tab ${JSON.stringify(tab)}.`);
    if (!this.cardID) throw new Error("Cannot change a Scrum card modal tab without an active card.");
    this.tab = tab;
    sessionStorage.setItem(this.storageKey(this.cardID), tab);
    this.syncRoute();
  }

  open(cardID: string): void {
    cardID = cardID.trim();
    if (!cardID) throw new Error("Scrum card modal requires a card id.");
    if (!this.cardExists(cardID)) {
      throw new Error(`Scrum card ${JSON.stringify(cardID)} is not present in server board state.`);
    }
    if (this.cardID === cardID && this.isOpen()) {
      this.syncRoute();
      return;
    }
		const projectID = this.projectID();
		if (!Number.isSafeInteger(projectID) || !projectID || projectID <= 0) {
			throw new Error("Scrum card modal requires one open server-authoritative project.");
		}

    const { modal, panel } = getModalElements();
    if (!modal || !panel) throw new Error("Scrum card modal shell is unavailable.");
    this.cardID = cardID;
    this.tab = this.resolveTab(cardID);
    openModalShell({ wide: true });
    const host = document.createElement("div");
    host.dataset.controller = "card-modal-spa";
    host.dataset.cardModalSpaCardIdValue = cardID;
    host.dataset.cardModalSpaInitialTabValue = this.tab;
    host.dataset.cardModalSpaProjectIdValue = String(projectID);
    panel.replaceChildren(host);
    this.syncRoute();
  }

  openFromLocation(): boolean {
    const cardID = parseScrumCardFromLocation();
    if (!cardID || this.isOpen()) return false;
    this.open(cardID);
    return true;
  }

  clearCardRoute(): void {
    this.cardID = null;
    this.tab = "card";
    this.syncRoute();
  }

  close(): void {
    this.clearCardRoute();
    closeModalShell();
  }

  reset(): void {
    this.cardID = null;
    this.tab = "card";
    getModalElements().panel?.replaceChildren();
    resetModalPanelWidth();
    this.syncRoute();
  }

  private isOpen(): boolean {
    const { modal, panel } = getModalElements();
    return Boolean(modal && !modal.classList.contains("hidden") && panel?.querySelector("[data-card-modal-spa-card-id-value]"));
  }

  private resolveTab(cardID: string): ScrumCardTab {
    if (parseScrumCardFromLocation() === cardID) return parseScrumTabFromLocation();
    const saved = sessionStorage.getItem(this.storageKey(cardID));
		if (saved === null) return "card";
		if (!isScrumCardTab(saved)) {
			throw new Error("Saved Scrum card modal tab must be one exact registered value.");
		}
		return saved;
  }

  private storageKey(cardID: string): string {
    return `omni.scrum.card-tab.${cardID}`;
  }

  private syncRoute(): void {
    const href = this.cardID ? scrumModalHref(this.cardID, this.tab) : clearScrumModalHref();
    history.replaceState(null, document.title, href);
  }
}
