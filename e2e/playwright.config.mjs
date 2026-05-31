import { defineConfig } from "@playwright/test";

const baseURL = process.env.BASE_URL || "http://127.0.0.1:8080";

export default defineConfig({
  testDir: ".",
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL,
    headless: true,
    trace: "retain-on-failure",
  },
  reporter: process.env.CI ? [["github"], ["list"]] : [["list"]],
});
