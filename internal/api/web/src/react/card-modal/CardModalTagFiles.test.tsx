import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SCRUM_COLUMNS, type ScrumCardModalResponse } from "../../lib/scrum_types";
import { CardModalApp } from "./CardModalApp";

const modalContext: ScrumCardModalResponse = {
  card: {
    id: "card_1", title: "React modal card", description: "Rendered by React", column: "assigned",
    checklist: [], ref_files: [], chat: [], tags: ["react"], test_criteria: [],
    channel_before_cursor: "", channel_has_more: false,
    created_at: "2026-06-13T12:00:00Z", updated_at: "2026-06-13T12:00:00Z",
  },
  board: {
    id: "board_1", name: "Board", project_directory: "", columns: [...SCRUM_COLUMNS], cards: [],
    updated_at: "2026-06-13T12:00:00Z",
  },
  tab: "card", project_id: 7,
  files: [], dirs: [], file_path: "", file_parent: "", file_has_parent: false,
  file_offset: 0, file_has_previous: false, file_previous_offset: 0,
  file_has_more: false, file_next_offset: 0,
  play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: [], queued_has_more: false },
  pilot_pending: false, channel_before_cursor: "", channel_has_more: false,
};

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "Content-Type": "application/json" } });
}

describe("card modal tag and file authority", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("shows a live error when the server-owned tag catalog cannot be loaded", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/tags?")) return jsonResponse({ error: "tag catalog failed" }, 503);
      return jsonResponse(modalContext);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<CardModalApp cardID="card_1" projectID={7} />);

    expect(await screen.findByRole("alert", {}, { timeout: 5_000 })).toHaveTextContent("Tag catalog unavailable: tag catalog failed");
    expect(fetchMock).toHaveBeenCalledWith("/v1/scrum/tags?project_id=7&limit=40");
  });

  it("sends exact user tag bytes and renders only the canonical server result", async () => {
    const patchBodies: unknown[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/tags?")) return jsonResponse({ tags: [] });
      if (init?.method === "PATCH") {
        patchBodies.push(JSON.parse(String(init.body)));
        return jsonResponse({
          card: { ...modalContext.card, tags: ["react", "api-client"], updated_at: "2026-06-13T12:01:00Z" },
        });
      }
      return jsonResponse(modalContext);
    }));

    render(<CardModalApp cardID="card_1" projectID={7} />);
    const input = await screen.findByPlaceholderText("Search or add tag");
    fireEvent.change(input, { target: { value: " API Client " } });
    const form = input.closest("form");
    if (!form) throw new Error("Tag form is unavailable.");
    fireEvent.submit(form);

    await waitFor(() => expect(patchBodies).toHaveLength(1));
    expect(patchBodies[0]).toMatchObject({ tags: ["react", " API Client "] });
    expect(await screen.findByRole("button", { name: "api-client ×" })).toBeInTheDocument();
  });

  it("loads bounded project file pages without a client inventory fallback", async () => {
    const first: ScrumCardModalResponse = {
      ...modalContext,
      tab: "files",
      files: ["pkg/first.go"], dirs: ["pkg/nested"],
      file_has_more: true, file_next_offset: 50,
    };
    const second = {
      files: ["pkg/second.go"], dirs: [], file_path: "", file_parent: "", file_has_parent: false,
      file_offset: 50, file_has_previous: true, file_previous_offset: 0,
      file_has_more: false, file_next_offset: 50,
    };
    const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
      jsonResponse(String(input).includes("file_offset=50") ? second : first));
    vi.stubGlobal("fetch", fetchMock);

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="files" />);

    expect(await screen.findByRole("option", { name: "pkg/first.go" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("option", { name: "pkg/second.go" })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/scrum/cards/card_1/files?project_id=7&file_path=&file_offset=50",
    );
  });
});
