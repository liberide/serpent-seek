import { expect, test } from '@playwright/test';
import { stubSession } from './helpers';

// The collapsed rail must keep the build version and the "Support the project"
// entry visible at the bottom, and remember the choice on reload.
test('sidebar collapses to an icon rail and keeps the version + support entry', async ({ page }) => {
	await stubSession(page);
	await page.goto('/');

	const sidebar = page.locator('aside');
	await expect(sidebar).toBeVisible();
	// Navigation entries use inline SVG glyphs, not emoji.
	await expect(sidebar.locator('nav svg').first()).toBeVisible();
	// Expanded: the version sits at the bottom of the menu, next to the author.
	await expect(sidebar.getByRole('link', { name: /v1\.1\.0-test/ })).toBeVisible();

	// Collapse the rail.
	await sidebar.getByRole('button', { name: 'Collapse menu' }).click();
	await expect(sidebar.getByRole('button', { name: 'Expand menu' })).toBeVisible();

	// Navigation labels disappear, the framed footer plaque stays.
	await expect(sidebar.getByText('Dashboard')).toBeHidden();
	await expect(sidebar.locator('.rail-badge')).toBeVisible();
	await expect(sidebar.locator('.support-heart')).toBeVisible();
	await expect(sidebar.getByRole('button', { name: 'Support the project' })).toBeVisible();
	await expect(sidebar.getByRole('link', { name: /v1\.1\.0-test/ })).toBeVisible();

	// The collapsed choice is persisted.
	await page.reload();
	await expect(page.locator('aside').getByRole('button', { name: 'Expand menu' })).toBeVisible();
});

// Hovering a plain rail icon reveals the section name; grouped items show the
// fly-out instead, so no separate tooltip is stacked on top of it.
test('collapsed rail shows the section name on hover', async ({ page }) => {
	await stubSession(page);
	await page.goto('/');
	const sidebar = page.locator('aside');
	await sidebar.getByRole('button', { name: 'Collapse menu' }).click();

	await sidebar.getByRole('link', { name: 'History' }).hover();
	const tip = page.locator('.collapsed-tip');
	await expect(tip).toBeVisible();
	await expect(tip).toHaveText('History');

	// Analytics opens its fly-out instead of a plain name tip.
	await sidebar.getByRole('link', { name: 'Analytics' }).hover();
	await expect(tip).toBeHidden();
	await expect(page.locator('.collapsed-flyout')).toBeVisible();
});

// A grouped item (Analytics) has no room for its sections on the icon rail, so
// it opens a fly-out submenu next to the rail on hover.
test('collapsed rail opens a fly-out submenu for grouped items', async ({ page }) => {
	await stubSession(page);
	await page.route('**/api/stats/analytics*', (route) =>
		route.fulfill({
			json: {
				range: { from: '2026-10-01', to: '2026-10-11' },
				report: { total_requests: 0, daily: [], hourly: [], top_queries: [] },
				providers: []
			}
		})
	);
	await page.route('**/api/stats/users*', (route) =>
		route.fulfill({
			json: {
				range: { from: '2026-10-01', to: '2026-10-11' },
				rows: [],
				totals: { requests: 0, ok: 0, empty: 0, fail: 0, active_users: 0, cost: 0, cost_configured: false }
			}
		})
	);

	await page.goto('/');
	const sidebar = page.locator('aside');
	await sidebar.getByRole('button', { name: 'Collapse menu' }).click();

	await sidebar.getByRole('link', { name: 'Analytics' }).hover();
	const flyout = page.locator('.collapsed-flyout');
	await expect(flyout.getByRole('link', { name: 'Overview' })).toBeVisible();
	await expect(flyout.getByRole('link', { name: 'By users' })).toBeVisible();

	await flyout.getByRole('link', { name: 'By users' }).click();
	await expect(page).toHaveURL(/\/analytics\/users/);
	await expect(flyout).toBeHidden();
});
