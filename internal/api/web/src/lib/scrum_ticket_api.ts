import { readJSON } from "./api";
import { projectQuery } from "./project_api";
import type { ScrumCard } from "./scrum_types";

type ScrumCardTicketAction =
  | { action: "generate"; expected_updated_at: string }
  | { action: "elaborate"; expected_updated_at: string; elaboration: string };

async function executeScrumCardTicketAction(
  cardID: string,
  action: ScrumCardTicketAction,
  projectID?: number | null,
): Promise<ScrumCard> {
  if (!cardID.trim() || !action.expected_updated_at.trim()) {
    throw new Error("Card ID and observed card revision are required for a ticket action.");
  }
  if (action.action === "elaborate" && !action.elaboration.trim()) {
    throw new Error("Ticket elaboration must not be blank.");
  }
  const response = await fetch(
    `/v1/scrum/cards/${encodeURIComponent(cardID)}/card-ticket${projectQuery(projectID)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(action),
    },
  );
  const payload = await readJSON<{ card?: ScrumCard | null }>(response);
  if (!payload.card?.id) {
    throw new Error("Card ticket action did not return authoritative card state.");
  }
  return payload.card;
}

export function generateScrumCardTicket(
  cardID: string,
  expectedUpdatedAt: string,
  projectID?: number | null,
): Promise<ScrumCard> {
  return executeScrumCardTicketAction(cardID, {
    action: "generate",
    expected_updated_at: expectedUpdatedAt,
  }, projectID);
}

export function elaborateScrumCardTicket(
  cardID: string,
  expectedUpdatedAt: string,
  elaboration: string,
  projectID?: number | null,
): Promise<ScrumCard> {
  return executeScrumCardTicketAction(cardID, {
    action: "elaborate",
    expected_updated_at: expectedUpdatedAt,
    elaboration: elaboration.trim(),
  }, projectID);
}
