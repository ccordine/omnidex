import { Controller } from "@hotwired/stimulus";
import { createRoot, type Root } from "react-dom/client";
import { CardModalApp } from "../react/card-modal/CardModalApp";

export default class CardModalSpaController extends Controller<HTMLElement> {
  static values = {
    cardId: String,
    projectId: Number,
    initialTab: String,
  };

  declare readonly cardIdValue: string;
  declare readonly projectIdValue: number;
  declare readonly hasProjectIdValue: boolean;
  declare readonly initialTabValue: string;

  private root: Root | null = null;

  connect(): void {
    if (this.root) return;
    const cardID = this.cardIdValue.trim();
    if (!cardID) {
      throw new Error("card-modal-spa requires cardId");
    }
    this.root = createRoot(this.element);
    this.root.render(
      <CardModalApp
        cardID={cardID}
        projectID={this.hasProjectIdValue && this.projectIdValue > 0 ? this.projectIdValue : null}
        initialTab={this.initialTabValue || "card"}
      />,
    );
  }

  disconnect(): void {
    this.root?.unmount();
    this.root = null;
  }
}
