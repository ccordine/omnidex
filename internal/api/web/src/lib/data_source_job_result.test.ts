import { describe, expect, it } from "vitest";
import { parseDataSourceJobResult } from "./data_source_job_result";

describe("parseDataSourceJobResult", () => {
  it("accepts the authoritative nested query result", () => {
    const result = parseDataSourceJobResult(JSON.stringify({
      query: {
        question: "How many?",
        answer: "Two",
        columns: ["count"],
        rows: [{ count: 2 }],
        count: 1,
      },
      job_id: 42,
    }));

    expect(result).toEqual({
      question: "How many?",
      answer: "Two",
      columns: ["count"],
      rows: [{ count: 2 }],
      count: 1,
    });
  });

  it("rejects the removed top-level result fallback and malformed output", () => {
    expect(() => parseDataSourceJobResult('{"columns":[],"rows":[],"count":0}')).toThrow(
      "required query object",
    );
    expect(() => parseDataSourceJobResult("not-json")).toThrow("not valid JSON");
    expect(() => parseDataSourceJobResult('{"query":{"columns":[],"rows":[]}}')).toThrow(
      "non-negative integer count",
    );
  });
});
