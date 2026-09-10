<script setup lang="ts">
import { isLocaleCode, supportedLocales } from "~/data/i18n";

const { locale, setLocale, t, te } = useI18n();
const pending = ref(false);
const error = ref(false);

const currentName = computed(
  () =>
    supportedLocales.find((item) => item.code === locale.value)?.name ??
    supportedLocales[0].name,
);

async function changeLanguage(event: Event) {
  const value = (event.target as HTMLSelectElement).value;
  if (!isLocaleCode(value) || value === locale.value || pending.value) return;

  pending.value = true;
  error.value = false;
  const previousLocale = locale.value;
  try {
    await setLocale(value);
    if (!te("seo.title", value))
      throw new Error(`Locale messages for ${value} were not loaded`);
  } catch {
    if (locale.value !== previousLocale) {
      try {
        await setLocale(previousLocale);
      } catch {
        // Keep the alert visible if rollback also fails.
      }
    }
    error.value = true;
    (event.target as HTMLSelectElement).value = locale.value;
  } finally {
    pending.value = false;
  }
}
</script>

<template>
  <div class="language-switcher">
    <span class="language-switcher__icon" aria-hidden="true">文</span>
    <select
      :value="locale"
      :aria-label="t('language.current', { language: currentName })"
      :disabled="pending"
      :aria-busy="pending"
      @change="changeLanguage"
    >
      <option v-for="item in supportedLocales" :key="item.code" :value="item.code">
        {{ item.name }}
      </option>
    </select>
    <span v-if="error" class="visually-hidden" role="alert">
      {{ t("language.switchError") }}
    </span>
  </div>
</template>
