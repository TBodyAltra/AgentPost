import { test, expect } from "@playwright/test";

test("login with gateway token shows dashboard data", async ({ page }) => {
  const token = process.env.GATEWAY_TOKEN;
  if (!token) {
    test.skip();
    return;
  }

  await page.goto("/dashboard/");
  await expect(page.locator("#login")).not.toHaveClass(/hidden/);
  await page.locator("#token-input").fill(token);
  await page.locator("#login-btn").click();
  await expect(page.locator("#login")).toHaveClass(/hidden/, { timeout: 15_000 });
  await expect(page.locator("#stat-mb")).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(async () => Number((await page.locator("#stat-mb").textContent()) ?? "0"), {
      timeout: 15_000,
    })
    .toBe(1);
});
