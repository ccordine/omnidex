import type { ScrumCard, ScrumCardModalResponse } from "../../lib/scrum_types";

export type CardModalTab = "card" | "files" | "tests" | "config" | "recipe" | "channel";

export const CARD_MODAL_TABS: Array<{ id: CardModalTab; label: string }> = [
  { id: "card", label: "Card" },
  { id: "files", label: "Files" },
  { id: "tests", label: "Tests" },
  { id: "config", label: "Config" },
  { id: "recipe", label: "Recipe" },
  { id: "channel", label: "Channel" },
];

export type RunMutation = <T>(label: string, fn: () => Promise<T>) => Promise<T | null>;

export type CardModalChildProps = {
  context: ScrumCardModalResponse;
  projectID: number | null;
  runMutation: RunMutation;
  onCardUpdated: (card: ScrumCard, options?: { reloadContext?: boolean }) => void;
};

export function normalizeCardModalTab(raw: string | null | undefined): CardModalTab {
  const value = String(raw ?? "").trim().toLowerCase();
  return CARD_MODAL_TABS.some((tab) => tab.id === value) ? (value as CardModalTab) : "card";
}
