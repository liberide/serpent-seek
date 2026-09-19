import ru from './i18n/ru.json';
import en from './i18n/en.json';
import pl from './i18n/pl.json';
import de from './i18n/de.json';
import fr from './i18n/fr.json';
import zh from './i18n/zh.json';
import languages from './i18n/languages.json';

export type Locale = 'ru' | 'en' | 'pl' | 'de' | 'fr' | 'zh';

function flatten(
	obj: Record<string, unknown>,
	prefix = '',
	out: Record<string, string> = {}
): Record<string, string> {
	for (const [key, value] of Object.entries(obj)) {
		const path = prefix ? `${prefix}.${key}` : key;
		if (value && typeof value === 'object' && !Array.isArray(value)) {
			flatten(value as Record<string, unknown>, path, out);
		} else {
			out[path] = String(value);
		}
	}
	return out;
}

const dictionaries: Record<Locale, Record<string, string>> = {
	ru: flatten(ru),
	en: flatten(en),
	pl: flatten(pl),
	de: flatten(de),
	fr: flatten(fr),
	zh: flatten(zh)
};

function isLocale(value: string | null): value is Locale {
	return value !== null && value in dictionaries;
}

function initialLocale(): Locale {
	if (typeof localStorage !== 'undefined') {
		const stored = localStorage.getItem('locale');
		if (isLocale(stored)) return stored;
	}
	if (typeof navigator !== 'undefined') {
		const nav = navigator.language?.toLowerCase() ?? '';
		const match = (Object.keys(dictionaries) as Locale[]).find((code) => nav.startsWith(code));
		if (match) return match;
	}
	return 'en';
}

let current: Locale = $state(initialLocale());

export function getLocale(): Locale {
	return current;
}

export function setLocale(locale: Locale): void {
	current = locale;
	if (typeof localStorage !== 'undefined') {
		localStorage.setItem('locale', locale);
	}
}

export const availableLanguages = languages as { code: Locale; label: string }[];

/** Translates a dot key, interpolating `{name}` placeholders and falling back to en. */
export function t(key: string, params?: Record<string, string | number>): string {
	let text = dictionaries[current][key] ?? dictionaries.en[key] ?? key;
	if (params) {
		for (const [name, value] of Object.entries(params)) {
			text = text.split(`{${name}}`).join(String(value));
		}
	}
	return text;
}
