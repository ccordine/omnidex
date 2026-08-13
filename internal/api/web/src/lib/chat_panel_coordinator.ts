import { readJSON } from "./api";
import { requireServerComponentBundle } from "./chat_component_api";
import { errorMessage } from "./feedback";
import {
  isOmniPanel,
  panelHref,
  parseAdminTabFromLocation,
  type OmniPanel,
} from "./panel_routing";

interface PanelResponse {
  panel?: unknown;
  locale?: unknown;
  html?: unknown;
}

export interface ChatPanelHost {
  root(): Element;
  locale(): string;
  renderPanel(html: string): Promise<void>;
  loadPanelData(panel: OmniPanel): void;
  pushRoute(path: string): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  reportError(error: unknown): void;
}

export class ChatPanelCoordinator {
  private activePanel: OmniPanel = "chat";

  constructor(private readonly host: ChatPanelHost) {}

  current(): OmniPanel {
    return this.activePanel;
  }

  isCurrent(panel: OmniPanel): boolean {
    return this.activePanel === panel;
  }

  async show(event: Event): Promise<void> {
    event.preventDefault();
    const rawPanel = (event.currentTarget as HTMLElement | null)?.dataset.panel;
    if (!isOmniPanel(rawPanel)) {
      throw new Error(`Invalid Omni panel ${JSON.stringify(rawPanel ?? "")}.`);
    }
    await this.activate(rawPanel, { pushHistory: true });
  }

  async activate(panel: OmniPanel, options: { pushHistory?: boolean } = {}): Promise<void> {
    if (!isOmniPanel(panel)) {
      throw new Error(`Invalid Omni panel ${JSON.stringify(panel)}.`);
    }

    try {
      const payload = await this.fetchPanel(panel);
      await this.host.renderPanel(payload.bundle);
    } catch (error) {
      this.host.addEvent("ui_panel_error", { panel, error: errorMessage(error) });
      this.host.reportError(error);
      throw error;
    }

    this.activePanel = panel;
    this.updateNavigation(panel);
    document.dispatchEvent(new CustomEvent("omni:panel-shown", { detail: { panel } }));
    this.host.loadPanelData(panel);

    if (options.pushHistory) {
      const extra = panel === "admin" ? { admin_tab: parseAdminTabFromLocation() } : {};
      this.host.pushRoute(panelHref(panel, window.location, extra));
    }
  }

  private async fetchPanel(panel: OmniPanel): Promise<{ bundle: string }> {
    const params = new URLSearchParams(window.location.search);
    if (panel === "chat") params.delete("panel");
    else params.set("panel", panel);
    const query = params.toString();
    const payload = await readJSON<PanelResponse>(await fetch(`/v1/ui/panel${query ? `?${query}` : ""}`));

    if (payload.panel !== panel) {
      throw new Error(`Server returned panel ${JSON.stringify(payload.panel)} when the client requested ${JSON.stringify(panel)}.`);
    }
    const locale = this.host.locale();
    if (payload.locale !== locale) {
      throw new Error(`Server returned panel locale ${JSON.stringify(payload.locale)} while the shell locale is ${JSON.stringify(locale)}.`);
    }
    return { bundle: requireServerComponentBundle(payload, `Panel ${panel}`) };
  }

  private updateNavigation(panel: OmniPanel): void {
    for (const button of this.host.root().querySelectorAll(".nav-button")) {
      const active = (button as HTMLElement).dataset.panel === panel;
      button.classList.toggle("is-active", active);
      button.classList.toggle("bg-white/[.06]", active);
      button.classList.toggle("text-zinc-100", active);
      button.classList.toggle("text-zinc-300", !active);
    }
  }
}
