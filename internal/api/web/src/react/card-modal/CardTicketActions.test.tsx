import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SCRUM_COLUMNS, type ScrumCardModalResponse } from "../../lib/scrum_types";
import { CardModalApp } from "./CardModalApp";

const context: ScrumCardModalResponse = {
  card: {
    id: "card_1",
    title: "Ticket rail",
    description: "Assemble exact card state.",
    column: "ready",
    checklist: [],
    ref_files: [],
    chat: [],
    tags: [],
    test_criteria: [],
    channel_before_cursor: "",
    channel_has_more: false,
    card_ticket: "",
    card_prompt: "Legacy display-only prompt.",
    created_at: "2026-08-13T12:00:00Z",
    updated_at: "2026-08-13T12:00:00.123456Z",
  },
  board: {
    id: "board_1",
    name: "Board",
    project_directory: "",
    columns: [...SCRUM_COLUMNS],
    cards: [],
    updated_at: "2026-08-13T12:00:00Z",
  },
  tab: "card",
  project_id: 7,
  files: [],
  dirs: [],
  file_path: "",
  file_parent: "",
  file_has_parent: false,
  file_offset: 0,
  file_has_previous: false,
  file_previous_offset: 0,
  file_has_more: false,
  file_next_offset: 0,
  play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: [], queued_has_more: false },
  pilot_pending: false,
  channel_before_cursor: "",
  channel_has_more: false,
};

function response(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("card ticket code-owned actions", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("shows animated assembly progress and reconciles the returned server card", async () => {
    let finishAssembly!: (value: Response) => void;
		const assembleResponse = new Promise<Response>((resolve) => {
      finishAssembly = resolve;
    });
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
			if (init?.method === "POST" && url.includes("/card-ticket")) return assembleResponse;
      if (url.includes("/tags")) return response({ tags: [] });
      return response(context);
    }));

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="card" />);
    fireEvent.click(await screen.findByRole("button", { name: "Assemble ticket" }));

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("Assembling ticket...");
    expect(status.querySelector(".animate-spin")).not.toBeNull();

    finishAssembly(response({
      card: {
        ...context.card,
        card_ticket: "# Ticket rail\n\n## Objective\n\nAssemble exact card state.\n",
        updated_at: "2026-08-13T12:00:01.654321Z",
      },
    }));
    await waitFor(() => {
      expect(screen.getByPlaceholderText("Ticket details")).toHaveValue("# Ticket rail\n\n## Objective\n\nAssemble exact card state.\n");
    });
    expect(screen.queryByText("Assembling ticket...")).not.toBeInTheDocument();
  });

  it("submits explicit elaboration and reconciles its canonical ticket", async () => {
    const requests: Array<{ url: string; body: unknown }> = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "POST" && url.includes("/card-ticket")) {
        requests.push({ url, body: JSON.parse(String(init.body)) });
        return response({
          card: {
            ...context.card,
			card_prompt: "  Retain typed authority.\nKeep tab:\t ",
			card_ticket: "# Ticket rail\n\n## Elaboration\n\n  Retain typed authority.\nKeep tab:\t \n",
            updated_at: "2026-08-13T12:00:02Z",
          },
        });
      }
      if (url.includes("/tags")) return response({ tags: [] });
      return response(context);
    }));

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="card" />);
    expect(await screen.findByPlaceholderText("Manual ticket note")).toHaveValue("Legacy display-only prompt.");
    const elaboration = await screen.findByPlaceholderText("User-authored elaboration");
    expect(elaboration).toHaveValue("");
		fireEvent.change(elaboration, { target: { value: "  Retain typed authority.\nKeep tab:\t " } });
    fireEvent.click(screen.getByRole("button", { name: "Apply elaboration" }));

    await waitFor(() => expect(requests).toHaveLength(1));
    expect(requests[0]).toEqual({
      url: "/v1/scrum/cards/card_1/card-ticket?project_id=7",
      body: {
        action: "apply_elaboration",
        expected_updated_at: "2026-08-13T12:00:00.123456Z",
				elaboration: "  Retain typed authority.\nKeep tab:\t ",
      },
    });
    await waitFor(() => {
		expect(screen.getByPlaceholderText("Ticket details")).toHaveValue("# Ticket rail\n\n## Elaboration\n\n  Retain typed authority.\nKeep tab:\t \n");
		expect(screen.getByPlaceholderText("Manual ticket note")).toHaveValue("  Retain typed authority.\nKeep tab:\t ");
      expect(screen.getByPlaceholderText("User-authored elaboration")).toHaveValue("");
    });
  });
});
