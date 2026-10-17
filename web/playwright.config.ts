import { defineConfig } from '@playwright/test';
import { createECDH } from 'node:crypto';
const pushKeys = createECDH('prime256v1'); pushKeys.generateKeys();
export default defineConfig({
  testDir: './e2e', fullyParallel: false, workers: 1, retries: 0,
  timeout: 45000, expect: { timeout: 10000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: { baseURL: process.env.P1_BASE_URL || 'http://127.0.0.1:8083', viewport: { width: 1440, height: 900 }, locale: 'ru-RU', timezoneId: 'Europe/Moscow', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  webServer: process.env.P1_BASE_URL ? undefined : {
    command: 'go run ./cmd/server', cwd: '..', url: 'http://127.0.0.1:8083/health', timeout: 120000,
    env: { APP_ADDR: '127.0.0.1:8083', DATABASE_URL: process.env.TEST_DATABASE_URL || 'postgres://rhythm@127.0.0.1:5433/rhythm?sslmode=disable', SESSION_SECRET: 'e2e-only-secret-at-least-32-characters', PUBLIC_URL: 'http://127.0.0.1:8083', AUTH_RATE_LIMIT: '240', GENA_NLU_MODE: 'semantic_plan_legacy', GENA_SEMANTIC_PROVIDER: 'rules', NLP_PROVIDER: 'rules', AUTO_IMPORT_DEMO: 'true', APP_TIMEZONE: 'Europe/Moscow', YANDEX_CLIENT_ID: '', YANDEX_CLIENT_SECRET: '', YANDEX_REDIRECT_URI: '', WEB_PUSH_VAPID_PUBLIC_KEY: pushKeys.getPublicKey().toString('base64url'), WEB_PUSH_VAPID_PRIVATE_KEY: pushKeys.getPrivateKey().toString('base64url'), WEB_PUSH_SUBJECT: 'mailto:e2e@example.test' },
    reuseExistingServer: false,
  },
});
