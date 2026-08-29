import { readJSON } from "./api";
import type RecyclrController from "../controllers/recyclr_controller";
import { setGlobalLoading } from "./loading";

export type ServerComponent = {
  html: { bundle: string };
};

export function requireServerBundle(payload: unknown, label: string): string {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error(`${label} response is not an object.`);
  }
  const html = (payload as { html?: unknown }).html;
  if (!html || typeof html !== "object" || Array.isArray(html) || Object.keys(html).length !== 1 || !("bundle" in html)) {
    throw new Error(`${label} response did not include one exact server-rendered html authority.`);
  }
  const bundle = (html as { bundle?: unknown }).bundle;
  if (typeof bundle !== "string" || !bundle.trim()) {
    throw new Error(`${label} response did not include its required server-rendered bundle.`);
  }
  return bundle;
}

export async function fetchServerComponent<T extends ServerComponent>(url: string, init?: RequestInit): Promise<T> {
  setGlobalLoading(true);
  try {
    const payload = await readJSON<T>(await fetch(url, init));
    requireServerBundle(payload, "Component");
    return payload;
  } finally {
    setGlobalLoading(false);
  }
}

export async function renderServerBundle(host: RecyclrController, payload: unknown, label: string): Promise<void> {
  setGlobalLoading(true);
  try {
    await host.renderBundle(requireServerBundle(payload, label));
  } finally {
    setGlobalLoading(false);
  }
}
