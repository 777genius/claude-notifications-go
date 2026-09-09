import { test, expect, type Page } from "@playwright/test";
async function chooseOS(page: Page, value: string) {
  const labels: Record<string, string> = {
    macos: "macOS",
    linux: "Linux",
    windows: "Windows · Git Bash",
    manual: "Manual instructions",
  };
  await page
    .getByRole("combobox", { name: "02 / Target operating system" })
    .click();
  await page.getByRole("option", { name: labels[value], exact: true }).click();
}
test("production command matrix, aftercare, clipboard and configuration", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("");
  await expect(page.getByRole("heading", { level: 1 })).toContainText(
    "Stay in flow",
  );
  for (const [name, product] of [
    ["Claude Code", "claude"],
    ["Codex CLI · beta", "codex"],
    ["Both agents", "both"],
  ]) {
    await page.getByRole("button", { name, exact: true }).click();
    for (const os of ["macos", "linux", "windows"]) {
      await chooseOS(page, os);
      for (const intent of ["Install", "Update"]) {
        await page.getByRole("button", { name: intent, exact: true }).click();
        await expect(page.getByLabel(intent + " command")).toHaveValue(
          "curl -fsSL https://raw.githubusercontent.com/777genius/claude-notifications-go/main/bin/bootstrap.sh | bash -s -- --product " +
            product,
        );
      }
      if (os === "windows")
        await expect(
          page.getByText("Run in Git Bash on Windows.", { exact: true }),
        ).toBeVisible();
    }
    if (product !== "claude")
      await expect(
        page.getByText("Codex CLI beta — release prerequisite.", {
          exact: true,
        }),
      ).toBeVisible();
    await page.getByRole("button", { name: "Configure", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Copy command" }),
    ).toHaveCount(0);
    if (product === "codex")
      await expect(
        page.getByText("/claude-notifications-go:settings", { exact: true }),
      ).toHaveCount(0);
  }
  await page.getByRole("button", { name: "Install", exact: true }).click();
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Copy command" }).click();
  await expect(page.getByRole("status")).toContainText("Copied");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toContain(
    "--product both",
  );
  await page.evaluate(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("denied");
        },
      },
    });
  });
  await page.getByRole("button", { name: "Copy command" }).click();
  await expect(page.getByRole("status")).toContainText("Copy unavailable");
  await expect(page.getByLabel("Install command")).toBeFocused();
  expect(errors).toEqual([]);
});
test("unknown target, manual route and mobile layout", async ({ browser }) => {
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    userAgent: "unknown",
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  await page.goto("http://127.0.0.1:4173/claude-notifications-go/");
  await expect(page.getByRole("button", { name: "Copy command" })).toHaveCount(
    0,
  );
  await chooseOS(page, "manual");
  await expect(
    page.getByRole("link", { name: "Manual Claude installation", exact: true }),
  ).toBeVisible();
  await chooseOS(page, "windows");
  await expect(
    page.getByRole("button", { name: "Copy command" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.evaluate(() => window.scrollTo({top: 0, behavior: "instant"}));
  await page.screenshot({ path: "test-results/mobile.png", fullPage: true });
  await context.close();
});
test("keyboard navigation, base-path reload and desktop screenshot", async ({
  page,
}) => {
  await page.goto("");
  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("link", { name: "Skip to content" }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await page.reload();
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  for (const asset of await page
    .locator("script[src]")
    .evaluateAll((nodes) => nodes.map((n) => (n as HTMLScriptElement).src)))
    expect(asset).toContain("/claude-notifications-go/");
  await page.screenshot({ path: "test-results/desktop.png", fullPage: true });
});
test("pending clipboard completion cannot claim a different command was copied", async ({
  page,
}) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: () =>
          new Promise<void>((resolve) => {
            (window as any).finishCopy = resolve;
          }),
      },
    }),
  );
  await page.goto("");
  await chooseOS(page, "linux");
  await page.getByRole("button", { name: "Copy command" }).click();
  await page.getByRole("button", { name: "Both agents", exact: true }).click();
  await page.evaluate(() => (window as any).finishCopy());
  await expect(page.getByRole("status")).not.toContainText("Copied");
  await expect(page.getByLabel("Install command")).toHaveValue(
    /--product both$/,
  );
});
test("assets load, hydration is clean and reduced motion disables background animation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error" || /hydration/i.test(msg.text()))
      errors.push(msg.text());
  });
  page.on("response", (r) => {
    if (r.status() >= 400 && r.url().includes("127.0.0.1"))
      errors.push(r.url());
  });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("");
  await page.waitForLoadState("networkidle");
  expect(
    await page
      .locator(".page-bg__orb")
      .evaluateAll((nodes) =>
        nodes.every((n) => getComputedStyle(n).animationName === "none"),
      ),
  ).toBe(true);
  expect(
    await page.locator("h1").evaluate((n) => getComputedStyle(n).fontSize),
  ).not.toBe("32px");
  expect(errors).toEqual([]);
});
test("notification sequence covers statuses and agents, pause and reduced motion", async ({
  page,
}) => {
  await page.clock.install();
  await page.goto("");
  await page.getByRole("button", { name: "Pause", exact: true }).click();
  const cards = page.locator(".notification-card");
  const initial = await cards.allTextContents();
  await page.clock.fastForward(7000);
  expect(await cards.allTextContents()).toEqual(initial);
  await page.getByRole("button", { name: "Resume", exact: true }).click();
  const seen = new Set<string>();
  for (let i = 0; i < 7; i++) {
    for (const title of await cards.locator("h3").allTextContents())
      seen.add(title);
    await page.clock.fastForward(3400);
    await page.clock.runFor(600);
  }
  expect([...seen].sort()).toEqual(
    [
      "❓ Question",
      "📋 Plan",
      "✅ Completed",
      "🔍 Review",
      "🔐 Permission Request",
      "⏱️ Session Limit Reached",
      "🔴 API Error: 401",
    ].sort(),
  );
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(
    page.getByRole("button", { name: "Next", exact: true }),
  ).toBeVisible();
  const staticCards = await cards.allTextContents();
  await page.clock.fastForward(10000);
  expect(await cards.allTextContents()).toEqual(staticCards);
  await page.getByRole("button", { name: "Next", exact: true }).click();
  expect(await cards.allTextContents()).not.toEqual(staticCards);
});
test("installation order, sticky header and custom select keyboard behavior", async ({
  page,
}) => {
  await page.goto("");
  expect(
    await page
      .locator("main > *")
      .evaluateAll((nodes) =>
        nodes.map((n) => n.id || n.className).slice(0, 2),
      ),
  ).toEqual(["hero-wrap", "install"]);
  for (const title of await page.locator("h1,h2,h3").allTextContents())
    expect(title).not.toContain(".");
  const select = page.getByRole("combobox", {
    name: "02 / Target operating system",
  });
  await select.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("option", { name: "Linux", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Home");
  await expect(
    page.getByRole("option", { name: "Choose target OS", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(select).toContainText("macOS");
  await expect(select).toBeFocused();
  expect(
    await page
      .locator(".header")
      .evaluate((n) => Math.abs(n.getBoundingClientRect().top)),
  ).toBeLessThan(1);
  for (const logo of await page
    .locator(".agent-logo")
    .evaluateAll((nodes) =>
      nodes.map((n) => (n as HTMLImageElement).naturalWidth),
    ))
    expect(logo).toBeGreaterThan(0);
});

test('hero headline remains a single unclipped line at narrow widths', async ({ page }) => {
  for (const width of [320, 390, 768, 1024, 1280]) {
    await page.setViewportSize({width, height: 900});
    await page.goto('');
    const box = await page.locator('h1 em').boundingBox();
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    expect(box!.height).toBeLessThan(60);
  }
});
