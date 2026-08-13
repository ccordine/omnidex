import { afterEach, describe, expect, it, vi } from "vitest";
import { newLifecycleOperationID } from "./lifecycle_operation";
import { chatScrumCard, elaborateScrumCardTicket, generateScrumCardTicket } from "./scrum_api";

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

describe("Scrum card ticket actions", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("sends only the code-owned generate action and observed card revision", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response(JSON.stringify({ card: { id: "card-7", updated_at: "2026-08-13T12:01:00Z" } }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await generateScrumCardTicket("card-7", "2026-08-13T12:00:00Z", 14);

    const call = fetchMock.mock.calls[0];
    if (!call) throw new Error("Expected ticket generation to issue one request.");
    const [url, init] = call;
    expect(url).toBe("/v1/scrum/cards/card-7/card-ticket?project_id=14");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      action: "generate",
      expected_updated_at: "2026-08-13T12:00:00Z",
    });
  });

  it("sends explicit user-authored elaboration without legacy prompt fields", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response(JSON.stringify({ card: { id: "card-7", updated_at: "2026-08-13T12:01:00Z" } }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await elaborateScrumCardTicket("card-7", "2026-08-13T12:00:00Z", "Preserve exact state.", 14);

    const call = fetchMock.mock.calls[0];
    if (!call) throw new Error("Expected ticket elaboration to issue one request.");
    const [, init] = call;
    const body = JSON.parse(String(init?.body));
    expect(body).toEqual({
      action: "elaborate",
      expected_updated_at: "2026-08-13T12:00:00Z",
      elaboration: "Preserve exact state.",
    });
    expect(body).not.toHaveProperty("prompt");
    expect(body).not.toHaveProperty("iterate");
    expect(body).not.toHaveProperty("ticket");
  });
});
