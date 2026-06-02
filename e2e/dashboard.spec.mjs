import { test, expect } from "@playwright/test";

const MAILBOX_COUNT = 4;

async function waitForDashboardReady(page, mailboxCount = MAILBOX_COUNT) {
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
    await waitForDashboardReady(page, MAILBOX_COUNT);
  });

  test("delivery matrix is shown by default", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".matrix-table")).toBeVisible({ timeout: 15_000 });
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
  });

  test("message log panel is visible", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator("#message-log")).toBeVisible();
    await expect(page.locator("#log-title")).toBeVisible();
    await expect(page.locator("#log-clear-btn")).toBeVisible();
  });

  test("mailbox detail opens on selection and shows tabbed sections", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".mailbox-item").first()).toBeVisible();

    await page.locator(".mailbox-item").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
    await expect(page.locator("#detail-hero")).toContainText("@");
    await expect(page.locator("#detail-tabs button")).toHaveCount(4);
    await expect(page.locator(".tab-pane.active")).toBeVisible();

    await page.locator("#detail-close").click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
  });

  test("regex search filters mailboxes and matrix", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".matrix-table")).toBeVisible();

    const search = page.locator("#search-input");
    await search.fill("/^alpha@/");
    await expect(page.locator("#search-hint")).toHaveText(/regex|正則/i);
    await expect(page.locator(".mailbox-item")).toHaveCount(1);
    await expect(page.locator(".matrix-table tbody tr")).toHaveCount(4, { timeout: 5000 });

    await search.fill("/[/");
    await expect(page.locator("#search-hint")).toHaveClass(/err/);
    await expect(page.locator(".mailbox-item")).toHaveCount(0);

    await search.fill("/policy_bl");
    await expect(page.locator("#search-hint")).toHaveClass(/err/);
    await expect(page.locator(".mailbox-item")).toHaveCount(0);

    await search.fill("/^gamma@agent\\.test$/");
    await expect(page.locator("#search-hint")).not.toHaveClass(/err/);
    await expect(page.locator(".mailbox-item")).toHaveCount(1);
  });

  test("search shows matched mailboxes and delivery peers in matrix", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".matrix-table")).toBeVisible();

    const search = page.locator("#search-input");
    await search.fill("alpha");
    await expect(page.locator(".matrix-table tbody tr")).toHaveCount(4, { timeout: 5000 });
    await expect(page.locator(".matrix-table thead th.col-header")).toHaveCount(4);
    await expect(page.locator(".matrix-table td.cell-allowed").first()).toBeVisible();
  });

  test("matrix highlights row and column; cell vs header selection", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".matrix-table")).toBeVisible();

    await page.locator(".mailbox-item").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
    await expect(page.locator(".matrix-table th.row-header.axis-highlight")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.col-header.axis-highlight")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.domain-header.axis-highlight")).toHaveCount(0);

    await page.locator("#detail-close").click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);

    const emptyCell = page.locator(".matrix-table td.cell-empty").first();
    await emptyCell.click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
    await expect(page.locator(".matrix-table th.row-header.axis-highlight")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.col-header.axis-highlight")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.domain-header.axis-highlight")).toHaveCount(0);
    await expect(page.locator(".matrix-table td.axis-highlight").first()).toBeVisible();

    const allowedCell = page.locator(".matrix-table td.cell-allowed").first();
    await allowedCell.click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
    await expect(page.locator(".matrix-table td.cell-focus")).toHaveCount(1);

    const diagonalCell = page.locator(".matrix-table td.cell-self").first();
    await diagonalCell.click();
    await expect(page.locator("#detail-panel")).not.toHaveClass(/open/);
    await expect(page.locator(".matrix-table td.cell-self.cell-focus")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.row-header.axis-highlight")).toHaveCount(1);
    await expect(page.locator(".matrix-table th.col-header.axis-highlight")).toHaveCount(1);

    await page.locator(".matrix-table th.row-header").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
  });

  test("matrix groups rows and columns by domain with merged headers", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".matrix-table th.domain-header")).toHaveCount(4, { timeout: 5000 });
    await expect(page.locator(".matrix-table th.domain-col")).toHaveCount(2);
    await expect(page.locator(".matrix-table th.domain-row")).toHaveCount(2);
    await expect(page.locator(".matrix-table th.domain-col").first()).toHaveAttribute("colspan", "3");
    await expect(page.locator(".matrix-table th.domain-row").first()).toHaveAttribute("rowspan", "3");
    await expect(page.locator(".matrix-table th.domain-col").nth(1)).toContainText("partner.test");
    await expect(page.locator(".matrix-table .domain-block-start").first()).toBeVisible();
  });

  test("detail connections tab lists allowed peers only", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await page.locator(".mailbox-item").first().click();
    await expect(page.locator("#detail-panel")).toHaveClass(/open/);
    await page.locator('#detail-tabs button[data-tab="connections"]').click();
    await expect(page.locator('.tab-pane[data-pane="connections"].active')).toBeVisible();
    await expect(page.locator("#detail-content")).not.toContainText("Inbox Policy");
  });

  test("mailbox list shows polling activity after agent polls inbox", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    const beta = page.locator('.mailbox-item[data-email="beta@agent.test"]');
    await expect(beta).toBeVisible();
    await expect(beta.locator(".activity-dot.online")).toBeVisible();
    await expect(beta.locator(".mailbox-activity")).toContainText(/在线|在線|Online/i);
    await expect(page.locator("#stat-poll")).toHaveText("1");
  });

  test("language and refresh controls respond", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator("#lang-seg")).toBeVisible();

    await page.locator('#lang-seg button[data-lang="en"]').click();
    await expect(page.locator("#topology-title")).toHaveText(/matrix/i);

    await page.locator("#refresh-btn").click();
    await expect(page.locator("#refresh-btn")).not.toHaveClass(/spinning/, { timeout: 10_000 });
    await expect(page.locator(".toast.err")).toHaveCount(0);
  });

  test("message log hover shows decoded markdown body", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);
    await expect(page.locator(".log-table .log-row").first()).toBeVisible({ timeout: 15_000 });

    const row = page.locator(".log-table .log-row").first();
    await row.hover();
    const tip = page.locator("#log-body-tooltip");
    await expect(tip).not.toHaveClass(/hidden/);
    await expect(tip).toContainText("目标");
    await expect(tip).not.toContainText("\\u76ee");
    await expect(tip.locator(".md-ol li")).toHaveCount(2);

    await tip.hover();
    await expect(tip).not.toHaveClass(/hidden/);
    await tip.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    await expect(tip).not.toHaveClass(/hidden/);
  });

  test("refresh keeps stable KPI values without rolling from zero", async ({ page }) => {
    await page.goto("/dashboard/");
    await waitForDashboardReady(page, MAILBOX_COUNT);

    const mb = page.locator("#stat-mb");
    await expect(mb).toHaveAttribute("data-v", String(MAILBOX_COUNT));
    const before = await mb.textContent();

    await page.locator("#refresh-btn").click();
    await expect(page.locator("#refresh-btn")).not.toHaveClass(/spinning/, { timeout: 10_000 });

    await expect(mb).toHaveAttribute("data-v", String(MAILBOX_COUNT));
    await expect(mb).toHaveText(before ?? "2");
    await expect(mb).not.toHaveText("0");
  });
});
