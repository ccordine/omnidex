import { describe, expect, it } from "vitest";
import { requireJobWorkspaceBinding, requireWorkspaceIdentity } from "./job_workspace_contract";

describe("frozen job workspace contract", () => {
  it("preserves unbound Normal jobs and requires the direct coding workspace", () => {
    expect(requireJobWorkspaceBinding({ job: { id: 17, status: "running", current_generation: 1 }, steps: [] }, 17)).toEqual({});
    expect(() => requireJobWorkspaceBinding({ job: { id: 17, pipeline: "coding", status: "running", current_generation: 1 }, steps: [] }, 17)).toThrow("frozen workspace binding");
  });

  it("accepts canonical host and portable client directory identities", () => {
    for (const identity of ["directory_0_1", "directory_18446744073709551615_18446744073709551615", "client_0123456789abcdef0123456789abcdef_directory_1_42"]) {
      expect(requireWorkspaceIdentity(identity, "fixture")).toBe(identity);
    }
  });

  it.each([null, 1, "directory_1_0", "directory_01_42", "directory_1_042", "directory_18446744073709551616_42", "directory_1_18446744073709551616", " client_0123456789abcdef0123456789abcdef_directory_1_42", "client_0123456789ABCDEF0123456789abcdef_directory_1_42"])("rejects an inexact workspace identity %s", (identity) => {
    expect(() => requireWorkspaceIdentity(identity, "fixture")).toThrow("exact directory or client identity");
  });
});
