import { expect, test } from '@playwright/test';
import { stubSession } from './helpers';

// Leaderboard fixture: one admin with a quota, one dormant viewer and the
// anonymous bucket, mirroring GET /api/stats/users.
const leaderboard = {
	range: { from: '2026-10-01', to: '2026-10-11' },
	rows: [
		{
			user_id: 'u1',
			name: 'admin',
			role: 'admin',
			disabled: false,
			created_at: '2026-09-01T00:00:00Z',
			exists: true,
			requests: 12,
			ok: 10,
			empty: 1,
			fail: 1,
			avg_ms: 240,
			last_active: '2026-10-11T10:00:00Z',
			keys_count: 2,
			quota: {
				requests: 50,
				period: 'daily',
				window_hours: 24,
				tz_offset: 0,
				used: 12,
				remaining: 38,
				reset_at: '2026-10-12T00:00:00Z'
			}
		},
		{
			user_id: 'u2',
			name: 'viewer',
			role: 'viewer',
			disabled: false,
			created_at: '2026-09-02T00:00:00Z',
			exists: true,
			requests: 0,
			ok: 0,
			empty: 0,
			fail: 0,
			avg_ms: 0,
			keys_count: 1
		},
		{
			user_id: 'anonymous',
			name: '',
			exists: true,
			requests: 3,
			ok: 3,
			empty: 0,
			fail: 0,
			avg_ms: 90,
			last_active: '2026-10-10T09:00:00Z',
			keys_count: 0
		}
	],
	totals: {
		requests: 15,
		ok: 13,
		empty: 1,
		fail: 1,
		active_users: 1,
		cost: 0,
		cost_configured: false
	}
};

const userReport = {
	range: { from: '2026-10-01', to: '2026-10-11' },
	providers: [
		{
			provider: 'Stub',
			code: 'stub_ok',
			enabled: true,
			configured: true,
			total: 12,
			ok: 10,
			empty: 1,
			fail: 1,
			skip: 0,
			avg_ms: 240,
			success_rate: 83.3
		}
	],
	report: {
		total_requests: 12,
		ok: 10,
		empty: 1,
		fail: 1,
		success_rate: 83.3,
		avg_ms: 240,
		p95_ms: 500,
		active_days: 5,
		busiest_date: '2026-10-11',
		busiest_count: 4,
		daily: [{ date: '2026-10-11', requests: 4, ok: 3, empty: 0, fail: 1, avg_ms: 240 }],
		hourly: Array.from({ length: 24 }, (_, hour) => ({
			hour,
			requests: hour === 10 ? 4 : 0,
			ok: hour === 10 ? 4 : 0,
			empty: 0,
			fail: 0
		})),
		top_queries: [{ query: 'e2e query', count: 4, ok: 4, fail: 0 }],
		cost_configured: false
	}
};

test('analytics sections are selectable from the sidebar', async ({ page }) => {
	await stubSession(page);
	await page.route('**/api/stats/summary', (route) =>
		route.fulfill({
			json: {
				summary: {
					requests_today: 0,
					success_rate: 0,
					avg_ms: 0,
					errors_today: 0,
					recent: [],
					daily: []
				}
			}
		})
	);
	await page.route('**/api/stats/analytics*', (route) =>
		route.fulfill({
			json: { ...userReport, report: { ...userReport.report, total_requests: 0 }, providers: [] }
		})
	);
	await page.route('**/api/stats/users*', (route) => route.fulfill({ json: leaderboard }));

	// The parent item navigates to the overview; the group auto-expands.
	await page.goto('/');
	await page.locator('aside nav').getByRole('link', { name: 'Analytics' }).click();
	await expect(page).toHaveURL(/\/analytics$/);
	const sidebar = page.locator('aside nav');
	await expect(sidebar.getByRole('link', { name: 'Overview' })).toBeVisible();
	await expect(sidebar.getByRole('link', { name: 'By users' })).toBeVisible();

	// The child link opens the per-user section.
	await sidebar.getByRole('link', { name: 'By users' }).click();
	await expect(page).toHaveURL(/\/analytics\/users/);
	await expect(page.getByText('Active users')).toBeVisible();

	// Collapsing the group hides the sections again.
	await sidebar.getByRole('button', { name: 'Analytics' }).click();
	await expect(sidebar.getByRole('link', { name: 'Overview' })).toBeHidden();
});

test('users leaderboard lists owners with quota and share', async ({ page }) => {
	await stubSession(page);
	await page.route('**/api/stats/users*', (route) => route.fulfill({ json: leaderboard }));
	await page.route('**/api/stats/analytics*', (route) =>
		route.fulfill({ json: { ...userReport, providers: [] } })
	);

	await page.goto('/analytics/users');
	// Totals strip.
	await expect(page.getByText('Active users')).toBeVisible();
	// Rows: names, quota badge and the anonymous bucket.
	await expect(page.getByRole('cell', { name: 'admin' })).toBeVisible();
	await expect(page.getByRole('cell', { name: 'Anonymous' })).toBeVisible();
	await expect(page.getByText('38/50 · day')).toBeVisible();
	// Dormant users are listed too.
	await expect(page.getByRole('cell', { name: 'viewer' })).toBeVisible();
});

test('selecting a user opens the per-user graphs and back returns to the list', async ({
	page
}) => {
	await stubSession(page);
	await page.route('**/api/stats/users*', (route) => route.fulfill({ json: leaderboard }));
	let detailCalls = 0;
	await page.route('**/api/stats/analytics*', (route) => {
		detailCalls++;
		route.fulfill({ json: userReport });
	});

	await page.goto('/analytics/users');
	await page.getByRole('cell', { name: 'admin' }).click();

	// The URL mirrors the selection so the view is shareable.
	await expect(page).toHaveURL(/\/analytics\/users\?user=u1/);
	// Quota usage card.
	await expect(page.getByText('🚦 Quota · Daily')).toBeVisible();
	// KPIs from the per-user report.
	await expect(page.getByText('95th percentile')).toBeVisible();
	await expect(page.getByText('e2e query')).toBeVisible();
	expect(detailCalls).toBeGreaterThan(0);

	// Back returns to the leaderboard.
	await page.getByTitle('Back to all users').click();
	await expect(page).toHaveURL(/\/analytics\/users$/);
	await expect(page.getByRole('cell', { name: 'viewer' })).toBeVisible();
});