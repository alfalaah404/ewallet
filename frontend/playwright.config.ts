import { defineConfig, devices } from '@playwright/test';

// Ubuntu 26.04 isn't in Playwright 1.60's OS allowlist yet; pin the closest
// supported host string so the bundled Chromium launches without a manual env
// var. Remove once Playwright officially supports this OS.
if (!process.env.PLAYWRIGHT_HOST_PLATFORM_OVERRIDE) {
  process.env.PLAYWRIGHT_HOST_PLATFORM_OVERRIDE = 'ubuntu24.04-x64';
}


// E2E config. Single worker by requirement (npm run test == 1 worker), and
// because tests share one live backend + DB, serial execution avoids cross-test
// interference. Playwright boots `vite preview` (which proxies /api -> :8080).
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
    headless: true,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
  webServer: {
    command: 'npm run build && npm run preview',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
