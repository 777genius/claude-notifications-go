<script setup lang="ts">
const notifications = [
  {
    agent: "claude",
    title: "❓ Question",
    body: "What would you like to name the test file in /tmp/?",
  },
  {
    agent: "claude",
    title: "📋 Plan",
    body: "Plan: Create test file in /tmp/",
  },
  {
    agent: "codex",
    title: "✅ Completed",
    body: "Done. Installation now comes right after the hero.",
    detail: "✏️ 3 edited  ⏱ 35s",
  },
  {
    agent: "claude",
    title: "🔍 Review",
    body: "Review complete. No issues found in the updated installer.",
  },
  {
    agent: "codex",
    title: "🔐 Permission Request",
    body: "Approval required: shell",
  },
  {
    agent: "claude",
    title: "⏱️ Session Limit Reached",
    body: "Session limit reached. Please wait before continuing.",
  },
  {
    agent: "codex",
    title: "🔴 API Error: 401",
    body: "Authentication expired. Please sign in again.",
  },
] as const;
const cursor = ref(0);
const paused = ref(false);
const reduced = ref(false);
const visible = computed(() =>
  [0, 1, 2].map((offset) => ({
    item: notifications[(cursor.value + offset) % notifications.length]!,
    id: cursor.value + offset,
  })),
);
let timer: ReturnType<typeof setInterval> | undefined;
let media: MediaQueryList | undefined;
function preference() {
  reduced.value = media?.matches ?? false;
}
onMounted(() => {
  media = matchMedia("(prefers-reduced-motion: reduce)");
  preference();
  media.addEventListener("change", preference);
  timer = setInterval(() => {
    if (!paused.value && !reduced.value && !document.hidden) cursor.value++;
  }, 3400);
});
onUnmounted(() => {
  clearInterval(timer);
  media?.removeEventListener("change", preference);
});
</script>
<template>
  <section
    class="notification-preview"
    aria-label="Agent notification examples"
  >
    <div class="preview-heading">
      <span>YOUR AGENT HAS AN UPDATE</span>
      <button
        v-if="!reduced"
        class="preview-control"
        :aria-pressed="paused"
        @click="paused = !paused"
      >
        {{ paused ? "Resume" : "Pause" }}
      </button>
      <button v-else class="preview-control" @click="cursor++">Next</button>
    </div>
    <TransitionGroup name="notification" tag="div" class="notification-stack">
      <article
        v-for="{ item, id } in visible"
        :key="id"
        class="notification-card"
      >
        <AgentLogo :agent="item.agent" />
        <div class="notification-copy">
          <div class="notification-title">
            <h3>{{ item.title }}</h3>
            <span>now</span>
          </div>
          <strong>main · notification_plugin_go</strong>
          <p>{{ item.body }}</p>
          <span v-if="'detail' in item" class="notification-detail">{{
            item.detail
          }}</span>
        </div>
      </article>
    </TransitionGroup>
  </section>
</template>
