import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CardModalApp } from "./CardModalApp";
import { jsonResponse, makeModalContext } from "./CardModalTestSupport";

describe("card item exact byte transport", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it.each([
    ["checklist", "card", "Add checklist item", "checklist", "  Preserve checklist\tbytes  "],
    ["test criteria", "tests", "e.g. go test ./internal/api passes", "test-criteria", "  Preserve test\tbytes  "],
  ] as const)("preserves accepted %s bytes while using trim only for blank detection", async (_name, tab, placeholder, collection, text) => {
    const context = makeModalContext({ tab });
    const bodies: unknown[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/tags?")) return jsonResponse({ tags: [] });
      if (init?.method === "POST" && url.includes(`/${collection}?`)) {
        bodies.push(JSON.parse(String(init.body)));
        const item = { id: "item_1", text, done: false };
        return jsonResponse({
          card: {
            ...context.card,
            ...(collection === "checklist" ? { checklist: [item] } : { test_criteria: [item] }),
            updated_at: "2026-06-13T12:01:00Z",
          },
        });
      }
      return jsonResponse(context);
    }));

    render(<CardModalApp cardID="card_1" projectID={7} initialTab={tab} />);
    const input = await screen.findByPlaceholderText(placeholder);
    fireEvent.change(input, { target: { value: text } });
    const form = input.closest("form");
    if (!form) throw new Error("Card item form is unavailable.");
    fireEvent.submit(form);

    await waitFor(() => expect(bodies).toEqual([{
      action: "add",
      expected_updated_at: "2026-06-13T12:00:00Z",
      text,
    }]));
  });
});
