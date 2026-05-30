import { ar } from "./ar";
import { de } from "./de";
import { en, type MessageCatalog, type MessageKey } from "./en";
import { es } from "./es";
import { hi } from "./hi";
import { ja } from "./ja";
import { ru } from "./ru";
import { th } from "./th";
import { zhHans } from "./zh-Hans";

export type LocaleCode = "en" | "es" | "zh-Hans" | "ja" | "th" | "de" | "ru" | "hi" | "ar";

export type LocaleOption = {
  code: LocaleCode;
  label: string;
  nativeLabel: string;
  dir: "ltr" | "rtl";
};

export const LOCALE_OPTIONS: LocaleOption[] = [
  { code: "en", label: "English", nativeLabel: "English", dir: "ltr" },
  { code: "es", label: "Spanish", nativeLabel: "Español", dir: "ltr" },
  { code: "zh-Hans", label: "Chinese (Simplified)", nativeLabel: "简体中文", dir: "ltr" },
  { code: "ja", label: "Japanese", nativeLabel: "日本語", dir: "ltr" },
  { code: "th", label: "Thai", nativeLabel: "ไทย", dir: "ltr" },
  { code: "de", label: "German", nativeLabel: "Deutsch", dir: "ltr" },
  { code: "ru", label: "Russian", nativeLabel: "Русский", dir: "ltr" },
  { code: "hi", label: "Hindi (India)", nativeLabel: "हिन्दी", dir: "ltr" },
  { code: "ar", label: "Arabic", nativeLabel: "العربية", dir: "rtl" },
];

const STORAGE_KEY = "omni.locale.v1";

const catalogs: Record<LocaleCode, MessageCatalog> = {
  en,
  es,
  "zh-Hans": zhHans,
  ja,
  th,
  de,
  ru,
  hi,
  ar,
};

let activeLocale: LocaleCode = "en";

function isLocaleCode(value: string): value is LocaleCode {
  return value in catalogs;
}

export function getLocale(): LocaleCode {
  return activeLocale;
}

export function getLocaleOption(code = activeLocale): LocaleOption {
  return LOCALE_OPTIONS.find((item) => item.code === code) ?? LOCALE_OPTIONS[0];
}

export function setLocale(code: LocaleCode): void {
  activeLocale = code;
  try {
    localStorage.setItem(STORAGE_KEY, code);
  } catch {
    /* ignore storage failures */
  }
}

export function t(key: MessageKey, locale: LocaleCode = activeLocale): string {
  return catalogs[locale]?.[key] ?? catalogs.en[key] ?? key;
}

export function initI18n(): LocaleCode {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && isLocaleCode(stored)) activeLocale = stored;
  } catch {
    /* ignore */
  }
  applyDocumentLocale();
  applyI18n(document);
  return activeLocale;
}

export function applyDocumentLocale(locale: LocaleCode = activeLocale): void {
  const option = getLocaleOption(locale);
  document.documentElement.lang = locale;
  document.documentElement.dir = option.dir;
  document.title = t("app.pageTitle", locale);
}

export function applyI18n(root: ParentNode = document): void {
  root.querySelectorAll<HTMLElement>("[data-i18n]").forEach((node) => {
    const key = node.dataset.i18n as MessageKey | undefined;
    if (key) node.textContent = t(key);
  });
  root.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("[data-i18n-placeholder]").forEach((node) => {
    const key = node.dataset.i18nPlaceholder as MessageKey | undefined;
    if (key) node.placeholder = t(key);
  });
  root.querySelectorAll<HTMLElement>("[data-i18n-title]").forEach((node) => {
    const key = node.dataset.i18nTitle as MessageKey | undefined;
    if (key) node.title = t(key);
  });
  root.querySelectorAll<HTMLElement>("[data-i18n-aria]").forEach((node) => {
    const key = node.dataset.i18nAria as MessageKey | undefined;
    if (key) node.setAttribute("aria-label", t(key));
  });
}

export { type MessageKey, type MessageCatalog };
