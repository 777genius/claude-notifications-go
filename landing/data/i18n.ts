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
] as const;

export type LocaleCode = (typeof supportedLocales)[number]["code"];

export function isLocaleCode(value: unknown): value is LocaleCode {
  return supportedLocales.some((locale) => locale.code === value);
}
