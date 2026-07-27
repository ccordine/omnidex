import type { ChatMessage } from "./types";

export class TranscriptStore {
  private readonly key = "omni.chat.transcript.v1";

  load(): ChatMessage[] {
    const stored = localStorage.getItem(this.key);
    if (stored === null) return [];
    let value: unknown;
    try {
      value = JSON.parse(stored);
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error);
      throw new Error(`Stored chat transcript is invalid JSON: ${detail}`);
    }
    return validateTranscript(value);
  }

  save(messages: ChatMessage[]): void {
    const compact = validateTranscript(messages).slice(-80);
    localStorage.setItem(this.key, JSON.stringify(compact));
  }

  clear(): void {
    localStorage.removeItem(this.key);
  }
}

function validateTranscript(value: unknown): ChatMessage[] {
  if (!Array.isArray(value)) throw new Error("Stored chat transcript must be an array.");
  return value.map((message, index) => validateMessage(message, index));
}

function validateMessage(value: unknown, index: number): ChatMessage {
  if (!value || typeof value !== "object") {
    throw new Error(`Stored chat transcript message ${index} must be an object.`);
  }
  const message = value as Record<string, unknown>;
  if (!isChatRole(message.role)) {
    throw new Error(`Stored chat transcript message ${index} has an invalid role.`);
  }
  if (typeof message.content !== "string") {
    throw new Error(`Stored chat transcript message ${index} has invalid content.`);
  }
  if (typeof message.at !== "string" || !message.at.trim() || Number.isNaN(Date.parse(message.at))) {
    throw new Error(`Stored chat transcript message ${index} has an invalid timestamp.`);
  }
  return { role: message.role, content: message.content, at: message.at };
}

function isChatRole(value: unknown): value is ChatMessage["role"] {
  return value === "user" || value === "assistant" || value === "system" || value === "error";
}
