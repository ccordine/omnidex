import { describe, expect, it } from "vitest";
import { HTTPResponseError, readJSON } from "./api";

describe("exact JSON response transport", () => {
  it("rejects a second JSON value after the authoritative response", async () => {
    const response = new Response('{"ok":true} {"fallback":true}', {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });

    await expect(readJSON(response)).rejects.toThrow(/trailing data/);
  });

  it("rejects a declared response larger than the caller's transport bound", async () => {
    const response = new Response('{"ok":true}', {
      status: 200,
      headers: { "Content-Type": "application/json", "Content-Length": "12" },
    });
    await expect(readJSON(response, 11)).rejects.toThrow(/11-byte transport bound/);
  });

  it("rejects a streamed response that crosses the caller's transport bound", async () => {
    const response = new Response(new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('{"value":"'));
        controller.enqueue(new TextEncoder().encode('too large"}'));
        controller.close();
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } });
    await expect(readJSON(response, 12)).rejects.toThrow(/12-byte transport bound/);
  });

  it("rejects invalid UTF-8 before JSON parsing", async () => {
    const response = new Response(new Uint8Array([0x7b, 0x22, 0x78, 0x22, 0x3a, 0xff, 0x7d]), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
    await expect(readJSON(response)).rejects.toThrow(/not valid UTF-8/);
  });

  it("preserves typed HTTP status authority for reconciliation", async () => {
    const response = new Response('{"error":"stale authority"}', {
      status: 409,
      headers: { "Content-Type": "application/json" },
    });
    let observed: unknown;
    try {
      await readJSON(response);
    } catch (error) {
      observed = error;
    }
    expect(observed).toBeInstanceOf(HTTPResponseError);
    expect((observed as HTTPResponseError).status).toBe(409);
    expect((observed as Error).message).toBe("stale authority");
  });
});
