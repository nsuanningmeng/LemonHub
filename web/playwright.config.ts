/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { defineConfig } from '@playwright/test'

const baseURL = process.env.E2E_BASE_URL || 'http://127.0.0.1:5176'

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  outputDir: process.env.E2E_OUTPUT_DIR || 'test-results',
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL,
    viewport: { width: 1920, height: 1080 },
    locale: 'en-US',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    launchOptions: {
      executablePath: process.env.CHROMIUM_PATH,
    },
  },
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: process.env.CI
          ? 'bun run preview --host 127.0.0.1 --port 5176'
          : 'bun run dev --host 127.0.0.1 --port 5176',
        url: baseURL,
        reuseExistingServer: !process.env.CI,
      },
})
