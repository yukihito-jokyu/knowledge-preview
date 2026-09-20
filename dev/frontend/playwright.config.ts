import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.E2E_BASE_URL ?? "https://app.knowledge.test";

const artifactDir = process.env.CI
  ? "/artifacts"
  : new URL("./e2e/artifacts/", import.meta.url).pathname;

export default defineConfig({
  tsconfig: "./tsconfig.e2e.json",
  testDir: "./e2e",
  testMatch: ["tests/**/*.spec.ts", "contracts/**/*.spec.ts"],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: true,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  outputDir: `${artifactDir}/results`,
  reporter: [
    ["list"],
    ["html", { outputFolder: `${artifactDir}/report`, open: "never" }],
    ["json", { outputFile: `${artifactDir}/results/playwright.json` }],
    ["blob", { outputDir: `${artifactDir}/blob` }],
  ],
  use: {
    baseURL,
    ignoreHTTPSErrors: false,
    trace: "off",
    video: "on",
    screenshot: "off",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    {
      name: "firefox",
      use: { ...devices["Desktop Firefox"] },
    },
    {
      name: "webkit",
      use: { ...devices["Desktop Safari"] },
    },
  ],
});
