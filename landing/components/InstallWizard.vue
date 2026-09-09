<script setup lang="ts">
import {
  command,
  detectTarget,
  products,
  targets,
  intents,
  repo,
  type Product,
  type Target,
  type Intent,
} from "~/data/install";
const product = ref<Product>("claude");
const target = ref<Target>("unknown");
const intent = ref<Intent>("install");
const manualOverride = ref(false);
const copyStatus = ref("");
const commandField = ref<HTMLTextAreaElement>();
const snippet = computed(() =>
  command(product.value, target.value, intent.value),
);
onMounted(() => {
  if (!manualOverride.value)
    target.value = detectTarget(navigator.userAgent, navigator.maxTouchPoints);
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
    class="section anchor-offset"
    aria-labelledby="install-title"
  >
    <p class="eyebrow">YOUR NEXT STEP</p>
    <h2 id="install-title">A little setup, a lot less checking</h2>
    <p>Choose your agent and the machine where it runs.</p>
    <div class="panel wizard">
      <fieldset>
        <legend>01 / Agent</legend>
        <div class="choices">
          <button
            v-for="item in products"
            :key="item.value"
            :aria-pressed="product === item.value"
            @click="product = item.value"
          >
            {{ item.label }}
          </button>
        </div>
      </fieldset>
      <div class="wizard-row">
        <div class="target-field">
          <span class="field-label">02 / Target operating system</span>
          <AppSelect
            :model-value="target"
            :options="targets"
            label="02 / Target operating system"
            @update:model-value="
              target = $event as Target;
              manualOverride = true;
            "
          />
        </div>
        <fieldset>
          <legend>03 / What would you like to do?</legend>
          <div class="choices">
            <button
              v-for="item in intents"
              :key="item"
              :aria-pressed="intent === item"
              @click="intent = item"
            >
              {{ item[0].toUpperCase() + item.slice(1) }}
            </button>
          </div>
        </fieldset>
      </div>
      <div class="instructions" aria-live="polite">
        <p v-if="product !== 'claude'" class="notice">
          <strong>Codex CLI beta — release prerequisite.</strong> Requires a
          published stable plugin release ≥1.42.0. That release is currently
          draft; the Codex installer is not available until it is published.
          <a :href="repo + '/releases'">Check releases</a>. Install the Codex
          CLI first<span v-if="product === 'both'">, and Claude Code too</span>.
        </p>
        <p v-if="product === 'claude'">
          Prerequisite: install Claude Code first.
        </p>
        <template v-if="intent === 'configure'">
          <h3>Make it sound like you</h3>
          <p v-if="product !== 'codex'">
            Inside Claude Code chat, run
            <code>/claude-notifications-go:settings</code>. This is a Claude
            slash command, not a shell command.
          </p>
          <p v-if="product !== 'claude'">
            For Codex, edit the shared settings file
            <code>~/.claude/claude-notifications-go/config.json</code> using the
            <a :href="repo + '#manual-configuration'"
              >documented manual configuration</a
            >. Claude Code is not required for Codex-only setup.
          </p>
          <p>
            Both agents share this configuration. Keep your settings when
            updating.
          </p>
        </template>
        <template v-else-if="target === 'manual'">
          <h3>Follow the manual route</h3>
          <p v-if="product !== 'codex'">
            <a :href="repo + '#manual-install'">Manual Claude installation</a>
          </p>
          <p v-if="product !== 'claude'">
            <a :href="repo + '#manual-codex-registration'"
              >Manual Codex registration</a
            >
          </p>
        </template>
        <p v-else-if="!snippet">
          Choose a target operating system to reveal your command. Mobile and
          unknown browsers need an explicit choice.
        </p>
        <template v-else>
          <p v-if="target === 'windows'">
            <strong>Run in Git Bash on Windows.</strong> Do not use WSL or paste
            this pipe into PowerShell.
          </p>
          <p v-else>
            Run in a terminal on your
            {{ target === "macos" ? "Mac" : "Linux machine" }}.
          </p>
          <label class="command-label" for="command"
            >{{ intent === "update" ? "Update" : "Install" }} command</label
          >
          <div class="command-box">
            <textarea
              id="command"
              ref="commandField"
              :value="snippet"
              readonly
              spellcheck="false"
              rows="3"
            /><button class="primary" @click="copy">Copy command</button>
          </div>
          <p role="status" class="copy-status">{{ copyStatus }}</p>
          <p v-if="intent === 'update'">
            Install and update use the same bootstrap command. Your existing
            shared settings are preserved.
          </p>
          <p v-if="product !== 'codex'">
            After {{ intent }}: restart Claude Code.
          </p>
          <p v-if="product !== 'claude'">
            After {{ intent }}: restart Codex, open <code>/hooks</code>, then
            review and trust the installed hooks yourself. Changed definitions
            require review again; trust is never automatic.
          </p>
          <p class="muted">
            If setup fails, follow the
            <a
              :href="
                repo +
                (product === 'claude'
                  ? '#manual-install'
                  : '#manual-codex-registration')
              "
              >manual instructions</a
            ><template v-if="product === 'both'">
              or
              <a :href="repo + '#manual-install'"
                >manual Claude installation</a
              ></template
            >.
          </p>
        </template>
      </div>
    </div>
  </section>
</template>
