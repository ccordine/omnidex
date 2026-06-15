import { Application } from "@hotwired/stimulus";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import CardModalSpaController from "../../controllers/card_modal_spa_controller";
import type { ScrumCardModalResponse } from "../../lib/scrum_types";
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

  afterEach(() => {
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

  it("saves only explicit agent overrides when switching agent systems", async () => {
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

    const agentModelInput = (await screen.findByLabelText("Agent model")) as HTMLInputElement;
    expect(agentModelInput).toHaveValue("");
    expect(agentModelInput).toHaveAttribute("placeholder", "gpt-inherited");

    fireEvent.click(screen.getByRole("button", { name: "Use codex" }));

    await waitFor(() => expect(patchBodies).toHaveLength(1));
    expect(patchBodies[0]).toEqual({ agent_config: { agent_system: "codex", agent_strict: "true" } });
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
