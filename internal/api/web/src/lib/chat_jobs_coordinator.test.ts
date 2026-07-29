import { describe, expect, it } from "vitest";
import { authoritativeControlJobID } from "./chat_jobs_coordinator";

describe("authoritativeControlJobID", () => {
	it("accepts only the job the user controlled", () => {
		expect(authoritativeControlJobID({ job: { id: 42 } }, "42")).toBe("42");
		expect(() => authoritativeControlJobID({ job: { id: 43 } }, "42")).toThrow(/expected job 42/i);
	});

	it("rejects missing and malformed authority instead of watching the old job", () => {
		for (const payload of [{}, { job: {} }, { job: { id: 0 } }, { job: { id: "42" } }]) {
			expect(() => authoritativeControlJobID(payload, "42")).toThrow(/authoritative job/i);
		}
	});
});
