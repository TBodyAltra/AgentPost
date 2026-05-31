import { test } from "@playwright/test";
import path from "node:path";

const screenshotPath =
  process.env.SCREENSHOT_PATH ||
  path.join(process.cwd(), "..", "docs", "images", "dashboard.png");

test("capture dashboard screenshot for GitHub Pages", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("agentpost_dashboard_lang", "zh-hans");
    localStorage.setItem("agentpost_dashboard_theme", "dark");
    document.documentElement.setAttribute("data-theme", "dark");
  });

  // Wide enough for sidebar + matrix + open detail drawer (matches marketing screenshot).
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/dashboard/");
  await page.waitForSelector("#stat-mb", { timeout: 15_000 });
  await page.waitForSelector(".matrix-table", { timeout: 15_000 });

  await page.locator('.mailbox-item[data-email="lab-coordinator@atlas.institute"]').click();
  await page.locator("#detail-panel.open").waitFor({ timeout: 5000 });
  await page.locator('#detail-tabs button[data-tab="inbox"]').click();
  await page.locator(".inbox-item").first().waitFor({ timeout: 5000 });
  // Keep detail drawer open (matrix cell click would clear mailbox selection).

  await page.screenshot({
    path: screenshotPath,
    fullPage: false,
    animations: "disabled",
  });
});
