import { defineConfig, devices } from '@playwright/test';

// Browser scenarios for the SvelteKit UI. The specs stub `/api/**` with
// Playwright route interception, so no Go backend is required: the webServer
// only starts the Vite dev server and the SPA talks to the stubs.
export default defineConfig({
	testDir: './e2e',
	timeout: 30_000,
	expect: { timeout: 10_000 },
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
	use: {
		baseURL: 'http://127.0.0.1:5173',
		trace: 'on-first-retry'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: 'npm run dev -- --host 127.0.0.1',
		url: 'http://127.0.0.1:5173',
		reuseExistingServer: !process.env.CI,
		timeout: 120_000
	}
});
