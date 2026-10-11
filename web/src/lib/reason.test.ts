import { describe, expect, it } from 'vitest';
import { providerReason } from './reason';

describe('providerReason', () => {
	it('returns an empty string for empty input', () => {
		expect(providerReason('')).toBe('');
		expect(providerReason(null)).toBe('');
		expect(providerReason(undefined)).toBe('');
	});

	it('localizes known engine codes (never returns the raw code)', () => {
		for (const code of ['provider_disabled', 'searxng_url_missing', 'api_key_missing']) {
			const localized = providerReason(code);
			expect(localized).not.toBe('');
			expect(localized).not.toBe(code);
		}
	});

	it('renders the unknown driver name', () => {
		const localized = providerReason('unknown_driver:my_driver');
		expect(localized).toContain('my_driver');
		expect(localized).not.toBe('unknown_driver:my_driver');
	});

	it('passes through real upstream errors unchanged', () => {
		expect(providerReason('HTTP 500: upstream exploded')).toBe('HTTP 500: upstream exploded');
	});
});
