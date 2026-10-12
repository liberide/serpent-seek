import { describe, expect, it } from 'vitest';
import { ANALYTICS_PRESETS, presetRange } from './analyticsRange';

describe('presetRange', () => {
	// Ranges are UTC calendar dates, so the assertions are deterministic.
	it('today covers the current UTC day', () => {
		const { from, to } = presetRange('today');
		expect(from).toBe(to);
		expect(from).toMatch(/^\d{4}-\d{2}-\d{2}$/);
	});

	it('yesterday is a single previous day', () => {
		const { from, to } = presetRange('yesterday');
		expect(from).toBe(to);
		const today = presetRange('today').from;
		expect(from).not.toBe(today);
	});

	it('last7 and last30 end today and span the window', () => {
		const today = presetRange('today').to;
		for (const [key, days] of [
			['last7', 7],
			['last30', 30]
		] as const) {
			const { from, to } = presetRange(key);
			expect(to).toBe(today);
			const span =
				(Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / 86_400_000 + 1;
			expect(span).toBe(days);
		}
	});

	it('thisMonth starts on the 1st and ends today', () => {
		const { from, to } = presetRange('thisMonth');
		expect(from.endsWith('-01')).toBe(true);
		expect(to).toBe(presetRange('today').to);
	});

	it('lastMonth is fully in the past', () => {
		const { from, to } = presetRange('lastMonth');
		expect(to < presetRange('today').from).toBe(true);
		expect(from.endsWith('-01')).toBe(true);
	});

	it('thisYear starts on January 1st', () => {
		const { from, to } = presetRange('thisYear');
		expect(from.endsWith('-01-01')).toBe(true);
		expect(to).toBe(presetRange('today').to);
	});

	it('unknown keys fall back to thisMonth', () => {
		expect(presetRange('nope')).toEqual(presetRange('thisMonth'));
	});
});

describe('ANALYTICS_PRESETS', () => {
	it('has unique keys matching the i18n labels', () => {
		const keys = ANALYTICS_PRESETS.map((p) => p.key);
		expect(new Set(keys).size).toBe(keys.length);
		for (const preset of ANALYTICS_PRESETS) {
			expect(preset.label).toBe(`analytics.presets.${preset.key}`);
		}
	});
});