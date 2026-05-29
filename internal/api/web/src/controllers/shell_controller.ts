import { Controller } from "@hotwired/stimulus";

/** Keeps slide drawers open on tap/click (touch) until dismissed. Hover still works via CSS. */
export default class ShellController extends Controller {
  static targets = ["leftDrawer", "rightDrawer"];

  declare readonly leftDrawerTarget: HTMLElement;
  declare readonly rightDrawerTarget: HTMLElement;
  declare readonly hasLeftDrawerTarget: boolean;
  declare readonly hasRightDrawerTarget: boolean;

  private pinnedSide: "left" | "right" | null = null;

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
