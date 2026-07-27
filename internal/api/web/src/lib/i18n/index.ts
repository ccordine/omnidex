import enMessages from "../../../locales/en.json";
import esMessages from "../../../locales/es.json";
import jaMessages from "../../../locales/ja.json";
import ruMessages from "../../../locales/ru.json";
import zhHansMessages from "../../../locales/zh-Hans.json";

export const LOCALE_CODES = ["en", "es", "zh-Hans", "ru", "ja"] as const;

export type LocaleCode = (typeof LOCALE_CODES)[number];
export type MessageKey = keyof typeof enMessages;
export type MessageCatalog = Record<MessageKey, string>;

type LocaleOption = {
  code: LocaleCode;
  dir: "ltr" | "rtl";
};

const localeOptions: Record<LocaleCode, LocaleOption> = {
  en: { code: "en", dir: "ltr" },
  es: { code: "es", dir: "ltr" },
  "zh-Hans": { code: "zh-Hans", dir: "ltr" },
  ru: { code: "ru", dir: "ltr" },
  ja: { code: "ja", dir: "ltr" },
};

function validateCatalog(locale: LocaleCode, input: Record<string, unknown>): MessageCatalog {
  const englishKeys = Object.keys(enMessages).sort();
  const keys = Object.keys(input).sort();
  const missing = englishKeys.filter((key) => !Object.prototype.hasOwnProperty.call(input, key));
  const unknown = keys.filter((key) => !Object.prototype.hasOwnProperty.call(enMessages, key));
  if (missing.length || unknown.length) {
    throw new Error(`Locale ${locale} catalog mismatch; missing=[${missing.join(", ")}], unknown=[${unknown.join(", ")}].`);
  }
  for (const key of englishKeys) {
    const message = input[key];
    if (typeof message !== "string" || !message.trim()) {
      throw new Error(`Locale ${locale} message ${JSON.stringify(key)} must be a non-empty string.`);
    }
  }
  return input as MessageCatalog;
}

const catalogs: Record<LocaleCode, MessageCatalog> = {
  en: validateCatalog("en", enMessages),
  es: validateCatalog("es", esMessages),
  "zh-Hans": validateCatalog("zh-Hans", zhHansMessages),
  ru: validateCatalog("ru", ruMessages),
  ja: validateCatalog("ja", jaMessages),
};

let activeLocale: LocaleCode = "en";

export function isLocaleCode(value: string): value is LocaleCode {
  return LOCALE_CODES.includes(value as LocaleCode);
}

export function initI18n(): LocaleCode {
  const serverLocale = document.documentElement.lang.trim();
  if (!isLocaleCode(serverLocale)) {
    throw new Error(`Server rendered unsupported UI locale ${JSON.stringify(serverLocale)}.`);
  }
  const option = localeOptions[serverLocale];
  if (document.documentElement.dir !== option.dir) {
    throw new Error(`Server rendered locale ${serverLocale} with invalid direction ${JSON.stringify(document.documentElement.dir)}.`);
  }
  activeLocale = serverLocale;
  return activeLocale;
}

export function getLocale(): LocaleCode {
  return activeLocale;
}

export function t(key: MessageKey, locale: LocaleCode = activeLocale): string {
  const catalog = catalogs[locale];
  if (!catalog) throw new Error(`Unsupported UI locale ${JSON.stringify(locale)}.`);
  if (!Object.prototype.hasOwnProperty.call(catalog, key)) {
    throw new Error(`Locale ${locale} has no message ${JSON.stringify(key)}.`);
  }
  const message = catalog[key];
  if (typeof message !== "string" || !message.trim()) {
    throw new Error(`Locale ${locale} message ${JSON.stringify(key)} is blank.`);
  }
  return message;
}

export function tf(
  key: MessageKey,
  parameters: Record<string, string | number>,
  locale: LocaleCode = activeLocale,
): string {
  const message = t(key, locale);
  const required = new Set(Array.from(message.matchAll(/\{([a-z][a-z0-9_]*)\}/gi), (match) => match[1]));
  for (const name of required) {
    if (!Object.prototype.hasOwnProperty.call(parameters, name)) {
      throw new Error(`Locale ${locale} message ${JSON.stringify(key)} requires parameter ${JSON.stringify(name)}.`);
    }
  }
  for (const name of Object.keys(parameters)) {
    if (!required.has(name)) {
      throw new Error(`Locale ${locale} message ${JSON.stringify(key)} does not accept parameter ${JSON.stringify(name)}.`);
    }
  }
  return message.replace(/\{([a-z][a-z0-9_]*)\}/gi, (_placeholder, name: string) => String(parameters[name]));
}
