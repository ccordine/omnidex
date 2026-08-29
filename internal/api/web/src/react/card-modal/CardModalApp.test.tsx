import { Application } from "@hotwired/stimulus";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import CardModalSpaController from "../../controllers/card_modal_spa_controller";
import { requestRealtimeSync } from "../../lib/realtime_sync";
import { CardModalApp } from "./CardModalApp";
import { jsonResponse, makeModalContext, modalContext, prepareCardModalDOM, realtimeCardUpdate } from "./CardModalTestSupport";

describe("card modal React SPA", () => {
  let application: Application | null = null;

  beforeEach(() => {
    prepareCardModalDOM();
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(modalContext)));
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
    await waitFor(() => expect(fetch).toHaveBeenCalledWith("/v1/scrum/cards/card_1/modal?project_id=7&tab=card"));
  });

  it("applies the single typed realtime card event without refetching modal context", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
      jsonResponse(String(input).includes("/tags?") ? { tags: [] } : modalContext));
    vi.stubGlobal("fetch", fetchMock);
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByText("React modal card")).toBeInTheDocument();
    fetchMock.mockClear();
    const boardRefresh = vi.fn();
    document.addEventListener("omni:scrum-refresh", boardRefresh, { once: true });
    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", { detail: realtimeCardUpdate({
      ...modalContext.card, title: "Live typed card", updated_at: "2026-06-13T12:01:00Z",
    }) }));
    expect(await screen.findByText("Live typed card")).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalledWith("/v1/scrum/cards/card_1/modal?project_id=7&tab=card");
    expect(boardRefresh).not.toHaveBeenCalled();
    document.removeEventListener("omni:scrum-refresh", boardRefresh);
  });

  it("rejects malformed realtime card authority and reconciles from the server", async () => {
    const reconciled = makeModalContext({ card: { title: "Reconciled after invalid live state" } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(modalContext))
      .mockResolvedValueOnce(jsonResponse(reconciled));
    vi.stubGlobal("fetch", fetchMock);
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByText("React modal card")).toBeInTheDocument();

    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", { detail: {
      ...realtimeCardUpdate({ ...modalContext.card, title: "Untrusted title" }),
      mutation: "model-selected",
    } }));

    expect(await screen.findByText("Reconciled after invalid live state")).toBeInTheDocument();
    expect(screen.getByText(/unknown field.*Authoritative card state was reloaded/)).toBeInTheDocument();
    expect(screen.queryByText("Untrusted title")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("reloads authoritative modal state when the realtime stream requires synchronization", async () => {
    const refreshed = makeModalContext({ card: { title: "Server reconciled card" } });
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(modalContext)).mockResolvedValueOnce(jsonResponse(refreshed));
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

  it("reloads authoritative modal and board state after a mutation response is lost", async () => {
    const committed = makeModalContext({ card: { title: "Committed server title", updated_at: "2026-06-13T12:02:00Z" } });
    let modalReads = 0;
    const refreshDetails: unknown[] = [];
    document.addEventListener("omni:scrum-refresh", (event) => refreshDetails.push((event as CustomEvent).detail));
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes("/tags")) return jsonResponse({ tags: [] });
      if (init?.method === "PATCH") throw new TypeError("response connection lost");
      modalReads += 1;
      return jsonResponse(modalReads === 1 ? modalContext : committed);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByDisplayValue("React modal card")).toBeInTheDocument();
    fireEvent.change(screen.getByDisplayValue("React modal card"), { target: { value: "Submitted title" } });
    fireEvent.click(screen.getByRole("button", { name: "Save details" }));
    expect(await screen.findByText(/response connection lost.*Authoritative card state was reloaded/)).toBeInTheDocument();
    expect(await screen.findByDisplayValue("Committed server title")).toBeInTheDocument();
    expect(refreshDetails).toContainEqual({ project_id: 7 });
    expect(modalReads).toBe(2);
  });

  it("locks every modal mutation control until the authoritative response settles", async () => {
    let releasePatch!: (response: Response) => void;
    const pendingPatch = new Promise<Response>((resolve) => { releasePatch = resolve; });
    let patchRequests = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes("/tags")) return jsonResponse({ tags: [] });
      if (init?.method === "PATCH") {
        patchRequests += 1;
        return pendingPatch;
      }
      return jsonResponse(modalContext);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} />);
    const save = await screen.findByRole("button", { name: "Save details" });
    fireEvent.click(save);
    await waitFor(() => expect(save).toBeDisabled());
    expect(screen.getByRole("button", { name: "Done" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Files" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
    fireEvent.click(save);
    expect(patchRequests).toBe(1);
    releasePatch(jsonResponse({ card: { ...modalContext.card, updated_at: "2026-06-13T12:01:00Z" } }));
    await waitFor(() => expect(save).not.toBeDisabled());
    expect(patchRequests).toBe(1);
  });

  it("reloads authoritative state after a stale revision conflict", async () => {
    const newer = makeModalContext({ card: { title: "Newer server title", updated_at: "2026-06-13T12:03:00Z" } });
    let modalReads = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes("/tags")) return jsonResponse({ tags: [] });
      if (init?.method === "PATCH") return jsonResponse({ error: "Scrum card changed; reload and retry" }, 409);
      modalReads += 1;
      return jsonResponse(modalReads === 1 ? modalContext : newer);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByDisplayValue("React modal card")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save details" }));
    expect(await screen.findByText(/Scrum card changed.*Authoritative card state was reloaded/)).toBeInTheDocument();
    expect(await screen.findByDisplayValue("Newer server title")).toBeInTheDocument();
    expect(modalReads).toBe(2);
  });

  it("applies and reports a committed-degraded card move without presenting full success", async () => {
    const moved = {
      ...modalContext.card, title: "Moved authoritative card", column: "done", updated_at: "2026-06-13T12:04:00Z",
    };
    const refreshDetails: unknown[] = [];
    document.addEventListener("omni:scrum-refresh", (event) => refreshDetails.push((event as CustomEvent).detail));
    vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
      init?.method === "POST"
        ? jsonResponse({
          commit_state: "committed_degraded", card: moved,
          operation_error: "Card moved, but play-queue reconciliation failed.",
        }, 207)
        : jsonResponse(modalContext)));
    render(<CardModalApp cardID="card_1" projectID={7} />);
    expect(await screen.findByText("React modal card")).toBeInTheDocument();
    fireEvent.change(screen.getByDisplayValue("assigned"), { target: { value: "done" } });
    expect(await screen.findByText("Moved authoritative card")).toBeInTheDocument();
    expect(screen.getByText("Card moved, but play-queue reconciliation failed.")).toBeInTheDocument();
    expect(screen.queryByText("Moving card complete")).not.toBeInTheDocument();
    expect(refreshDetails).toContainEqual({ project_id: 7 });
  });

  it("does not expose the retired card-level model configuration control", () => {
    expect(() => render(<CardModalApp cardID="card_1" projectID={7} initialTab="config" />)).toThrow(/exact registered value/);
    expect(fetch).not.toHaveBeenCalled();
  });

  it("plays paused cards instead of sending another pause request", async () => {
    const context = makeModalContext({ card: { play_state: "paused" } });
    const requestURLs: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      requestURLs.push(`${init?.method ?? "GET"} ${url}`);
      if (init?.method === "POST" && url.includes("/play")) {
        const card = { ...context.card, column: "in_progress", job_id: "41", play_state: "queued", updated_at: "2026-06-13T12:01:00Z" };
        return jsonResponse({ card, job_id: "41", column: "in_progress", message: "queued" });
      }
      if (init?.method === "POST" && url.includes("/pause")) return jsonResponse({ error: "only active cards can be paused" }, 400);
      return jsonResponse(context);
    }));
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
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      requestURLs.push(`${init?.method ?? "GET"} ${url}`);
      return init?.method === "DELETE" ? jsonResponse({
        commit_state: "committed",
        project_id: 7,
        card_id: "card_1",
        expected_updated_at: context.card.updated_at,
        deleted: true,
      }) : jsonResponse(context);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="card" />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await waitFor(() => expect(requestURLs).toContain("DELETE /v1/scrum/cards/card_1?project_id=7"));
    await waitFor(() => expect(refreshDetails).toEqual([{ project_id: 7 }]));
  });
});
