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

  test("mailbox detail opens on selection and shows tabbed sections", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await expect(page.locator(".mailbox-item").first()).toBeVisible();

    await page.locator(".mailbox-item").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
    await expect(page.locator("#detail-hero")).toContainText("@");
    await expect(page.locator("#detail-tabs button")).toHaveCount(4);
    await expect(page.locator(".tab-pane.active")).toBeVisible();

    await page.locator("#detail-close").click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
  });

  test("search shows matched mailboxes and delivery peers in matrix", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await expect(page.locator(".matrix-table")).toBeVisible();

    const search = page.locator("#search-input");
    await search.fill("alpha");
    await expect(page.locator(".matrix-table tbody tr")).toHaveCount(2, { timeout: 5000 });
    await expect(page.locator(".matrix-table thead th.col-header")).toHaveCount(2);
    await expect(page.locator(".matrix-table td.cell-allowed").first()).toBeVisible();
  });

  test("matrix highlights row and column for mailbox and cell selection", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await expect(page.locator(".matrix-table")).toBeVisible();

    await page.locator(".mailbox-item").first().click();
    await expect(page.locator(".matrix-table th.axis-highlight")).toHaveCount(2);
    await expect(page.locator(".matrix-table td.axis-highlight").first()).toBeVisible();

    const cell = page.locator(".matrix-table td.cell-allowed").first();
    await cell.click();
    await expect(page.locator(".matrix-table td.cell-focus")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.axis-highlight")).toHaveCount(2);
  });

  test("detail connections tab lists allowed peers only", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);
    await page.locator(".mailbox-item").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
    await page.locator('#detail-tabs button[data-tab="connections"]').click();
    await expect(page.locator('.tab-pane[data-pane="connections"].active')).toBeVisible();
    await expect(page.locator("#detail-content")).not.toContainText("Inbox Policy");
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

  test("refresh keeps stable KPI values without rolling from zero", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, 2);

    const mb = page.locator("#stat-mb");
    await expect(mb).toHaveAttribute("data-v", "2");
    const before = await mb.textContent();

    await page.locator("#refresh-btn").click();
    await expect(page.locator("#refresh-btn")).not.toHaveClass(/spinning/, { timeout: 10_000 });

    await expect(mb).toHaveAttribute("data-v", "2");
    await expect(mb).toHaveText(before ?? "2");
    await expect(mb).not.toHaveText("0");
  });
});
