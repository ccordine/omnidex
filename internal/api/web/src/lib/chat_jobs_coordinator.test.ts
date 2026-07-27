import { describe, expect, it } from "vitest";
import { authoritativeControlJobID } from "./chat_jobs_coordinator";

describe("authoritativeControlJobID", () => {
  it("follows the server-returned successor job", () => {
    expect(authoritativeControlJobID({ job: { id: 42 } })).toBe("42");
  });

  it("rejects missing and malformed authority instead of watching the old job", () => {
    for (const payload of [{}, { job: {} }, { job: { id: 0 } }, { job: { id: "42" } }]) {
      expect(() => authoritativeControlJobID(payload)).toThrow(/authoritative job/i);
    }
  });
});
