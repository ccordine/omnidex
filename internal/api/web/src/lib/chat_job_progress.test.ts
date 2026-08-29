import { beforeEach, describe, expect, it } from "vitest";
import {
  describeChatJobProgress,
  describeJobStatus,
  describeRealtimeJobPhase,
} from "./chat_job_progress";
import { initI18n } from "./i18n";

describe("typed chat job descriptions", () => {
  beforeEach(() => {
    document.documentElement.lang = "en";
    document.documentElement.dir = "ltr";
    initI18n();
  });

  it.each([
    ["es", "Escribiendo código… (#9)"],
    ["zh-Hans", "正在编写代码… (#9)"],
    ["ru", "Написание кода… (#9)"],
    ["ja", "コードを作成中… (#9)"],
  ] as const)("localizes live coding progress in %s", (locale, expected) => {
    document.documentElement.lang = locale;
    initI18n();
    expect(describeChatJobProgress({
      job: { id: 9, status: "running", current_generation: 1 },
      steps: [{ id: 3, action: "v3_coding", status: "running" }],
    })).toBe(expected);
  });

  it("rejects unknown server states instead of displaying guessed labels", () => {
    expect(() => describeJobStatus("maybe")).toThrow('Unsupported job status "maybe"');
    expect(() => describeRealtimeJobPhase("maybe")).toThrow('Unsupported realtime job phase "maybe"');
  });
});
