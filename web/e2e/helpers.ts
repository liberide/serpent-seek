import type { Page } from '@playwright/test';

// stubSession renders the app as an authenticated admin without a backend.
export async function stubSession(page: Page): Promise<void> {
	await page.route('**/version', (route) => route.fulfill({ body: '"1.1.0-test"' }));
	await page.route('**/api/update', (route) => route.fulfill({ json: {} }));
	await page.route('**/api/setup/status', (route) =>
		route.fulfill({
			json: { auth_enabled: true, needs_setup: false, auth_off_notice_hidden: true }
		})
	);
	await page.route('**/api/me', (route) =>
		route.fulfill({
			json: {
				via: 'key',
				admin: true,
				scopes: { admin: true, read: true, search: true },
				passkeys_enabled: false,
				user: { id: 'u1', name: 'admin', role: 'admin' }
			}
		})
	);
	// The live feed is admin-only; answer with an empty event stream.
	await page.route('**/api/events', (route) =>
		route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': connected\n\n' })
	);
	await page.route('**/api/users/u1/passkeys', (route) => route.fulfill({ json: [] }));
}
