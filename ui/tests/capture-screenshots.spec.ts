import { test } from "@playwright/test";

import { installReplayApiMocks } from "./fixtures/replay-api";

test.beforeEach(async ({ page }) => {
  await installReplayApiMocks(page);
});

test("capture divergence detail screenshot", async ({ page }) => {
  await page.goto(
    "/divergence/trace-candidate-rollback?baseline=trace-baseline-support",
  );
  await page.waitForTimeout(1000);
  await page.screenshot({
    path: "../docs/screenshots/divergence-detail.png",
    fullPage: true,
  });
});

test("capture shadow replay screenshot", async ({ page }) => {
  await page.goto(
    "/shadow-replay?baseline=trace-baseline-support&candidate=trace-candidate-rollback",
  );
  await page.waitForTimeout(1000);
  await page.screenshot({
    path: "../docs/screenshots/shadow-replay.png",
    fullPage: true,
  });
});

test("capture gauntlet report screenshot", async ({ page }) => {
  await page.goto("/gauntlet/exp-governance-reject");
  await page.waitForTimeout(1000);
  await page.screenshot({
    path: "../docs/screenshots/gauntlet-report.png",
    fullPage: true,
  });
});
