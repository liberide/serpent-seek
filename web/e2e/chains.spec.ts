import { expect, test } from '@playwright/test';
import { stubSession } from './helpers';

test('creating a chain posts a logical Start block and opens the editor', async ({ page }) => {
	await stubSession(page);
	let posted: { nodes?: Array<{ kind?: string }>; edges?: unknown[] } | undefined;

	await page.route('**/api/providers', (route) =>
		route.fulfill({ json: [{ id: 'p1', code: 'stub_ok', name: 'Stub', enabled: true }] })
	);
	await page.route('**/api/chains', async (route) => {
		if (route.request().method() === 'POST') {
			posted = route.request().postDataJSON();
			await route.fulfill({
				json: { id: 'new-chain', name: 'New chain', mode: 'first_success', version: 1, active: false, nodes: [], edges: [] }
			});
			return;
		}
		await route.fulfill({ json: [] });
	});
	await page.route('**/api/chains/new-chain', (route) =>
		route.fulfill({
			json: { id: 'new-chain', name: 'New chain', mode: 'first_success', version: 1, active: false, nodes: [], edges: [] }
		})
	);

	await page.goto('/chains');
	await page.getByRole('button', { name: /create chain/i }).click();
	await page.waitForURL('**/chains/new-chain');

	expect(posted?.nodes?.some((node) => node.kind === 'start')).toBe(true);
	expect(posted?.edges?.length).toBe(1);
});
