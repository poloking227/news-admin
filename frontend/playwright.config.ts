import { defineConfig, devices } from '@playwright/test'

/**
 * 端到端套件：真实浏览器驱动完整前后端链路。
 * webServer 依次重建独立库 news_admin_e2e（cmd/e2e prep + migrate up + seed，
 * 三账号已知首登口令且 must_change_password=true）→ 启动后端 :8080 → 前端 :5173。
 * 数据库为整个运行共享，用例按文件串行执行（workers=1），用例数据用唯一名称隔离。
 */
const E2E_DATABASE_URL =
  process.env.E2E_DATABASE_URL ??
  'postgres://newsadmin:newsadmin_dev@localhost:5432/news_admin_e2e?sslmode=disable'

const backendEnv = [
  `DATABASE_URL='${E2E_DATABASE_URL}'`,
  "APP_ENV='development'",
  "APP_PORT='8080'",
  "JWT_SECRET='e2e-suite-secret'",
  "CORS_ORIGIN='http://localhost:5173'",
  "LOG_LEVEL='warn'"
].join(' ')

export default defineConfig({
  testDir: './e2e',
  // E2E 库为整个运行共享：串行执行避免用例相互覆盖数据
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 15_000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure'
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: `export ${backendEnv} && cd ../backend && go run ./cmd/e2e prep && go run ./cmd/migrate up && go run ./cmd/e2e seed && go run ./cmd/server`,
      url: 'http://localhost:8080/healthz',
      reuseExistingServer: !process.env.CI,
      timeout: 120_000
    },
    {
      command: 'pnpm dev',
      url: 'http://localhost:5173',
      reuseExistingServer: !process.env.CI,
      timeout: 120_000
    }
  ]
})
