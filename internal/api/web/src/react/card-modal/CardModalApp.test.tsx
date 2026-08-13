import { Application } from "@hotwired/stimulus";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import CardModalSpaController from "../../controllers/card_modal_spa_controller";
import type { ScrumCardModalResponse } from "../../lib/scrum_types";
import { requestRealtimeSync } from "../../lib/realtime_sync";
import { CardModalApp } from "./CardModalApp";

type ModalContextOverrides = Partial<Omit<ScrumCardModalResponse, "card" | "board">> & {
  card?: Partial<ScrumCardModalResponse["card"]>;
  board?: Partial<ScrumCardModalResponse["board"]>;
};

const modalContext: ScrumCardModalResponse = {
  card: {
    id: "card_1",
    title: "React modal card",
    description: "Rendered by React",
    column: "assigned",
    checklist: [],
    ref_files: [],
    chat: [],
    tags: ["react"],
    test_criteria: [],
    created_at: "2026-06-13T12:00:00Z",
    updated_at: "2026-06-13T12:00:00Z",
  },
  board: {
    id: "board_1",
    name: "Board",
    project_directory: "",
    columns: ["backlog", "assigned", "done"],
    cards: [],
    updated_at: "2026-06-13T12:00:00Z",
  },
  tab: "card",
  project_id: 7,
  files: [],
  dirs: [],
  play_queue: { queued_count: 0, queued_card_ids: [] },
  model_fields: [],
  model_overrides: {},
  agent_fields: [],
  agent_overrides: {},
  recipes: [],
  project_recipe: {},
};

function makeModalContext(overrides: ModalContextOverrides = {}): ScrumCardModalResponse {
  const { card, board, ...rest } = overrides;
  return {
    ...modalContext,
    ...rest,
    card: { ...modalContext.card, ...card },
    board: { ...modalContext.board, ...board },
  };
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "Content-Type": "application/json" } });
}

describe("card modal React SPA", () => {
  let application: Application | null = null;

  beforeEach(() => {
    document.body.innerHTML = `<div data-controller="card-modal-spa" data-card-modal-spa-card-id-value="card_1" data-card-modal-spa-project-id-value="7" data-card-modal-spa-initial-tab-value="card"></div>`;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify(modalContext), { status: 200, headers: { "Content-Type": "application/json" } })),
    );
  });

  afterEach(async () => {
    document.querySelector('[data-controller="card-modal-spa"]')?.remove();
    await Promise.resolve();
    application?.stop();
    application = null;
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("mounts from the Stimulus controller and renders card context", async () => {
    application = Application.start();
    application.register("card-modal-spa", CardModalSpaController);

    expect(await screen.findByText("React modal card")).toBeInTheDocument();
    expect(screen.getByText("Rendered by React")).toBeInTheDocument();
    expect(document.querySelector("[data-recyclr-target]")).not.toBeInTheDocument();

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith("/v1/scrum/cards/card_1/modal?project_id=7&tab=card");
    });
  });

  it("summarizes structured tool activity and keeps verbose command output in details", async () => {
    const context = makeModalContext({
      tab: "channel",
      card: {
        chat: [{
          id: "tool_1",
          role: "tool",
          created_at: "2026-06-13T12:00:00Z",
          content: JSON.stringify({
            activity: "command",
            title: "Run · go test ./internal/api",
            status: "completed",
            command: "go test ./internal/api -count=1",
            detail: "all tests passed",
          }),
        }],
      },
    });
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(context)));

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);

    expect(await screen.findByText("Run · go test ./internal/api")).toBeInTheDocument();
    expect(screen.queryByText(/"activity":"command"/)).not.toBeInTheDocument();
    expect(screen.getByText("all tests passed")).not.toBeVisible();
    expect(screen.getByText("Details")).toBeInTheDocument();
  });

  it("preserves the reader's channel position when earlier activity is loaded", async () => {
    const context = makeModalContext({
      tab: "channel",
      channel_before_cursor: "cursor_50",
      channel_has_more: true,
      card: {
        chat: [{ id: "current", role: "assistant", content: "current activity", created_at: "2026-06-13T12:00:00Z" }],
      },
    });
    let releasePage!: (response: Response) => void;
    const pageResponse = new Promise<Response>((resolve) => {
      releasePage = resolve;
    });
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/chat?")) return pageResponse;
      return jsonResponse(context);
    }));

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);

    const current = await screen.findByText("current activity");
    const stream = current.closest(".scrollbar") as HTMLDivElement;
    let scrollHeight = 500;
    Object.defineProperty(stream, "scrollHeight", { configurable: true, get: () => scrollHeight });
    stream.scrollTop = 200;
    fireEvent.scroll(stream);
    fireEvent.click(screen.getByRole("button", { name: "Load earlier activity" }));
    scrollHeight = 700;
    releasePage(jsonResponse({
      messages: [{ id: "earlier", role: "assistant", content: "earlier activity", created_at: "2026-06-13T11:00:00Z" }],
      before_cursor: "cursor_1",
      has_more: false,
    }));

    expect(await screen.findByText("earlier activity")).toBeInTheDocument();
    await waitFor(() => expect(stream.scrollTop).toBe(400));
  });

  it("applies the single typed realtime card event without refetching modal context", async () => {
    const fetchMock = vi.fn(async () => jsonResponse(modalContext));
    vi.stubGlobal("fetch", fetchMock);
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByText("React modal card")).toBeInTheDocument();
    fetchMock.mockClear();
    const boardRefresh = vi.fn();
    document.addEventListener("omni:scrum-refresh", boardRefresh, { once: true });

    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", {
      detail: {
        cardID: "card_1",
        projectID: 7,
        reason: "agent output",
        card: { ...modalContext.card, title: "Live typed card", updated_at: "2026-06-13T12:01:00Z" },
      },
    }));

    expect(await screen.findByText("Live typed card")).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(boardRefresh).not.toHaveBeenCalled();
    document.removeEventListener("omni:scrum-refresh", boardRefresh);
  });

  it("retains visible channel history when a bounded realtime card snapshot arrives", async () => {
    const context = makeModalContext({
      tab: "channel",
      card: {
        chat: [
          { id: "older", role: "assistant", content: "older visible activity", created_at: "2026-06-13T11:00:00Z" },
          { id: "current", role: "assistant", content: "current visible activity", created_at: "2026-06-13T12:00:00Z" },
        ],
      },
    });
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(context)));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);
    expect(await screen.findByText("older visible activity")).toBeInTheDocument();

    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", {
      detail: {
        cardID: "card_1",
        projectID: 7,
        reason: "agent output",
        card: {
          ...context.card,
          chat: [
            { id: "current", role: "assistant", content: "current visible activity", created_at: "2026-06-13T12:00:00Z" },
            { id: "new", role: "assistant", content: "new live activity", created_at: "2026-06-13T12:01:00Z" },
          ],
        },
      },
    }));

    expect(await screen.findByText("new live activity")).toBeInTheDocument();
    expect(screen.getByText("older visible activity")).toBeInTheDocument();
  });

  it("reloads authoritative modal state when the realtime stream requires synchronization", async () => {
    const refreshed = makeModalContext({ card: { title: "Server reconciled card" } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(modalContext))
      .mockResolvedValueOnce(jsonResponse(refreshed));
    vi.stubGlobal("fetch", fetchMock);
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByText("React modal card")).toBeInTheDocument();

    await requestRealtimeSync("replay_gap", 42);

    expect(await screen.findByText("Server reconciled card")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("rejects realtime synchronization when authoritative modal state cannot be read", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(modalContext))
      .mockResolvedValueOnce(jsonResponse({ error: "modal synchronization failed" }, 503));
    vi.stubGlobal("fetch", fetchMock);
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByText("React modal card")).toBeInTheDocument();

    await expect(requestRealtimeSync("replay_gap", 43)).rejects.toThrow("modal synchronization failed");
    expect(await screen.findByText("modal synchronization failed")).toBeInTheDocument();
  });

  it("saves only explicit model overrides from the config tab", async () => {
    const context = makeModalContext({
      tab: "config",
      model_fields: [
        { key: "model", label: "Model", value: "gpt-inherited" },
        { key: "temperature", label: "Temperature", value: "0.2" },
      ],
      model_overrides: {},
    });
    const patchBodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "PATCH") {
          patchBodies.push(JSON.parse(String(init.body ?? "{}")));
          return jsonResponse({ card: { ...context.card, updated_at: "2026-06-13T12:01:00Z" } });
        }
        return jsonResponse(context);
      }),
    );

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="config" />);

    const modelInput = (await screen.findByLabelText("Model")) as HTMLInputElement;
    expect(modelInput).toHaveValue("");
    expect(modelInput).toHaveAttribute("placeholder", "gpt-inherited");

    fireEvent.change(screen.getByLabelText("Temperature"), { target: { value: "0.8" } });
    fireEvent.click(screen.getByRole("button", { name: "Save model" }));

    await waitFor(() => expect(patchBodies).toHaveLength(1));
    expect(patchBodies[0]).toEqual({ model_config: { temperature: "0.8" } });
  });

  it("does not expose generic card agent override controls", async () => {
    const context = makeModalContext({
      tab: "config",
      agent_system: "omnidex",
      agent_fields: [
        { key: "agent_system", label: "Agent system", options: ["omnidex", "codex", "cursor"], value: "omnidex" },
        { key: "agent_model", label: "Agent model", value: "gpt-inherited" },
        { key: "reasoning_effort", label: "Reasoning", value: "medium" },
      ],
      agent_overrides: {},
    });
    const patchBodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "PATCH") {
          patchBodies.push(JSON.parse(String(init.body ?? "{}")));
          return jsonResponse({ card: { ...context.card, updated_at: "2026-06-13T12:01:00Z" } });
        }
        return jsonResponse(context);
      }),
    );

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="config" />);

    await screen.findByText("Model Overrides");
    expect(screen.queryByText("Agent Overrides")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Agent model")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Use codex" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save agent" })).not.toBeInTheDocument();
    expect(patchBodies).toHaveLength(0);
  });

  it("plays paused cards instead of sending another pause request", async () => {
    const context = makeModalContext({
      card: { play_state: "paused" },
    });
    const requestURLs: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        requestURLs.push(`${init?.method ?? "GET"} ${url}`);
        if (init?.method === "POST" && url.includes("/play")) {
          return jsonResponse({ card: { ...context.card, play_state: "queued", updated_at: "2026-06-13T12:01:00Z" } });
        }
        if (init?.method === "POST" && url.includes("/pause")) {
          return jsonResponse({ error: "only active cards can be paused" }, 400);
        }
        return jsonResponse(context);
      }),
    );

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="card" />);

    const playButton = await screen.findByRole("button", { name: "Play" });
    expect(screen.queryByRole("button", { name: "Pause" })).not.toBeInTheDocument();
    fireEvent.click(playButton);

    await waitFor(() => expect(requestURLs).toContain("POST /v1/scrum/cards/card_1/play?project_id=7"));
    expect(requestURLs).not.toContain("POST /v1/scrum/cards/card_1/pause?project_id=7");
  });

  it("refreshes the board after a successful modal delete", async () => {
    const context = makeModalContext();
    const requestURLs: string[] = [];
    const refreshDetails: unknown[] = [];
    document.addEventListener("omni:scrum-refresh", (event) => refreshDetails.push((event as CustomEvent).detail), { once: true });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        requestURLs.push(`${init?.method ?? "GET"} ${url}`);
        if (init?.method === "DELETE") return jsonResponse({});
        return jsonResponse(context);
      }),
    );

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="card" />);

    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));

    await waitFor(() => expect(requestURLs).toContain("DELETE /v1/scrum/cards/card_1?project_id=7"));
    await waitFor(() => expect(refreshDetails).toEqual([{ project_id: 7 }]));
  });
});
