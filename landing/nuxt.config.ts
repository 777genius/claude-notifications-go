export default defineNuxtConfig({
  compatibilityDate: "2026-09-09",
  devtools: { enabled: false },
  app: {
    baseURL: process.env.NUXT_APP_BASE_URL || "/agent-notifications/",
    head: {
      title: "Agent Notifications — Stay in flow",
      htmlAttrs: { lang: "en" },
      meta: [
        {
          name: "description",
          content:
            "Desktop notifications for Claude Code and Codex CLI beta. Know when work finishes, a question needs you, or something goes wrong.",
        },
      ],
    },
  },
  css: ["~/assets/main.css"],
  nitro: { preset: "static" },
});
