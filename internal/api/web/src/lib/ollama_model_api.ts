import { jsonRequest, readJSON } from "./api";

export async function pullOllamaModel(model: string): Promise<void> {
  await readJSON(await fetch("/v1/ollama/models", jsonRequest({ model })));
}

export async function deleteOllamaModel(name: string): Promise<void> {
  await readJSON(await fetch(`/v1/ollama/models/${encodeURIComponent(name)}`, { method: "DELETE" }));
}
