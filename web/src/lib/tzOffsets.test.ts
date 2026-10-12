import { describe, expect, it } from 'vitest';
import { TZ_OFFSETS, formatTzOffset } from './tzOffsets';

describe('TZ_OFFSETS', () => {
	it('covers the full real-world offset range', () => {
		expect(TZ_OFFSETS[0].offset).toBe(-720); // UTC-12:00
		expect(TZ_OFFSETS[TZ_OFFSETS.length - 1].offset).toBe(840); // UTC+14:00
	});

	it('is sorted west to east and has unique offsets', () => {
		const offsets = TZ_OFFSETS.map((o) => o.offset);
		expect(new Set(offsets).size).toBe(offsets.length);
		for (let i = 1; i < offsets.length; i++) {
			expect(offsets[i]).toBeGreaterThan(offsets[i - 1]);
		}
	});

	it('includes the half-hour and 45-minute zones', () => {
		const offsets = TZ_OFFSETS.map((o) => o.offset);
		for (const rare of [-570, 330, 345, 525, 570, 765]) {
			expect(offsets).toContain(rare);
		}
	});

	it('keeps every offset inside the backend validation range', () => {
		for (const o of TZ_OFFSETS) {
			expect(o.offset).toBeGreaterThanOrEqual(-720);
			expect(o.offset).toBeLessThanOrEqual(840);
		}
	});

	it('labels every entry with a readable UTC offset', () => {
		for (const o of TZ_OFFSETS) {
			expect(o.label).toContain(formatTzOffset(o.offset));
		}
	});
});

describe('formatTzOffset', () => {
	it('formats positive, negative and zero offsets', () => {
		expect(formatTzOffset(0)).toBe('(UTC+00:00)');
		expect(formatTzOffset(330)).toBe('(UTC+05:30)');
		expect(formatTzOffset(-300)).toBe('(UTC-05:00)');
		expect(formatTzOffset(345)).toBe('(UTC+05:45)');
	});
});
