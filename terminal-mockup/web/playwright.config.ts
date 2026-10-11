import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  use: {
    baseURL: 'http://127.0.0.1:8080',
    headless: true,
    launchOptions: process.env.PLAYWRIGHT_CHROME_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROME_PATH }
      : undefined,
  },
  webServer: {
    command: 'go run . -static ../web/dist',
    cwd: '../server',
    url: 'http://127.0.0.1:8080/api/status',
    reuseExistingServer: false,
    env: { AI_GATEWAY_API_KEY: '' },
  },
})
