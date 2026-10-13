import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './e2e', fullyParallel: false, workers: 1, retries: 0,
  timeout: 45000, expect: { timeout: 10000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: { baseURL: process.env.P1_BASE_URL || 'http://127.0.0.1:8083', viewport: { width: 1440, height: 900 }, locale: 'ru-RU', timezoneId: 'Europe/Moscow', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  webServer: process.env.P1_BASE_URL ? undefined : {
    command: 'go run ./cmd/server', cwd: '..', url: 'http://127.0.0.1:8083/health', timeout: 120000,
    env: { APP_ADDR: '127.0.0.1:8083', DATABASE_PATH: 'var/p1-e2e.sqlite3', NLP_PROVIDER: 'rules', AUTO_IMPORT_DEMO: 'true', APP_TIMEZONE: 'Europe/Moscow' },
    reuseExistingServer: false,
  },
});
