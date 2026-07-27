import { beforeEach, describe, expect, it, vi } from "vitest";
import { TranscriptStore } from "./transcript_store";

describe("TranscriptStore", () => {
  let values: Map<string, string>;

  beforeEach(() => {
    values = new Map();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      removeItem: (key: string) => { values.delete(key); },
    });
  });

  it("returns an empty transcript only when no transcript exists", () => {
    expect(new TranscriptStore().load()).toEqual([]);
  });

  it("round-trips a valid typed transcript", () => {
    const messages = [{ role: "user" as const, content: "Hello", at: "2026-01-01T00:00:00.000Z" }];
    const store = new TranscriptStore();

    store.save(messages);

    expect(store.load()).toEqual(messages);
  });

  it("fails loudly when stored JSON is corrupt", () => {
    localStorage.setItem("omni.chat.transcript.v1", "{not-json");

    expect(() => new TranscriptStore().load()).toThrow("Stored chat transcript is invalid JSON");
  });

  it("fails loudly when a stored message has an invalid role", () => {
    localStorage.setItem("omni.chat.transcript.v1", JSON.stringify([
      { role: "tool", content: "hidden", at: "2026-01-01T00:00:00.000Z" },
    ]));

    expect(() => new TranscriptStore().load()).toThrow("message 0 has an invalid role");
  });
});
