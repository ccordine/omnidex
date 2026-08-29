import { afterEach, describe, expect, it, vi } from "vitest";
import { authoritativeControlJobID, ChatJobsCoordinator, type ChatJobsHost } from "./chat_jobs_coordinator";

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe("authoritativeControlJobID", () => {
	it("accepts only the job the user controlled", () => {
		const operationID = `lifecycle_operation_${"a".repeat(64)}`;
		expect(authoritativeControlJobID({ job_id: 42, operation_id: operationID, status: "running" }, "42", operationID)).toBe("42");
		expect(() => authoritativeControlJobID({ job_id: 43, operation_id: operationID, status: "running" }, "42", operationID)).toThrow(/expected job 42/i);
	});

	it("rejects missing and malformed authority instead of watching the old job", () => {
		const operationID = `lifecycle_operation_${"a".repeat(64)}`;
		for (const payload of [
			{},
			{ job_id: 42, operation_id: operationID, status: "running", job: {} },
			{ job_id: 0, operation_id: operationID, status: "running" },
			{ job_id: 42, operation_id: `lifecycle_operation_${"b".repeat(64)}`, status: "running" },
			{ job_id: 42, operation_id: operationID, status: "invented" },
		]) {
			expect(() => authoritativeControlJobID(payload, "42", operationID)).toThrow();
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
				const body = JSON.parse(String(init.body));
				return jsonResponse({ job_id: 42, operation_id: body.operation_id, status: "running" });
			}
			if (input === "/v1/ui/chat/jobs/42") return jsonResponse({
				job: { id: 42, status: "running", current_generation: 1 }, steps: [],
				progress: { latest_context_id: 0, count: 0 }, html: { bundle: "server-job-details" },
			});
			throw new Error(`Unexpected request ${input}`);
		});
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("  use the corrected plan\nwith trailing space  ");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button");
		button.dataset.jobId = "42";
		button.textContent = "Replan";

		await coordinator.replan({ currentTarget: button } as unknown as Event);

		expect(controlRequests).toHaveLength(1);
		const request = JSON.parse(String(controlRequests[0]?.body));
		expect(request.feedback).toBe("  use the corrected plan\nwith trailing space  ");
		expect(request.operation_id).toMatch(/^lifecycle_operation_[0-9a-f]{64}$/);
		expect(button.disabled).toBe(false);
		expect(button.getAttribute("aria-busy")).toBe("false");
		expect(button.textContent).toBe("Replan");
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
				const body = JSON.parse(String(init.body));
				return jsonResponse({ job_id: 42, operation_id: body.operation_id, status: "canceled" });
			}
			if (input === "/v1/ui/chat/jobs?limit=20&offset=0") {
				return jsonResponse({ has_more: false, html: { bundle: "server-job-list" } });
			}
			throw new Error(`Unexpected request ${input}`);
		});
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("  deliberate cancellation\n  ");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button");
		button.dataset.jobId = "42";
		button.textContent = "Cancel";
		const event = { currentTarget: button } as unknown as Event;

		await expect(coordinator.cancel(event)).rejects.toThrow(/response lost after commit/i);
		await coordinator.cancel(event);

		expect(cancelRequests).toHaveLength(2);
		const first = JSON.parse(String(cancelRequests[0]?.body));
		const second = JSON.parse(String(cancelRequests[1]?.body));
		expect(second.operation_id).toBe(first.operation_id);
		expect(second.reason).toBe("  deliberate cancellation\n  ");
		expect(button.disabled).toBe(false);
		expect(button.getAttribute("aria-busy")).toBe("false");
		expect(button.textContent).toBe("Cancel");
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
