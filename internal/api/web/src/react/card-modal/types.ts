import type { ScrumCard, ScrumCardModalResponse } from "../../lib/scrum_types";

export type CardModalTab = "card" | "files" | "tests" | "channel";

export const CARD_MODAL_TABS: Array<{ id: CardModalTab; label: string }> = [
  { id: "card", label: "Card" },
  { id: "files", label: "Files" },
  { id: "tests", label: "Tests" },
  { id: "channel", label: "Channel" },
];

export type RunMutation = <T>(label: string, fn: () => Promise<T>) => Promise<T | null>;

export type CardModalChildProps = {
  context: ScrumCardModalResponse;
  projectID: number;
  mutationBusy: boolean;
  runMutation: RunMutation;
  onCardUpdated: (card: ScrumCard, options?: { reloadContext?: boolean }) => void;
  onContextUpdated: (context: ScrumCardModalResponse) => void;
};

export function normalizeCardModalTab(raw: string | null | undefined): CardModalTab {
	if (raw === null || raw === undefined) return "card";
	if (!CARD_MODAL_TABS.some((tab) => tab.id === raw)) {
		throw new Error("Scrum card modal tab must be one exact registered value.");
	}
	return raw as CardModalTab;
}
