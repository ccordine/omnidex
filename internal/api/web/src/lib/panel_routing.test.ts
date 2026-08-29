import { describe, expect, it } from "vitest";
import { parseScrumTabFromLocation } from "./panel_routing";

describe("Scrum modal route authority", () => {
  it("defaults to card only when the tab is omitted", () => {
    expect(parseScrumTabFromLocation({ search: "?panel=projects" })).toBe("card");
    expect(parseScrumTabFromLocation({ search: "?panel=projects&scrum_tab=files" })).toBe("files");
  });

  it.each([
    "?panel=projects&scrum_tab=",
    "?panel=projects&scrum_tab=FILES",
    "?panel=projects&scrum_tab=%20files",
    "?panel=projects&scrum_tab=config",
    "?panel=projects&scrum_tab=card&scrum_tab=files",
  ])("rejects an explicit invalid or ambiguous tab in %s", (search) => {
    expect(() => parseScrumTabFromLocation({ search })).toThrow(/Scrum card modal tab/);
  });
});
