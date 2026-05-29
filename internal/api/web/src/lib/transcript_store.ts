import type { ChatMessage } from "./types";

export class TranscriptStore {
  private readonly key = "omni.chat.transcript.v1";

  load(): ChatMessage[] {
    try {
      return JSON.parse(localStorage.getItem(this.key) || "[]") as ChatMessage[];
    } catch {
      return [];
    }
  }

  save(messages: ChatMessage[]): void {
    const compact = messages.slice(-80);
    localStorage.setItem(this.key, JSON.stringify(compact));
  }

  clear(): void {
    localStorage.removeItem(this.key);
  }
}
