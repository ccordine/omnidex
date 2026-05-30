const CHANNEL_NOISE = new Set([
  "external agent session completed",
  "agent finished",
  "agent running…",
  "agent running...",
  "agent running",
]);

export function isScrumChannelNoiseContent(role: string, content: string): boolean {
  const lower = content.trim().toLowerCase();
  if (CHANNEL_NOISE.has(lower)) return true;
  if (lower.startsWith("job status:")) return true;
  if (role === "system") {
    if (lower.startsWith("execution agent:")) return true;
    if (lower.startsWith("agent config source:")) return true;
    if (lower.startsWith("models:")) return true;
    if (lower.startsWith("channel steer sent")) return true;
    if (lower.startsWith("channel message sent")) return true;
  }
  return false;
}
