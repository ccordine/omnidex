import { afterEach, describe, expect, it, vi } from "vitest";
import { newLifecycleOperationID } from "./lifecycle_operation";
import { chatScrumCard } from "./scrum_api";

describe("chatScrumCard", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("sends one explicit lifecycle identity with the authoritative message", async () => {
    const operationID = newLifecycleOperationID();
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response(JSON.stringify({ card: { id: "card-7" }, reply: "" }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await chatScrumCard("card-7", "Continue once.", operationID, 14);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const call = fetchMock.mock.calls[0];
    expect(call).toBeDefined();
    if (!call) {
      throw new Error("Expected chatScrumCard to issue one fetch call.");
    }
    const [url, init] = call;
    expect(init).toBeDefined();
    if (!init) {
      throw new Error("Expected chatScrumCard to provide request options.");
    }
    expect(url).toBe("/v1/scrum/cards/card-7/chat?project_id=14");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      operation_id: operationID,
      message: "Continue once.",
    });
  });
});
