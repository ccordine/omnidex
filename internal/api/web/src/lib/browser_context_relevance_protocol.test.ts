import { describe, expect, it } from "vitest";
import {
  browserContextCompletionRequest,
  browserContextConfigSchema,
  browserContextFailure,
  browserContextJobSchema,
  browserContextProviderResult,
  browserContextResultSchema,
  browserContextSuccess,
  contextRelevanceStation,
  requireBrowserContextConfig,
  requireBrowserContextJob,
} from "./browser_context_relevance_protocol";

const model = "Llama-3.2-1B-Instruct-q4f16_1-MLC";

function jobFixture() {
  return {
    schema: browserContextJobSchema,
    job_id: "bcr_0123456789abcdef0123456789abcdef",
    station: contextRelevanceStation,
    model,
    prompt: "Return only the relevant opaque candidate IDs.",
    prompt_hint: "Return only the requested output.",
    max_output_tokens: 256,
  };
}

describe("browser context relevance protocol", () => {
  it("accepts only explicit provider config", () => {
    expect(requireBrowserContextConfig({
      schema: browserContextConfigSchema,
      enabled: true,
      station: contextRelevanceStation,
      model,
    }).model).toBe(model);
    expect(() => requireBrowserContextConfig({
      schema: browserContextConfigSchema,
      enabled: true,
      station: contextRelevanceStation,
      model,
      fallback: "server",
    })).toThrow(/unknown field fallback/);
  });

  it("keeps provider execution below the context_relevance station contract", () => {
    const job = requireBrowserContextJob(jobFixture(), model);
    const request = browserContextCompletionRequest(job);
    expect(request.messages).toEqual([
      { role: "system", content: job.prompt },
      { role: "user", content: job.prompt_hint },
    ]);
    expect(request.temperature).toBe(0);
    expect(request.max_tokens).toBe(256);
    expect(request).not.toHaveProperty("response_format");
    expect(request).not.toHaveProperty("tools");
  });

  it("rejects station and configured-model mismatches", () => {
    expect(() => requireBrowserContextJob({ ...jobFixture(), station: "browser_context_relevance" }, model)).toThrow();
    expect(() => requireBrowserContextJob(jobFixture(), "different-model")).toThrow();
    expect(() => requireBrowserContextJob({
      ...jobFixture(), response_schema: { type: "object" },
    }, model)).toThrow(/unknown field response_schema/);
  });

  it("returns only raw semantic bytes or one bounded failure", () => {
    const job = requireBrowserContextJob(jobFixture(), model);
    expect(browserContextSuccess(job, "candidate_1")).toEqual({
      schema: browserContextResultSchema,
      job_id: job.job_id,
      model,
      raw_result: "candidate_1",
    });
    const failure = browserContextFailure(job, new Error("🔥".repeat(2_000)));
    expect(failure.raw_result).toBeUndefined();
    expect(new TextEncoder().encode(failure.error).byteLength).toBeLessThanOrEqual(2 * 1024);
  });

  it("removes only WebLLM's exact empty thinking envelope", () => {
    const result = "candidate_1";
    expect(browserContextProviderResult(`<think>\n\n</think>\n\n${result}`)).toBe(result);
    expect(browserContextProviderResult(`<think>reasoning</think>${result}`)).toBe(
      `<think>reasoning</think>${result}`,
    );
    expect(browserContextProviderResult(result)).toBe(result);
  });
});
