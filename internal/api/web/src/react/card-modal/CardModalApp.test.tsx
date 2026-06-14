import { Application } from "@hotwired/stimulus";
import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import CardModalSpaController from "../../controllers/card_modal_spa_controller";

const modalContext = {
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
});
