import { afterEach, describe, expect, it, vi } from "vitest";
import { authoritativeControlJobID, ChatJobsCoordinator, type ChatJobsHost } from "./chat_jobs_coordinator";

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

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

describe("ChatJobsCoordinator lifecycle retries", () => {
	it("rejects malformed bounded job state before applying server markup", async () => {
		vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({
			job: { id: 42, status: "running", current_generation: 2 },
			steps: [{ id: 9, action: "v3_coding", status: "running", generation: 1 }],
			progress: { latest_context_id: 1, count: 25 },
			html: { bundle: "must-not-render" },
		})));
		const host = chatJobsTestHost();
		const coordinator = new ChatJobsCoordinator(host);
		const button = document.createElement("button");
		button.dataset.jobId = "42";

		await expect(coordinator.select({ currentTarget: button } as unknown as Event))
			.rejects.toThrow(/current generation|bounded progress/i);
		expect(host.renderComponentBundle).not.toHaveBeenCalled();
		expect(host.setCurrentJobID).not.toHaveBeenCalled();
	});

	it("submits one replan operation per interaction", async () => {
		const controlRequests: RequestInit[] = [];
		const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
			if (input === "/v1/jobs/42/replan") {
				if (!init) throw new Error("Replan request options are required.");
				controlRequests.push(init);
				return jsonResponse({ job: { id: 42 } });
			}
			if (input === "/v1/ui/chat/jobs/42") return jsonResponse({
				job: { id: 42, status: "running", current_generation: 1 }, steps: [],
				progress: { latest_context_id: 0, count: 0 }, html: { bundle: "server-job-details" },
			});
			throw new Error(`Unexpected request ${input}`);
		});
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("  use the corrected plan  ");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button");
		button.dataset.jobId = "42";

		await coordinator.replan({ currentTarget: button } as unknown as Event);

		expect(controlRequests).toHaveLength(1);
		const request = JSON.parse(String(controlRequests[0]?.body));
		expect(request.feedback).toBe("use the corrected plan");
		expect(request.operation_id).toMatch(/^lifecycle_operation_[0-9a-f]{64}$/);
	});

	it("reuses the cancellation identity after an ambiguous response", async () => {
		const cancelRequests: RequestInit[] = [];
		let cancelCalls = 0;
		const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
			if (input === "/v1/jobs/42/cancel") {
				if (!init) throw new Error("Cancellation request options are required.");
				cancelRequests.push(init);
				cancelCalls++;
				if (cancelCalls === 1) throw new Error("response lost after commit");
				return jsonResponse({ job: { id: 42 } });
			}
			if (input === "/v1/ui/chat/jobs?limit=20&offset=0") {
				return jsonResponse({ has_more: false, html: { bundle: "server-job-list" } });
			}
			throw new Error(`Unexpected request ${input}`);
		});
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("  deliberate cancellation  ");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button");
		button.dataset.jobId = "42";
		const event = { currentTarget: button } as unknown as Event;

		await expect(coordinator.cancel(event)).rejects.toThrow(/response lost after commit/i);
		await coordinator.cancel(event);

		expect(cancelRequests).toHaveLength(2);
		const first = JSON.parse(String(cancelRequests[0]?.body));
		const second = JSON.parse(String(cancelRequests[1]?.body));
		expect(second.operation_id).toBe(first.operation_id);
		expect(second.reason).toBe("deliberate cancellation");
	});
});

function chatJobsTestHost(): ChatJobsHost {
	const filter = document.createElement("select");
	return {
		queueEnabled: () => true,
		jobFilter: () => filter,
		hasJobBadge: () => false,
		jobBadge: () => document.createElement("div"),
		setCurrentJobID: vi.fn(),
		renderComponentBundle: vi.fn(async () => undefined),
		addEvent: vi.fn(),
	};
}

function jsonResponse(payload: unknown): Response {
	return new Response(JSON.stringify(payload), {
		status: 200,
		headers: { "Content-Type": "application/json" },
	});
}
