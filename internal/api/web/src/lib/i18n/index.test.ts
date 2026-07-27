import { beforeEach, describe, expect, it } from "vitest";
import { initI18n, t, tf, type MessageKey } from ".";

describe("server-authoritative localization", () => {
  beforeEach(() => {
    document.documentElement.lang = "en";
    document.documentElement.dir = "ltr";
    initI18n();
  });

  it.each([
    ["es", "Nuevo hilo"],
    ["zh-Hans", "新对话"],
    ["ru", "Новая ветка"],
    ["ja", "新しいスレッド"],
  ] as const)("loads the complete %s catalog selected by the server", (locale, expected) => {
    document.documentElement.lang = locale;
    initI18n();
    expect(t("nav.newThread")).toBe(expected);
  });

  it("rejects an unsupported server-rendered locale", () => {
    document.documentElement.lang = "fr";
    expect(() => initI18n()).toThrow(/unsupported UI locale/);
  });

  it("rejects missing messages instead of falling back to English or the key", () => {
    expect(() => t("missing.message" as MessageKey)).toThrow(/has no message/);
  });

  it("requires every interpolation parameter", () => {
    expect(tf("ai.pausedQueued", { pending: 4 })).toBe("paused · 4 queued");
    expect(() => tf("ai.pausedQueued", {})).toThrow(/requires parameter/);
    expect(() => tf("ai.pausedQueued", { pending: 4, extra: 1 })).toThrow(/does not accept parameter/);
  });
});
