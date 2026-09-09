<script setup lang="ts">
import {
  command,
  detectTarget,
  products,
  targets,
  repo,
  type Product,
  type Target,
  type Intent,
} from "~/data/install";
const installTitle = ref<HTMLHeadingElement>();
async function changeIntent(value: Intent) {
  intent.value = value;
  await nextTick();
  installTitle.value?.focus({ preventScroll: true });
}
const product = ref<Product>("claude");
const target = ref<Target>("unknown");
const intent = ref<Intent>("install");
const manualOverride = ref(false);
const detected = ref<Target>("unknown");
const showOSPicker = ref(false);
const agentName = computed(() =>
  product.value === "both"
    ? "Claude Code and Codex CLI"
    : product.value === "claude"
      ? "Claude Code"
      : "Codex CLI",
);
const osLabel = computed(
  () =>
    targets.find((item) => item.value === target.value)?.label ??
    "Choose target OS",
);
const copyStatus = ref("");
const commandField = ref<HTMLTextAreaElement>();
const snippet = computed(() =>
  command(product.value, target.value, intent.value),
);
const displaySnippet = computed(() => snippet.value?.replace(" | ", " |\n"));
onMounted(() => {
  detected.value = detectTarget(navigator.userAgent, navigator.maxTouchPoints);
  if (!manualOverride.value) target.value = detected.value;
  showOSPicker.value = target.value === "unknown";
});
watch([product, target, intent], () => {
  copyStatus.value = "";
});
async function copy() {
  const value = snippet.value;
  if (!value) return;
  try {
    await navigator.clipboard.writeText(value);
    if (snippet.value === value)
      copyStatus.value = "Copied. Paste into your target terminal when ready.";
  } catch {
    if (snippet.value !== value) return;
    copyStatus.value =
      "Copy unavailable. Select the command and copy it manually.";
    commandField.value?.focus();
    commandField.value?.select();
  }
}
</script>
<template>
  <section
    id="install"
    class="section anchor-offset guided-install"
    aria-labelledby="install-title"
  >
    <header class="install-heading">
      <h2 id="install-title" ref="installTitle" tabindex="-1">
        Set up notifications
      </h2>
      <p>
        Follow the steps below to add desktop notifications to {{ agentName }}.
      </p>
    </header>

    <div class="agent-cards" role="group" aria-label="Choose your agent">
      <button
        v-for="item in products.filter((item) => item.value !== 'both')"
        :key="item.value"
        class="agent-card"
        :aria-label="item.label"
        :aria-pressed="product === item.value"
        @click="product = item.value"
      >
        <AgentLogo :agent="item.value as 'claude' | 'codex'" />
        <span class="agent-card-copy"
          ><strong
            >{{ item.value === "claude" ? "Claude Code" : "Codex CLI" }}
            <small v-if="item.value === 'codex'">beta</small></strong
          ><span
            >Requires
            {{ item.value === "claude" ? "Claude Code" : "Codex CLI" }}</span
          ></span
        >
        <span class="agent-check" aria-hidden="true">{{
          product === item.value ? "✓" : ""
        }}</span>
      </button>
    </div>
    <div class="environment-row">
      <div class="os-summary">
        <span>{{
          target === "unknown"
            ? "Choose the computer where your agent runs"
            : osLabel
        }}</span>
        <small v-if="target !== 'unknown' && target !== 'manual'">{{
          manualOverride ? "Selected" : "Detected automatically"
        }}</small>
        <button
          class="text-action"
          :aria-expanded="showOSPicker"
          aria-controls="os-picker"
          @click="showOSPicker = !showOSPicker"
        >
          Change
        </button>
      </div>
      <button
        class="text-action both-choice"
        :aria-pressed="product === 'both'"
        aria-label="Both agents"
        @click="product = product === 'both' ? 'claude' : 'both'"
      >
        {{
          product === "both"
            ? "✓ Both agents selected"
            : "Install for both agents"
        }}
      </button>
    </div>
    <div v-if="showOSPicker" id="os-picker" class="os-picker">
      <AppSelect
        :model-value="target"
        :options="targets"
        label="Target operating system"
        @update:model-value="
          target = $event as Target;
          manualOverride = true;
        "
      />
    </div>

    <p v-if="product !== 'claude'" class="notice install-prerequisite">
      <strong>Codex CLI beta — release prerequisite.</strong> Requires a
      published stable plugin release ≥1.42.0. That release is currently draft;
      the Codex installer is not available until it is published.
      <a :href="repo + '/releases'">Check releases</a>.
    </p>

    <div
      v-if="intent === 'configure'"
      class="setup-panel configuration instructions"
    >
      <h3>Make it sound like you</h3>
      <p v-if="product !== 'codex'">
        Inside Claude Code chat, run
        <code>/claude-notifications-go:settings</code>. This is a Claude slash
        command, not a shell command.
      </p>
      <p v-if="product !== 'claude'">
        For Codex, edit the shared settings file
        <code>~/.claude/claude-notifications-go/config.json</code> using the
        <a :href="repo + '#manual-configuration'"
          >documented manual configuration</a
        >. Claude Code is not required for Codex-only setup.
      </p>
      <p>
        Both agents share this configuration. Your settings are preserved when
        updating.
      </p>
    </div>
    <div v-else-if="target === 'manual'" class="setup-panel instructions">
      <h3>Follow the manual route</h3>
      <p v-if="product !== 'codex'">
        <a :href="repo + '#manual-install'">Manual Claude installation</a>
      </p>
      <p v-if="product !== 'claude'">
        <a :href="repo + '#manual-codex-registration'"
          >Manual Codex registration</a
        >
      </p>
    </div>
    <div v-else-if="!snippet" class="setup-panel instructions">
      <p>
        Choose a target operating system to reveal your command. Mobile and
        unknown browsers need an explicit choice.
      </p>
    </div>
    <template v-else>
      <div class="installation-command">
        <div class="command-panel-heading">
          <label for="command"
            >{{ intent === "update" ? "Update" : "Install" }} command</label
          >
          <div class="command-tools">
            <span>{{ target === "windows" ? "Git Bash" : "Bash" }}</span>

          </div>
        </div>
        <div class="install-command-line">
            <button class="copy-icon" aria-label="Copy command" title="Copy command" @click="copy">
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.6"
                aria-hidden="true"
              >
                <rect x="8" y="3" width="12" height="15" rx="2" />
                <path d="M16 18v3H4V7h4" /></svg
              >
            </button>
          <span class="terminal-prompt" aria-hidden="true">$</span>
          <textarea
            id="command"
            ref="commandField"
            :value="displaySnippet"
            readonly
            spellcheck="false"
            rows="2"
            wrap="off"
          />
        </div>
        <button
          v-if="intent === 'install'"
          class="text-action update-under-command"
          aria-label="Update"
          @click="changeIntent('update')"
        >
          Updating instead?
        </button>
        <p role="status" class="copy-status">{{ copyStatus }}</p>
        <p v-if="intent === 'update'" class="update-note">
          Install and update use the same command. Your existing settings are
          preserved.
        </p>
      </div>
      <div class="next-steps">
        <article class="setup-panel next-step">
          <span class="step-icon" aria-hidden="true"
            ><svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.7"
            >
              <path d="m5 6 6 6-6 6M13 18h6" /></svg
          ></span>
          <div>
            <span class="step-label">NEXT STEP 1</span>
            <h3>
              {{ target === "windows" ? "Run in Git Bash" : "Run in Terminal" }}
            </h3>
            <p v-if="target === 'windows'">
              <strong>Run in Git Bash on Windows.</strong> Do not use WSL or
              paste this pipe into PowerShell.
            </p>
            <p v-else>Open your terminal and run the command above.</p>
          </div>
        </article>
        <article class="setup-panel next-step">
          <span class="step-icon" aria-hidden="true"
            ><svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.7"
            >
              <path d="M20 10a8 8 0 1 0-1 7M20 4v6h-6" /></svg
          ></span>
          <div>
            <span class="step-label">NEXT STEP 2</span>
            <h3>
              Restart {{ product === "both" ? "both agents" : agentName }}
            </h3>
            <p v-if="product !== 'codex'">
              Close and reopen Claude Code for notifications to be active.
            </p>
            <p v-if="product !== 'claude'">
              Restart Codex, open <code>/hooks</code>, then review and trust the
              installed hooks. Changed definitions need review again; trust is
              never automatic.
            </p>
          </div>
        </article>
      </div>
    </template>
    <footer class="install-footer">
      <a
        :href="
          repo +
          (product === 'claude'
            ? '#manual-install'
            : '#manual-codex-registration')
        "
        >Installation help ↗</a
      >
      <a v-if="product === 'both'" :href="repo + '#manual-install'"
        >Claude installation help ↗</a
      >
      <div>
        <button
          v-if="intent !== 'install'"
          class="text-action"
          aria-label="Install"
          @click="changeIntent('install')"
        >
          Installation instructions
        </button>

        <button
          v-if="intent !== 'configure'"
          class="text-action"
          aria-label="Configure"
          @click="changeIntent('configure')"
        >
          ⚙ Configure settings
        </button>
      </div>
    </footer>
  </section>
</template>
