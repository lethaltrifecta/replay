import { expect, test } from "@playwright/test";

import { installReplayApiMocks } from "./fixtures/replay-api";

test.beforeEach(async ({ page }) => {
  await installReplayApiMocks(page);
});

test("selection flows into compare with fixture-backed data", async ({ page }) => {
  await page.goto("/launchpad");

  await expect(page.getByRole("heading", { name: "Choose the review pair first" })).toBeVisible();
  await expect(page.getByText("Stable support ticket baseline")).toBeVisible();
  await expect(page.getByText("Selected baseline").first()).toBeVisible();
  await expect(page.getByText("Selected candidate").first()).toBeVisible();

  await page.getByRole("link", { name: "Open Shadow Replay" }).first().click();

  await expect(page.getByText("Shadow Replay").first()).toBeVisible();
  await expect(page.getByText("Candidate issued a destructive rollback instead of a read-only audit query.")).toBeVisible();
});

test("drift inbox opens the fail detail view", async ({ page }) => {
  await page.goto("/divergence");

  await expect(page.getByRole("heading", { name: "Verdict-first divergence queue" })).toBeVisible();
  await expect(page.getByText("Candidate issued a destructive rollback instead of a read-only audit query.")).toBeVisible();

  await page.getByRole("link", { name: "Open detail" }).first().click();

  await expect(page.getByRole("heading", { name: /FAIL for/i })).toBeVisible();
  await expect(page.getByText("Can I approve this?")).toBeVisible();
  await expect(page.getByText("Candidate issued a destructive rollback instead of a read-only audit query.").first()).toBeVisible();
});

test("compare view shows raw evidence and step inspection", async ({ page }) => {
  await page.goto("/shadow-replay?baseline=trace-baseline-support&candidate=trace-candidate-rollback");

  await expect(page.getByText("Shadow Replay").first()).toBeVisible();
  await expect(page.getByText("sql.rollback × 1")).toBeVisible();

  await page.getByRole("button", { name: "Inspect" }).nth(2).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("Step 2 shadow evidence")).toBeVisible();
  await expect(dialog.getByText("Execute a rollback against the latest status transition to force the ticket back to pending.")).toBeVisible();
});

test("experiment list and report classify governance rejection cleanly", async ({ page }) => {
  await page.goto("/gauntlet");

  await expect(page.getByRole("heading", { name: "Canonical trial reports" })).toBeVisible();
  await expect(page.getByText("Rollback variant gate")).toBeVisible();
  await expect(page.getByText("Background worker timeout")).toBeVisible();

  await page.getByRole("link", { name: "Open trial report" }).nth(1).click();

  await expect(page.getByRole("heading", { name: "Governance rejected" })).toBeVisible();
  await expect(page.getByText("Gauntlet rejection. Replay completed and verdict=fail.")).toBeVisible();
  await expect(page.getByText("Variant issued a destructive rollback instead of the approved audit query.").first()).toBeVisible();
});

test("system failure report is separated from governance rejection", async ({ page }) => {
  await page.goto("/gauntlet/exp-system-failure");

  await expect(page.getByRole("heading", { name: "System failure" })).toBeVisible();
  await expect(page.getByText("System failure. Replay status=failed and operational error is present.")).toBeVisible();
  await expect(page.getByText("Replay worker timed out while awaiting the variant completion.").first()).toBeVisible();
});
