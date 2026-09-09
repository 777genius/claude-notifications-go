<script setup lang="ts">
import { repo } from "~/data/install";
const features = [
  {
    number: "01",
    title: "Step away, stay informed",
    text: "Get a desktop notification when your agent finishes or needs your attention. Spend less time watching a terminal.",
    icon: "↗",
  },
  {
    number: "02",
    title: "A sound for every moment",
    text: "Pick sounds, set the volume, and choose an audio device. Keep the signals you want and quiet the rest.",
    icon: "♫",
  },
  {
    number: "03",
    title: "Your workflow, connected",
    text: "Send updates to Slack, Discord, Telegram, Teams and more. Configure webhooks for wherever you work.",
    icon: "⌁",
  },
];
</script>
<template>
  <div class="site">
    <a class="skip" href="#main">Skip to content</a>
    <header class="header">
      <a class="brand" href="#"
        ><span class="brand-icon" aria-hidden="true"
          ><svg
            width="28"
            height="30"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
          >
            <path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9Z" />
            <path d="M9 21h6M12 1v2" /></svg></span
        ><span
          >Claude Notifications<small>FOR YOUR CODING AGENTS</small></span
        ></a
      >
      <nav aria-label="Main">
        <a href="#features">Features</a><a href="#install">Install</a
        ><a href="#faq">FAQ</a>
      </nav>
      <a class="github" :href="repo">GitHub ↗</a>
    </header>
    <main id="main">
      <div class="hero-wrap">
        <PageBackground />
        <section class="hero">
          <div class="hero-copy">
            <p class="eyebrow">
              <span class="dot" /> LESS WATCHING. MORE BUILDING.
            </p>
            <h1>Stay in flow<br /><em>We'll get your attention</em></h1>
            <p class="lead">
              Let your agent work. Know when it's done.<br />Sounds, desktop
              notifications and webhooks for Claude Code and Codex CLI.
            </p>
            <div class="hero-actions">
              <a class="primary" href="#install"
                >Set up notifications <span>↗</span></a
              ><a class="secondary" :href="repo">Explore on GitHub</a>
            </div>
            <p class="hero-note">Open source · Runs locally · Codex CLI beta</p>
          </div>
          <NotificationPreview />
        </section>
      </div>
      <div class="compatibility">
        <span>BUILT FOR YOUR WORKFLOW</span
        ><strong><AgentLogo agent="claude" /> Claude Code</strong
        ><strong
          ><AgentLogo agent="codex" /> Codex CLI <small>beta</small></strong
        ><PlatformLogos />
      </div>
      <InstallWizard />
      <section id="features" class="section anchor-offset">
        <p class="eyebrow">THE RIGHT SIGNAL. AT THE RIGHT TIME.</p>
        <h2>Keep your focus<br />Lose the constant checking</h2>
        <div class="feature-grid">
          <article
            v-for="feature in features"
            :key="feature.number"
            class="panel feature"
          >
            <div class="feature-top">
              <span>{{ feature.number }}</span
              ><b>{{ feature.icon }}</b>
            </div>
            <h3>{{ feature.title }}</h3>
            <p>{{ feature.text }}</p>
          </article>
        </div>
        <p class="feature-foot">
          Click-to-focus support depends on your terminal and OS.
          <a :href="repo + '#platform-support'">See platform details ↗</a>
        </p>
      </section>
      <section id="faq" class="section faq anchor-offset">
        <div>
          <p class="eyebrow">A FEW THINGS TO KNOW</p>
          <h2>Before you<br />get back to work</h2>
        </div>
        <div class="faq-list">
          <details>
            <summary>Do I need both agents installed?</summary>
            <p>
              No. Select Claude Code, Codex CLI, or both. Install the selected
              agent CLI first; the command installs this notification plugin.
            </p>
          </details>
          <details>
            <summary>Are Codex hooks installed automatically?</summary>
            <p>
              Yes. The installer registers the hooks and saves a stable runtime
              copy. Open <code>/hooks</code> in Codex to review and trust them.
              Changed definitions need review again.
            </p>
          </details>
          <details>
            <summary>Will an update reset my settings?</summary>
            <p>
              The installer preserves your shared configuration and foreign
              Codex hooks. Install and update use the same command. Restart the
              selected agent after updating.
            </p>
          </details>
          <details>
            <summary>Can I choose what makes a sound?</summary>
            <p>
              Yes. Configure sounds, volume, audio device and notification
              channels. Choose Configure in the guide for the steps specific to
              your agent.
            </p>
          </details>
          <details>
            <summary>Does click-to-focus work everywhere?</summary>
            <p>
              Support varies by OS and terminal. iTerm2 needs its Python API for
              precise tab targeting. Windows focuses a window, not an exact tab.
              <a :href="repo + '#platform-support'"
                >Check supported environments.</a
              >
            </p>
          </details>
          <details>
            <summary>What if I don't get a notification?</summary>
            <p>
              Check your OS notification permissions and chosen channels. For
              Codex, confirm the hooks are trusted.
              <a :href="repo + '#troubleshooting'">Follow troubleshooting</a>
              for platform-specific checks.
            </p>
          </details>
        </div>
      </section>
      <section class="closing">
        <p class="eyebrow">LET YOUR AGENT TAKE THE NEXT TURN</p>
        <h2>Your attention is valuable</h2>
        <a class="primary" href="#install">Make every notification count ↗</a>
      </section>
    </main>
    <footer>
      <a class="brand" href="#">Claude Notifications</a
      ><span
        >Built by <a href="https://github.com/777genius">777genius</a>. Open
        source.</span
      ><a :href="repo + '/blob/main/LICENSE'">License</a
      ><a :href="repo">Source ↗</a>
    </footer>
  </div>
</template>
