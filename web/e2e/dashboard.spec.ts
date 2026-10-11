import { expect, test } from '@playwright/test';
import { stubSession } from './helpers';

test('dashboard renders the summary after sign-in', async ({ page }) => {
	await stubSession(page);
	await page.route('**/api/stats/summary', (route) =>
		route.fulfill({
			json: {
				summary: {
					requests_today: 3,
					success_rate: 100,
					avg_ms: 42,
					errors_today: 0,
					recent: [],
					daily: []
				}
			}
		})
	);

	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
	await expect(page.getByText('Requests today')).toBeVisible();
});
