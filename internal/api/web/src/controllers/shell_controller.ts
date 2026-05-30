import { Controller } from "@hotwired/stimulus";
import { toastOk } from "../lib/feedback";
import { applyI18n, initI18n, LOCALE_OPTIONS, applyDocumentLocale, getLocale, setLocale, t, type LocaleCode } from "../lib/i18n";

/** Keeps slide drawers open on tap/click (touch) until dismissed. Hover still works via CSS. */
export default class ShellController extends Controller {
  static targets = ["leftDrawer", "rightDrawer", "localeSelect"];

  declare readonly leftDrawerTarget: HTMLElement;
  declare readonly rightDrawerTarget: HTMLElement;
  declare readonly hasLeftDrawerTarget: boolean;
  declare readonly hasRightDrawerTarget: boolean;
  declare readonly hasLocaleSelectTarget: boolean;
  declare readonly localeSelectTarget: HTMLSelectElement;

  private pinnedSide: "left" | "right" | null = null;
  private localeChangedHandler: ((event: Event) => void) | null = null;

  connect() {
    initI18n();
    this.populateLocaleSelect();
    this.localeChangedHandler = () => this.populateLocaleSelect();
    document.addEventListener("omni:locale-changed", this.localeChangedHandler);
  }

  disconnect() {
    if (this.localeChangedHandler) {
      document.removeEventListener("omni:locale-changed", this.localeChangedHandler);
    }
  }

  setLocale(event: Event) {
    const value = (event.currentTarget as HTMLSelectElement).value as LocaleCode;
    if (!value || value === getLocale()) return;
    setLocale(value);
    applyDocumentLocale();
    applyI18n(document);
    document.dispatchEvent(new CustomEvent("omni:locale-changed", { detail: { locale: value } }));
    toastOk(t("locale.changed"));
  }

  private populateLocaleSelect() {
    if (!this.hasLocaleSelectTarget) return;
    const current = getLocale();
    this.localeSelectTarget.innerHTML = LOCALE_OPTIONS.map(
      (option) => `<option value="${option.code}"${option.code === current ? " selected" : ""}>${option.nativeLabel}</option>`,
    ).join("");
  }

  toggleLeft(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    this.togglePin("left");
  }

  toggleRight(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    this.togglePin("right");
  }

  private togglePin(side: "left" | "right") {
    if (this.pinnedSide === side) {
      this.pinnedSide = null;
    } else {
      this.pinnedSide = side;
    }
    this.applyPinned();
  }

  private applyPinned() {
    if (this.hasLeftDrawerTarget) {
      this.leftDrawerTarget.classList.toggle("is-open", this.pinnedSide === "left");
    }
    if (this.hasRightDrawerTarget) {
      this.rightDrawerTarget.classList.toggle("is-open", this.pinnedSide === "right");
    }
  }

  dismissDrawers(event: Event) {
    const target = event.target as HTMLElement;
    if (target.closest(".slide-drawer")) return;
    if (this.pinnedSide == null) return;
    this.pinnedSide = null;
    this.applyPinned();
  }
}
