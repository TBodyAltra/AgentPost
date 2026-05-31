import { test, expect } from "@playwright/test";

async function waitForDashboardReady(page, mailboxCount) {
  await expect(page.locator("#stat-mb")).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(async () => Number((await page.locator("#stat-mb").textContent()) ?? "0"), {
      timeout: 15_000,
    })
    .toBe(mailboxCount);
}

test.describe("AgentPost dashboard", () => {
  test("loads without login when gateway token is disabled", async ({ page }) => {
    await page.goto("/dashboard/");
    await expect(page.locator("#login")).toHaveClass(/hidden/);
    await waitForDashboardReady(page, 2);
  });

  test("delivery matrix is shown by default", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await expect(page.locator(".matrix-table")).toBeVisible({ timeout: 15_000 });
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
  });

  test("mailbox detail opens on selection and shows delivery sections", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await expect(page.locator(".mailbox-row").first()).toBeVisible();

    await page.locator(".mailbox-row").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
    await expect(page.locator("#detail-content")).toContainText("@");
    await expect(page.locator(".delivery-block")).toHaveCount(2);

    await page.locator("#detail-close").click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
  });

  test("language and refresh controls respond", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await expect(page.locator("#lang-seg")).toBeVisible();

    await page.locator('#lang-seg button[data-lang="en"]').click();
    await expect(page.locator("#topology-title")).toHaveText(/matrix/i);

    await page.locator("#refresh-btn").click();
    await expect(page.locator("#refresh-btn")).not.toHaveClass(/spinning/, { timeout: 10_000 });
    await expect(page.locator(".toast.err")).toHaveCount(0);
  });
});
