import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ScrumCardModalResponse } from "../../lib/scrum_types";
import { CardModalApp } from "./CardModalApp";

const initialContext: ScrumCardModalResponse = {
  card: {
    id: "card_1",
    title: "State rail",
    description: "Move only through server state.",
    column: "ready",
    checklist: [],
    ref_files: [],
    chat: [],
    tags: [],
    test_criteria: [],
    created_at: "2026-08-13T12:00:00Z",
    updated_at: "2026-08-13T12:00:00.123456Z",
  },
  board: {
    id: "board_1",
    name: "Board",
    project_directory: "",
    columns: ["ready", "in_progress", "done"],
    cards: [],
    updated_at: "2026-08-13T12:00:00Z",
  },
  tab: "card",
  project_id: 7,
};

function response(payload: unknown): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("card state code-owned actions", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it.each([
    {
      name: "move",
      trigger: async () => fireEvent.change(await screen.findByDisplayValue("ready"), { target: { value: "in_progress" } }),
      path: "/move",
      body: { column: "in_progress" },
      label: "Moving card...",
      column: "in_progress",
    },
    {
      name: "done",
      trigger: async () => fireEvent.click(await screen.findByRole("button", { name: "Done" })),
      path: "/done",
      body: {},
      label: "Marking done...",
      column: "done",
    },
  ])("shows animated $name progress and reconciles only the returned card", async ({ trigger, path, body, label, column }) => {
    let current = initialContext;
    let finish!: () => void;
    const mutation = new Promise<Response>((resolve) => {
      finish = () => {
        current = {
          ...current,
          card: { ...current.card, column, updated_at: "2026-08-13T12:00:01.654321Z" },
        };
        resolve(response({ card: current.card }));
      };
    });
    const requests: Array<{ url: string; body: unknown }> = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "POST" && url.includes(path)) {
        requests.push({ url, body: JSON.parse(String(init.body)) });
        return mutation;
      }
      if (url.includes("/tags")) return response({ tags: [] });
      return response(current);
    }));

    render(<CardModalApp cardID="card_1" projectID={7} initialTab="card" />);
    await trigger();

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent(label);
    expect(status.querySelector(".animate-spin")).not.toBeNull();
    expect(requests).toEqual([{
      url: `/v1/scrum/cards/card_1${path}?project_id=7`,
      body,
    }]);

    finish();
    await waitFor(() => expect(document.querySelector("select")).toHaveValue(column));
    expect(screen.queryByText(label)).not.toBeInTheDocument();
  });
});
