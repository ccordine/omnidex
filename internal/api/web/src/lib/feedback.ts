import { showToast } from "./toast";

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function toastOk(message: string): void {
  showToast(message, "ok");
}

export function toastError(message: string): void {
  showToast(message, "error");
}

export function toastFromError(error: unknown): void {
  showToast(errorMessage(error), "error");
}

/** Inline status + success toast for completed user actions. */
export function reportOk(setStatus: (message: string, tone: "ok") => void, message: string): void {
  setStatus(message, "ok");
  toastOk(message);
}

/** Inline status + error toast from a caught exception. */
export function reportError(setStatus: (message: string, tone: "error") => void, error: unknown): void {
  const message = errorMessage(error);
  setStatus(message, "error");
  toastError(message);
}

/** Inline status + error toast for validation or API messages. */
export function reportErrorMessage(setStatus: (message: string, tone: "error") => void, message: string): void {
  setStatus(message, "error");
  toastError(message);
}
