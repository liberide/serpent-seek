/**
 * Shared date-range presets and helpers for the analytics screens.
 * All ranges are UTC calendar dates (YYYY-MM-DD), matching the backend.
 */

export type DateRange = { from: string; to: string };

export const ANALYTICS_PRESETS = [
	{ key: 'today', label: 'analytics.presets.today' },
	{ key: 'yesterday', label: 'analytics.presets.yesterday' },
	{ key: 'last7', label: 'analytics.presets.last7' },
	{ key: 'last30', label: 'analytics.presets.last30' },
	{ key: 'thisMonth', label: 'analytics.presets.thisMonth' },
	{ key: 'lastMonth', label: 'analytics.presets.lastMonth' },
	{ key: 'thisYear', label: 'analytics.presets.thisYear' }
] as const;

export type PresetKey = (typeof ANALYTICS_PRESETS)[number]['key'];

function iso(date: Date): string {
	return date.toISOString().slice(0, 10);
}

function addDays(base: Date, days: number): Date {
	const next = new Date(base);
	next.setUTCDate(next.getUTCDate() + days);
	return next;
}

function todayUTC(): Date {
	const now = new Date();
	return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
}

/** Computes the UTC date range covered by a preset key. */
export function presetRange(key: string): DateRange {
	const today = todayUTC();
	const year = today.getUTCFullYear();
	const month = today.getUTCMonth();
	switch (key) {
		case 'today':
			return { from: iso(today), to: iso(today) };
		case 'yesterday': {
			const y = addDays(today, -1);
			return { from: iso(y), to: iso(y) };
		}
		case 'last7':
			return { from: iso(addDays(today, -6)), to: iso(today) };
		case 'last30':
			return { from: iso(addDays(today, -29)), to: iso(today) };
		case 'lastMonth': {
			const start = new Date(Date.UTC(year, month - 1, 1));
			const end = new Date(Date.UTC(year, month, 0));
			return { from: iso(start), to: iso(end) };
		}
		case 'thisYear':
			return { from: iso(new Date(Date.UTC(year, 0, 1))), to: iso(today) };
		case 'thisMonth':
		default:
			return { from: iso(new Date(Date.UTC(year, month, 1))), to: iso(today) };
	}
}