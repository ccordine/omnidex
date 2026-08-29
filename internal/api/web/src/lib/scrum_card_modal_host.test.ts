import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ScrumCardModalHost } from "./scrum_card_modal_host";

describe("ScrumCardModalHost", () => {
  beforeEach(() => {
    history.replaceState(null, document.title, "/chat?panel=projects&scrum_card=card_1&scrum_tab=channel");
    document.body.innerHTML = `
      <div data-chat-target="modal" class="hidden">
        <div data-chat-target="modalPanel"></div>
      </div>`;
  });

  afterEach(() => {
    document.body.innerHTML = "";
    sessionStorage.clear();
    history.replaceState(null, document.title, "/chat");
  });

  it("restores a deep-linked card as the single typed React modal host", () => {
    const host = new ScrumCardModalHost(() => 7, (cardID) => cardID === "card_1");

    expect(host.openFromLocation()).toBe(true);

    const element = document.querySelector<HTMLElement>("[data-card-modal-spa-card-id-value]");
    expect(element?.dataset.controller).toBe("card-modal-spa");
    expect(element?.dataset.cardModalSpaCardIdValue).toBe("card_1");
    expect(element?.dataset.cardModalSpaProjectIdValue).toBe("7");
    expect(element?.dataset.cardModalSpaInitialTabValue).toBe("channel");
    expect(document.querySelector("[data-scrum-tab-panel]")).toBeNull();
  });

  it("fails loudly instead of opening a modal for an unknown card", () => {
    const host = new ScrumCardModalHost(() => 7, () => false);
    expect(() => host.open("missing")).toThrow('Scrum card "missing" is not present in server board state.');
  });

  it("rejects an invalid project before changing modal state", () => {
    const host = new ScrumCardModalHost(() => null, () => true);

    expect(() => host.open("card_1")).toThrow("one open server-authoritative project");
    expect(host.activeCardID()).toBeNull();
    expect(document.querySelector("[data-card-modal-spa-card-id-value]")).toBeNull();
    expect(document.querySelector("[data-chat-target='modal']")?.classList.contains("hidden")).toBe(true);
  });
});
