import { readJSON } from "./api";
import { projectQuery } from "./project_api";
import { validateScrumCardEnvelope } from "./scrum_response_authority";
import { exactString } from "./scrum_card_response";
import { validateCanonicalRevision } from "./revision_authority";
import type { ScrumCard } from "./scrum_types";

type ScrumCardTicketAction =
  | { action: "assemble"; expected_updated_at: string }
  | { action: "apply_elaboration"; expected_updated_at: string; elaboration: string };

async function executeScrumCardTicketAction(
  cardID: string,
  action: ScrumCardTicketAction,
  projectID: number,
): Promise<ScrumCard> {
  const exactCardID = exactString(cardID, "Scrum card ticket action card ID", {
    maxBytes: 256, nonblank: true, canonical: true,
  });
  const revision = validateCanonicalRevision(action.expected_updated_at, "Scrum card ticket action expected_updated_at");
  if (action.action === "apply_elaboration" && !action.elaboration.trim()) {
    throw new Error("Ticket elaboration must not be blank.");
  }
  const response = await fetch(
    `/v1/scrum/cards/${encodeURIComponent(exactCardID)}/card-ticket${projectQuery(projectID)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...action, expected_updated_at: revision }),
    },
  );
  return validateScrumCardEnvelope(await readJSON<unknown>(response), exactCardID);
}

export function assembleScrumCardTicket(
  cardID: string,
  expectedUpdatedAt: string,
  projectID: number,
): Promise<ScrumCard> {
  return executeScrumCardTicketAction(cardID, {
    action: "assemble",
    expected_updated_at: expectedUpdatedAt,
  }, projectID);
}

export function applyScrumCardElaboration(
  cardID: string,
  expectedUpdatedAt: string,
  elaboration: string,
  projectID: number,
): Promise<ScrumCard> {
  return executeScrumCardTicketAction(cardID, {
    action: "apply_elaboration",
    expected_updated_at: expectedUpdatedAt,
    elaboration,
  }, projectID);
}
