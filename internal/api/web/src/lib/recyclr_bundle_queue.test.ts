import { describe, expect, it } from "vitest";
import { parseRecyclrBundle } from "./recyclr_bundle_queue";

describe("server Recyclr bundle boundary", () => {
  it("maps one exact top-level server template to its registered sink", () => {
    expect(parseRecyclrBundle('<template data-recyclr-target="projects-list"><p>Ready</p></template>')).toEqual([{
      selector: '[data-recyclr-sink="projects-list"]',
      location: "innerHTML",
      selection: "<p>Ready</p>",
    }]);
  });

  it.each([
    ["ordinary markup", "<div>not a bundle</div>"],
    ["nested target", '<div><template data-recyclr-target="projects-list">bad</template></div>'],
    ["unbounded target", '<template data-recyclr-target="../../body">bad</template>'],
    ["unsupported location", '<template data-recyclr-target="projects-list" data-recyclr-location="replaceAll">bad</template>'],
  ])("rejects %s", (_label, bundle) => {
    expect(() => parseRecyclrBundle(bundle)).toThrow();
  });
});
