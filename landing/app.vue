<script setup lang="ts">
import { repo } from "~/data/install";
const features = [
  {
    number: "01",
    title: "Step away, stay informed",
    text: "Get a desktop notification when your agent finishes or needs your attention. Spend less time watching a terminal.",
    label: "DESKTOP NOTIFICATIONS",
  },
  {
    number: "02",
    title: "A sound for every moment",
    text: "Pick sounds, set the volume, and choose an audio device. Keep the signals you want and quiet the rest.",
    label: "SOUND CONTROLS",
  },
  {
    number: "03",
    title: "Your workflow, connected",
    text: "Send updates to Slack, Discord, Telegram, Teams and more. Configure webhooks for wherever you work.",
    label: "WEBHOOK INTEGRATIONS",
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
          >Agent Notifications<small>FOR YOUR CODING AGENTS</small></span
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
      <div class="compatibility" role="group" aria-label="Supported operating systems">
        <PlatformLogos />
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
            <div class="feature-visual" aria-hidden="true">
              <div v-if="feature.number === '01'" class="mini-notification">
                <span class="signal-icon">✓</span>
                <div><strong>Your agent is done</strong><span>Ready when you are.</span></div>
                <span class="mini-now">now</span>
              </div>
              <div v-else-if="feature.number === '02'" class="sound-preview">
                <div class="sound-wave"><i v-for="(height, index) in [12, 22, 36, 20, 46, 30, 54, 38, 24, 44, 28, 16, 32, 20, 10]" :key="index" :style="{ height: height + 'px' }" /></div>
                <span>Your sound. Your volume.</span>
              </div>
              <div v-else class="channel-preview">
                <span class="channel-source">Agent event</span>
                <span class="channel-line" />
                <div class="channel-tags"><span>Slack</span><span>Discord</span><span>Telegram</span><span>Teams</span></div>
              </div>
            </div>
            <div class="feature-top"><span>{{ feature.label }}</span></div>
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
        <div class="faq-header">
          <p class="eyebrow">A FEW THINGS TO KNOW</p>
          <h2>Frequently asked questions</h2>
          <p>Everything you need to get back to work.</p>
        </div>
        <div class="faq-content">
        <div class="faq-list">
          <details>
            <summary><span class="faq-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3v3m8-3v3M5 6h14v14H5zM9 11h.01M15 11h.01M9 16h6" /></svg></span><span>Do I need both agents installed?</span><span class="faq-chevron" aria-hidden="true"></span></summary>
            <p>
              No. Select Claude Code, Codex CLI, or both. Install the selected
              agent CLI first; the command installs this notification plugin.
            </p>
          </details>
          <details>
            <summary><span class="faq-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M8 4H4v6h4m8 4h4v6h-4M4 7h10a4 4 0 0 1 4 4v3M6 10v3a4 4 0 0 0 4 4h10" /></svg></span><span>Are Codex hooks installed automatically?</span><span class="faq-chevron" aria-hidden="true"></span></summary>
            <p>
              Yes. The installer registers the hooks and saves a stable runtime
              copy. Open <code>/hooks</code> in Codex to review and trust them.
              Changed definitions need review again.
            </p>
          </details>
          <details>
            <summary><span class="faq-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-1l2 6M4 12l2 6a7 7 0 0 0 12-1" /></svg></span><span>Will an update reset my settings?</span><span class="faq-chevron" aria-hidden="true"></span></summary>
            <p>
              The installer preserves your shared configuration and foreign
              Codex hooks. Install and update use the same command. Restart the
              selected agent after updating.
            </p>
          </details>
          <details>
            <summary><span class="faq-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 9h4l5-4v14l-5-4H4zM16 8a6 6 0 0 1 0 8M19 5a10 10 0 0 1 0 14" /></svg></span><span>Can I choose what makes a sound?</span><span class="faq-chevron" aria-hidden="true"></span></summary>
            <p>
              Yes. Configure sounds, volume, audio device and notification
              channels. Choose Configure in the guide for the steps specific to
              your agent.
            </p>
          </details>
          <details>
            <summary><span class="faq-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 4h16v12H4zM8 20h8m-4-4v4" /></svg></span><span>Does click-to-focus work everywhere?</span><span class="faq-chevron" aria-hidden="true"></span></summary>
            <p>
              Support varies by OS and terminal. iTerm2 needs its Python API for
              precise tab targeting. Windows focuses a window, not an exact tab.
              <a :href="repo + '#platform-support'"
                >Check supported environments.</a
              >
            </p>
          </details>
          <details>
            <summary><span class="faq-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M9 9a3 3 0 1 1 5 2c-2 1-2 2-2 3m0 3h.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0" /></svg></span><span>What if I don't get a notification?</span><span class="faq-chevron" aria-hidden="true"></span></summary>
            <p>
              Check your OS notification permissions and chosen channels. For
              Codex, confirm the hooks are trusted.
              <a :href="repo + '#troubleshooting'">Follow troubleshooting</a>
              for platform-specific checks.
            </p>
          </details>
        </div>
        <div class="faq-decoration" aria-hidden="true"><span class="faq-ring" /><span class="faq-ring" /><span class="faq-ring" /><span class="faq-orb">?</span></div>
        </div>
      </section>
      <section class="closing">
        <p class="eyebrow">LET YOUR AGENT TAKE THE NEXT TURN</p>
        <h2>Your attention is valuable</h2>
        <a class="primary" href="#install">Make every notification count ↗</a>
      </section>
    </main>
    <footer>
      <a class="brand" href="#">Agent Notifications</a
      ><span
        >Built by <a href="https://github.com/777genius">777genius</a>. Open
        source.</span
      ><a :href="repo + '/blob/main/LICENSE'">License</a
      ><a :href="repo">Source ↗</a>
    </footer>
  </div>
</template>
