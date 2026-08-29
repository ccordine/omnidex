import { exactMicrosecondTimestamp } from "./response_validation";

export function validateCanonicalRevision(value: unknown, label: string): string {
  return exactMicrosecondTimestamp(value, label);
}
