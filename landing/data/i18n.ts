export const defaultLocale = "en" as const;

export const supportedLocales = [
  {
    code: "en",
    language: "en-US",
    name: "English",
    file: "en.json",
    dir: "ltr" as const,
  },
  {
    code: "zh",
    language: "zh-CN",
    name: "简体中文",
    file: "zh.json",
    dir: "ltr" as const,
  },
  { code: "es", language: "es", name: "Español", file: "es.json", dir: "ltr" as const },
  { code: "fr", language: "fr", name: "Français", file: "fr.json", dir: "ltr" as const },
  { code: "de", language: "de", name: "Deutsch", file: "de.json", dir: "ltr" as const },
  { code: "pt", language: "pt-BR", name: "Português (Brasil)", file: "pt.json", dir: "ltr" as const },
  { code: "ja", language: "ja", name: "日本語", file: "ja.json", dir: "ltr" as const },
  { code: "ko", language: "ko", name: "한국어", file: "ko.json", dir: "ltr" as const },
  { code: "ru", language: "ru", name: "Русский", file: "ru.json", dir: "ltr" as const },
  { code: "ar", language: "ar", name: "العربية", file: "ar.json", dir: "rtl" as const },
  { code: "hi", language: "hi", name: "हिन्दी", file: "hi.json", dir: "ltr" as const },
  { code: "it", language: "it", name: "Italiano", file: "it.json", dir: "ltr" as const },
] as const;

export type LocaleCode = (typeof supportedLocales)[number]["code"];

export function isLocaleCode(value: unknown): value is LocaleCode {
  return supportedLocales.some((locale) => locale.code === value);
}
