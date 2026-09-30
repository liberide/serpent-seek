/** Formats a duration in milliseconds for display. */
export function ms(value: number | undefined | null): string {
	const n = value ?? 0;
	if (n < 1000) return `${n}ms`;
	if (n < 60000) return `${(n / 1000).toFixed(n < 10000 ? 2 : 1)}s`;
	const minutes = Math.floor(n / 60000);
	const seconds = Math.round((n % 60000) / 1000);
	return `${minutes}m ${seconds}s`;
}

import { getLocale, type Locale } from './i18n.svelte';

const intlLocales: Record<Locale, string> = {
	ru: 'ru-RU',
	en: 'en-US',
	pl: 'pl-PL',
	de: 'de-DE',
	fr: 'fr-FR',
	zh: 'zh-CN'
};

const numberFormatters = new Map<string, Intl.NumberFormat>();

function numberFormatter(locale: Locale, options: Intl.NumberFormatOptions): Intl.NumberFormat {
	const cacheKey = `${locale}:${JSON.stringify(options)}`;
	let formatter = numberFormatters.get(cacheKey);
	if (!formatter) {
		formatter = new Intl.NumberFormat(intlLocales[locale], options);
		numberFormatters.set(cacheKey, formatter);
	}
	return formatter;
}

/** Formats an integer with locale-aware grouping. */
export function formatInt(value: number | undefined | null): string {
	if (value === undefined || value === null || Number.isNaN(value)) return '—';
	return numberFormatter(getLocale(), { maximumFractionDigits: 0 }).format(value);
}

/** Formats a percentage with the given number of fraction digits. */
export function formatPct(value: number | undefined | null, digits = 1): string {
	if (value === undefined || value === null || Number.isNaN(value)) return '—';
	return `${numberFormatter(getLocale(), {
		minimumFractionDigits: digits,
		maximumFractionDigits: digits
	}).format(value)}%`;
}

const dayFormatters = new Map<string, Intl.DateTimeFormat>();

/**
 * Formats a UTC calendar date (YYYY-MM-DD) for charts. Day buckets come from the
 * backend in UTC, so formatting is pinned to UTC to avoid shifting labels.
 */
export function dayLabel(value: string, style: 'short' | 'long' = 'short'): string {
	const parsed = new Date(`${value.slice(0, 10)}T00:00:00Z`);
	if (Number.isNaN(parsed.getTime())) return value;
	const locale = getLocale();
	const options: Intl.DateTimeFormatOptions =
		style === 'short'
			? { day: 'numeric', month: 'short', timeZone: 'UTC' }
			: { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' };
	const key = `${locale}:${style}`;
	let formatter = dayFormatters.get(key);
	if (!formatter) {
		formatter = new Intl.DateTimeFormat(intlLocales[locale], options);
		dayFormatters.set(key, formatter);
	}
	return formatter.format(parsed);
}

const dateFormatters = new Map<Locale, Intl.DateTimeFormat>();

function dateFormatter(locale: Locale): Intl.DateTimeFormat {
	let formatter = dateFormatters.get(locale);
	if (!formatter) {
		formatter = new Intl.DateTimeFormat(intlLocales[locale], {
			dateStyle: 'short',
			timeStyle: 'medium',
			timeZone: 'Europe/Minsk'
		});
		dateFormatters.set(locale, formatter);
	}
	return formatter;
}

/** Formats an RFC3339 timestamp in the active locale (Europe/Minsk). */
export function dateTime(value: string | undefined | null): string {
	if (!value) return '—';
	const parsed = new Date(value);
	if (Number.isNaN(parsed.getTime())) return value;
	return dateFormatter(getLocale()).format(parsed);
}

export type StatusTone = 'ok' | 'empty' | 'fail' | 'running' | 'skipped' | 'gray';

/** Maps a request/step status to a colour tone. */
export function statusTone(status: string | undefined): StatusTone {
	switch (status) {
		case 'ok':
			return 'ok';
		case 'empty':
			return 'empty';
		case 'fail':
			return 'fail';
		case 'running':
			return 'running';
		case 'skip':
		case 'skipped':
			return 'skipped';
		default:
			return 'gray';
	}
}

export const toneClasses: Record<StatusTone, string> = {
	ok: 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/30',
	empty: 'bg-amber-500/15 text-amber-300 border border-amber-500/30',
	fail: 'bg-rose-500/15 text-rose-300 border border-rose-500/30',
	running: 'bg-amber-500/15 text-amber-300 border border-amber-500/30',
	skipped: 'bg-slate-600/20 text-slate-400 border border-slate-600/40',
	gray: 'bg-slate-700/20 text-slate-400 border border-slate-700/40'
};
