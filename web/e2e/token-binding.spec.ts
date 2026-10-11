import { expect, test } from '@playwright/test';
import { stubSession } from './helpers';

test('binding a token to a chain PATCHes the key', async ({ page }) => {
	await stubSession(page);

	await page.route('**/api/users', (route) =>
		route.fulfill({ json: [{ id: 'u1', name: 'admin', role: 'admin', disabled: false, created_at: '' }] })
	);
	await page.route('**/api/chains', (route) =>
		route.fulfill({
			json: [{ id: 'c1', name: 'My Chain', active: true, version: 1, nodes: [], edges: [] }]
		})
	);
	await page.route('**/api/keys', (route) =>
		route.fulfill({
			json: [
				{
					id: 'k1',
					prefix: 'seek_ak_abc',
					name: 'my key',
					scopes: 'search',
					chain_id: '',
					revoked_at: '',
					last_used_at: '',
					used_today: 0
				}
			]
		})
	);
	await page.route('**/api/keys/k1', (route) =>
		route.fulfill({ json: { id: 'k1', prefix: 'seek_ak_abc', name: 'my key', chain_id: 'c1' } })
	);

	await page.goto('/users');
	await page.getByRole('button', { name: /active chain/i }).click();
	await page.locator('#token-chain').selectOption('c1');

	const patched = page.waitForRequest(
		(request) => request.method() === 'PATCH' && request.url().endsWith('/api/keys/k1')
	);
	await page.getByRole('button', { name: /^save$/i }).click();

	const body = (await patched).postDataJSON();
	expect(body.chain_id).toBe('c1');
});
