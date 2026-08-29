import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CardModalApp } from "./CardModalApp";
import { jsonResponse, makeModalContext, prepareCardModalDOM, realtimeCardUpdate } from "./CardModalTestSupport";

describe("card modal channel authority", () => {
  beforeEach(prepareCardModalDOM);
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("summarizes structured tool activity and keeps verbose output in details", async () => {
    const context = makeModalContext({ tab: "channel", card: { chat: [{
      id: "tool_1", role: "tool", created_at: "2026-06-13T12:00:00Z",
      content: JSON.stringify({
        activity: "output", title: "Test output", status: "completed", detail: "all tests passed",
      }),
    }] } });
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(context)));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);
    expect(await screen.findByText("Test output")).toBeInTheDocument();
    expect(screen.queryByText(/"activity":"output"/)).not.toBeInTheDocument();
    expect(screen.getByText("all tests passed")).not.toBeVisible();
    expect(screen.getByText("Details")).toBeInTheDocument();
  });

  it("preserves the reader's channel position when earlier activity is loaded", async () => {
    const context = makeModalContext({
      tab: "channel", channel_before_cursor: "scrumchat_v1_1", channel_has_more: true,
      card: { chat: [{ id: "current", role: "assistant", content: "current activity", created_at: "2026-06-13T12:00:00Z" }] },
    });
    let releasePage!: (response: Response) => void;
    const pageResponse = new Promise<Response>((resolve) => { releasePage = resolve; });
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) =>
      String(input).includes("/chat?") ? pageResponse : jsonResponse(context)));
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
      project_id: 7, card_id: "card_1", requested_before: "scrumchat_v1_1", limit: 50,
      messages: [{ id: "earlier", role: "assistant", content: "earlier activity", created_at: "2026-06-13T11:00:00Z" }],
      before_cursor: "", has_more: false, busy: false,
    }));
    expect(await screen.findByText("earlier activity")).toBeInTheDocument();
    await waitFor(() => expect(stream.scrollTop).toBe(400));
  });

  it("preserves exact nonblank channel message bytes at the typed HTTP boundary", async () => {
    const context = makeModalContext({ tab: "channel" });
    const bodies: unknown[] = [];
    vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        const body = JSON.parse(String(init.body));
        bodies.push(body);
        return jsonResponse({
          operation_id: body.operation_id,
          project_id: 7,
          card: {
            ...context.card, column: "in_progress", job_id: "41", play_state: "running",
            chat: [{ role: "user", content: body.message, created_at: "2026-06-13T12:00:00Z", operation_id: body.operation_id }],
          },
          action: "started",
        });
      }
      return jsonResponse(context);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);
    const input = await screen.findByPlaceholderText("Send revision to this job...");
    fireEvent.change(input, { target: { value: "  Preserve these exact bytes.  " } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({ message: "  Preserve these exact bytes.  " });
  });

  it("atomically replaces the latest channel window and pages its older boundary", async () => {
    const context = makeModalContext({ tab: "channel", card: { chat: [
      { id: "older", role: "assistant", content: "older visible activity", created_at: "2026-06-13T11:00:00Z" },
      { id: "current", role: "assistant", content: "current visible activity", created_at: "2026-06-13T12:00:00Z" },
    ] } });
    const requestURLs: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      requestURLs.push(url);
      if (url.includes("/chat?")) return jsonResponse({
        project_id: 7, card_id: "card_1", requested_before: "scrumchat_v1_1", limit: 50,
        messages: [{ id: "older", role: "assistant", content: "older visible activity", created_at: "2026-06-13T11:00:00Z" }],
        before_cursor: "", has_more: false, busy: false,
      });
      return jsonResponse(context);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);
    expect(await screen.findByText("older visible activity")).toBeInTheDocument();
    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", { detail: realtimeCardUpdate({
        ...context.card,
        chat: [
          { id: "current", role: "assistant", content: "current visible activity", created_at: "2026-06-13T12:00:00Z" },
          { id: "new", role: "assistant", content: "new live activity", created_at: "2026-06-13T12:01:00Z" },
        ],
        channel_before_cursor: "scrumchat_v1_1", channel_has_more: true,
      }) }));
    expect(await screen.findByText("new live activity")).toBeInTheDocument();
    expect(screen.queryByText("older visible activity")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Load earlier activity" }));
    expect(await screen.findByText("older visible activity")).toBeInTheDocument();
    expect(requestURLs.some((url) => url.includes("before=scrumchat_v1_1"))).toBe(true);
  });

  it("advances the earlier-history cursor from each authoritative realtime window", async () => {
    const context = makeModalContext({ tab: "channel" });
    const requestURLs: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      requestURLs.push(url);
      if (url.includes("/chat?")) return jsonResponse({
        project_id: 7, card_id: "card_1", requested_before: "scrumchat_v1_a", limit: 50,
        messages: [], before_cursor: "", has_more: false, busy: false,
      });
      return jsonResponse(context);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);
    await screen.findByText("No channel messages yet.");
    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", { detail: realtimeCardUpdate({
        ...context.card,
        chat: [{ id: "latest", role: "assistant", content: "latest activity", created_at: "2026-06-13T12:01:00Z" }],
        channel_before_cursor: "scrumchat_v1_a", channel_has_more: true,
      }) }));
    fireEvent.click(await screen.findByRole("button", { name: "Load earlier activity" }));
    await waitFor(() => expect(requestURLs.some((url) => url.includes("before=scrumchat_v1_a"))).toBe(true));
  });

  it("replaces a paged history window atomically before loading across a new realtime boundary", async () => {
    const message = (ordinal: number) => ({
      id: `message_${ordinal}`, role: "assistant", content: `activity ${ordinal}`,
      created_at: `2026-06-13T12:${String(ordinal % 60).padStart(2, "0")}:00Z`,
    });
    const context = makeModalContext({
      tab: "channel", channel_before_cursor: "scrumchat_v1_1e", channel_has_more: true,
      card: { chat: Array.from({ length: 50 }, (_, index) => message(index + 51)) },
    });
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("before=scrumchat_v1_1e")) return jsonResponse({
        project_id: 7, card_id: "card_1", requested_before: "scrumchat_v1_1e", limit: 50,
        messages: Array.from({ length: 50 }, (_, index) => message(index + 1)),
        before_cursor: "", has_more: false, busy: false,
      });
      if (url.includes("before=scrumchat_v1_24")) return jsonResponse({
        project_id: 7, card_id: "card_1", requested_before: "scrumchat_v1_24", limit: 50,
        messages: Array.from({ length: 50 }, (_, index) => message(index + 27)),
        before_cursor: "scrumchat_v1_q", has_more: true, busy: true,
      });
      return jsonResponse(context);
    }));
    render(<CardModalApp cardID="card_1" projectID={7} initialTab="channel" />);
    expect(await screen.findByText("activity 51")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Load earlier activity" }));
    expect(await screen.findByText("activity 1")).toBeInTheDocument();

    document.dispatchEvent(new CustomEvent("omni:scrum-card-updated", { detail: realtimeCardUpdate({
        ...context.card,
        chat: Array.from({ length: 25 }, (_, index) => message(index + 77)),
        channel_before_cursor: "scrumchat_v1_24", channel_has_more: true,
      }) }));
    expect(await screen.findByText("activity 101")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("activity 1")).not.toBeInTheDocument());
    expect(screen.queryByText("activity 51")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Load earlier activity" }));
    expect(await screen.findByText("activity 51")).toBeInTheDocument();
    expect(screen.getByText("activity 76")).toBeInTheDocument();
  });
});
