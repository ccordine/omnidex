import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchScrumTags } from "./scrum_api";

describe("fetchScrumTags", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("preserves the exact bounded search and emits one canonical limit", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ tags: ["api"] }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchScrumTags(" API client ", 14, 40)).resolves.toEqual(["api"]);
    expect(fetchMock).toHaveBeenCalledWith("/v1/scrum/tags?project_id=14&q=+API+client+&limit=40");
  });

  it.each([
    ["missing project", "api", null, 40],
    ["fractional project", "api", 1.5, 40],
    ["zero limit", "api", 14, 0],
    ["fractional limit", "api", 14, 1.5],
    ["large limit", "api", 14, 101],
    ["NUL search", "bad\0tag", 14, 40],
    ["oversized search", "x".repeat(257), 14, 40],
    ["unpaired surrogate", "bad\ud800tag", 14, 40],
  ] as const)("rejects %s before transport", async (_name, query, projectID, limit) => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchScrumTags(query, projectID, limit)).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([
    ["missing tags", {}],
    ["extra field", { tags: [], model: "forbidden" }],
    ["wrong type", { tags: "api" }],
    ["duplicate", { tags: ["api", "api"] }],
    ["unsorted", { tags: ["zeta", "alpha"] }],
    ["too many", { tags: ["a", "b"] }],
  ] as const)("rejects %s server state", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })));
    await expect(fetchScrumTags("", 14, 1)).rejects.toThrow();
  });
});
