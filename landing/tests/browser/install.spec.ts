import { test, expect } from "@playwright/test";
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
      await page.getByLabel("02 / Target operating system").selectOption(os);
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
  await page.getByLabel("02 / Target operating system").selectOption("manual");
  await expect(
    page.getByRole("link", { name: "Manual Claude installation", exact: true }),
  ).toBeVisible();
  await page.getByLabel("02 / Target operating system").selectOption("windows");
  await expect(
    page.getByRole("button", { name: "Copy command" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
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
  await page.getByLabel("02 / Target operating system").selectOption("linux");
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
