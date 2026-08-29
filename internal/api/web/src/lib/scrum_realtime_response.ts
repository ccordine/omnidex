import {
  exactInteger,
  exactRecord,
  exactString,
  exactTimestamp,
  validateScrumCardProjection,
} from "./scrum_card_response";
import { SCRUM_CARD_REALTIME_REASON, type ScrumCard } from "./scrum_types";

const REALTIME_CARD_FIELDS = [
  "id", "stateKey", "occurredAt", "eventName", "reason", "toast", "toastTone",
  "projectID", "cardID", "card",
] as const;
const REALTIME_CARD_REQUIRED_FIELDS = [
  "id", "stateKey", "occurredAt", "eventName", "reason", "projectID", "cardID", "card",
] as const;
const REALTIME_CARD_REASONS = [
  "channel chat", SCRUM_CARD_REALTIME_REASON.jobProgress, "job resolved", "job updated",
] as const;

export type ScrumRealtimeCardUpdate = {
  projectID: number;
  cardID: string;
  card: ScrumCard;
  reason: (typeof REALTIME_CARD_REASONS)[number];
};

export function validateScrumRealtimeCardUpdate(value: unknown): ScrumRealtimeCardUpdate {
  const record = exactRecord(
    value,
    "Scrum realtime card update",
    REALTIME_CARD_FIELDS,
    REALTIME_CARD_REQUIRED_FIELDS,
  );
  const id = exactInteger(record.id, "Scrum realtime card update.id");
  if (id < 1) throw new Error("Scrum realtime card update.id must be positive.");
  const projectID = exactInteger(record.projectID, "Scrum realtime card update.projectID");
  if (projectID < 1) throw new Error("Scrum realtime card update.projectID must be positive.");
  const cardID = exactString(record.cardID, "Scrum realtime card update.cardID", {
    maxBytes: 256, nonblank: true, canonical: true,
  });
  const stateKey = exactString(record.stateKey, "Scrum realtime card update.stateKey", {
    maxBytes: 512, nonblank: true, canonical: true,
  });
  if (stateKey !== `scrum-card:${projectID}:${cardID}`) {
    throw new Error("Scrum realtime card update.stateKey contradicts its route identity.");
  }
  exactTimestamp(record.occurredAt, "Scrum realtime card update.occurredAt");
  if (record.eventName !== "scrum-card-updated") {
    throw new Error("Scrum realtime card update.eventName is not registered.");
  }
  const reason = exactString(record.reason, "Scrum realtime card update.reason", {
    maxBytes: 64, nonblank: true, canonical: true,
  });
  if (!REALTIME_CARD_REASONS.includes(reason as (typeof REALTIME_CARD_REASONS)[number])) {
    throw new Error("Scrum realtime card update.reason is not registered.");
  }
  if ("toast" in record) {
    exactString(record.toast, "Scrum realtime card update.toast", { maxBytes: 1024, nonblank: true });
    if (!Object.prototype.hasOwnProperty.call(record, "toastTone")) {
      throw new Error("Scrum realtime card update.toast requires one registered tone.");
    }
  }
  if ("toastTone" in record) {
    if (!("toast" in record) || !["info", "busy", "ok", "error"].includes(String(record.toastTone))) {
      throw new Error("Scrum realtime card update.toastTone is contradictory or unregistered.");
    }
  }
  return {
    projectID,
    cardID,
    card: validateScrumCardProjection(record.card, cardID),
    reason: reason as (typeof REALTIME_CARD_REASONS)[number],
  };
}
