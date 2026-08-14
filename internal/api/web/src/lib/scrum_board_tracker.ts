import { COLUMN_LABELS, type ScrumBoardResponse, type ScrumCard, type ScrumChecklistItem } from "./scrum_types";

export type ScrumBoardNotice = {
  message: string;
  tone: "info" | "ok";
};

export type ScrumBoardTransition = {
  duplicate: boolean;
  notices: ScrumBoardNotice[];
  commit: () => void;
};

export class ScrumBoardTracker {
  private fingerprint = "";
  private generation = 0;
  private readonly columns = new Map<string, string>();
  private readonly manualMoves = new Set<string>();

  reset(): void {
    this.fingerprint = "";
    this.generation++;
    this.columns.clear();
    this.manualMoves.clear();
  }

  markManualMove(cardID: string): void {
    cardID = cardID.trim();
    if (!cardID) throw new Error("Manual Scrum card move requires a card id.");
    this.manualMoves.add(cardID);
  }

  cancelManualMove(cardID: string): void {
    this.manualMoves.delete(cardID.trim());
  }

  prepare(payload: ScrumBoardResponse): ScrumBoardTransition {
    const fingerprint = boardPayloadFingerprint(payload);
    if (fingerprint === this.fingerprint) {
      return { duplicate: true, notices: [], commit: () => undefined };
    }

    const generation = this.generation;
    const notices = this.collectNotices(payload.board.cards);
    const movedCardIDs = payload.board.cards
      .filter((card) => this.columns.get(card.id) && this.columns.get(card.id) !== card.column)
      .map((card) => card.id);

    return {
      duplicate: false,
      notices,
      commit: () => {
        if (this.generation !== generation) {
          throw new Error("Cannot commit a stale Scrum board transition.");
        }
        this.generation++;
        this.fingerprint = fingerprint;
        this.columns.clear();
        payload.board.cards.forEach((card) => {
          this.columns.set(card.id, card.column);
        });
        movedCardIDs.forEach((cardID) => this.manualMoves.delete(cardID));
      },
    };
  }

  private collectNotices(cards: ScrumCard[]): ScrumBoardNotice[] {
    const notices: ScrumBoardNotice[] = [];
    cards.forEach((card) => {
      const previousColumn = this.columns.get(card.id);
      const manuallyMoved = this.manualMoves.has(card.id);
      if (previousColumn && previousColumn !== card.column && !manuallyMoved && card.column !== "in_progress" && card.column !== "review") {
        const label = COLUMN_LABELS[card.column] ?? card.column.replace(/_/g, " ");
        notices.push({ message: `Moved "${cardTitle(card.title)}" to ${label}`, tone: "info" });
      }
    });
    return notices;
  }
}

function cardTitle(title: string): string {
  const trimmed = title.trim();
  if (!trimmed) throw new Error("Authoritative Scrum card title must not be blank.");
  return trimmed.length > 52 ? `${trimmed.slice(0, 49)}…` : trimmed;
}

function normalizedText(value: unknown): string {
  return hash(String(value ?? "").replace(/\s+/g, " ").trim());
}

function messagesSymbol(messages?: ScrumCard["chat"]): string {
  return (messages ?? [])
    .map((message) => [message.id ?? "", message.role, normalizedText(message.content), message.created_at ?? "", message.status ?? ""].join(":"))
    .join("|");
}

function checklistSymbol(items?: ScrumChecklistItem[]): string {
  return (items ?? [])
    .map((item) => [item.id, item.done ? "1" : "0", normalizedText(item.text)].join(":"))
    .join("|");
}

function cardSymbol(card: ScrumCard): string {
  return hash([
    card.id,
    card.updated_at,
    card.column,
    card.board_order ?? 0,
    card.queue_order ?? 0,
    card.play_state ?? "",
    card.job_id ?? "",
    normalizedText(card.title),
    normalizedText(card.description),
    normalizedText(card.card_ticket),
    normalizedText(card.card_prompt),
    messagesSymbol(card.chat),
    card.tags?.map(normalizedText).join(",") ?? "",
    checklistSymbol(card.checklist),
    checklistSymbol(card.test_criteria),
    card.ref_files?.join(",") ?? "",
    card.flow_metrics ? JSON.stringify(card.flow_metrics) : "",
  ].join("::"));
}

function boardPayloadFingerprint(payload: ScrumBoardResponse): string {
  return hash([
    payload.board.id,
    payload.visible_column,
    payload.all_columns.join(","),
    JSON.stringify(payload.column_counts),
    payload.board.columns.join(","),
    payload.play_queue.running_card_id,
    payload.play_queue.queued_count,
    payload.play_queue.queued_card_ids.join(","),
    payload.auto_work.enabled ? "1" : "0",
    payload.auto_work.source_columns.join(","),
    JSON.stringify(payload.flow_summary),
    payload.board.cards.map(cardSymbol).join(";"),
    payload.html.bundle,
  ].join("||"));
}

function hash(value: string): string {
  let result = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    result ^= value.charCodeAt(index);
    result = Math.imul(result, 16777619);
  }
  return (result >>> 0).toString(36);
}
