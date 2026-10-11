import { describe, expect, it } from 'vitest';
import { formatCost, formatInt, formatPct, formatUptime, ms, statusTone } from './format';

describe('ms', () => {
	it('formats sub-second values', () => {
		expect(ms(0)).toBe('0ms');
		expect(ms(999)).toBe('999ms');
		expect(ms(null)).toBe('0ms');
		expect(ms(undefined)).toBe('0ms');
	});

	it('formats seconds and minutes', () => {
		expect(ms(1500)).toBe('1.50s');
		expect(ms(15000)).toBe('15.0s');
		expect(ms(65000)).toBe('1m 5s');
	});
});

describe('statusTone', () => {
	it('maps statuses and defaults to gray', () => {
		expect(statusTone('ok')).toBe('ok');
		expect(statusTone('empty')).toBe('empty');
		expect(statusTone('fail')).toBe('fail');
		expect(statusTone('running')).toBe('running');
		expect(statusTone('skip')).toBe('skipped');
		expect(statusTone('skipped')).toBe('skipped');
		expect(statusTone(undefined)).toBe('gray');
		expect(statusTone('weird')).toBe('gray');
	});
});

describe('formatInt / formatPct', () => {
	it('returns an em dash for nullish values', () => {
		expect(formatInt(null)).toBe('—');
		expect(formatInt(undefined)).toBe('—');
		expect(formatPct(null)).toBe('—');
	});

	it('formats numbers and percentages', () => {
		expect(formatInt(1234)).toMatch(/1\D?234/);
		expect(formatPct(12.345, 1)).toMatch(/^12\D3%$/);
	});
});

describe('formatCost', () => {
	it('returns an em dash for nullish values', () => {
		expect(formatCost(null)).toBe('—');
		expect(formatCost(undefined)).toBe('—');
	});

	it('formats with no fixed padding', () => {
		expect(formatCost(0)).toBe('0');
		expect(formatCost(0.25)).toMatch(/0\D25/);
		expect(formatCost(1.5)).toMatch(/1\D5/);
	});
});

describe('formatUptime', () => {
	it('formats seconds, minutes, hours and days', () => {
		expect(formatUptime(0)).toBe('0s');
		expect(formatUptime(12)).toBe('12s');
		expect(formatUptime(65)).toBe('1m 5s');
		expect(formatUptime(3661)).toBe('1h 1m 1s');
		expect(formatUptime(90061)).toBe('1d 1h 1m 1s');
		expect(formatUptime(null)).toBe('—');
	});
});
