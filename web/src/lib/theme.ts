import { writable } from 'svelte/store';

export type ThemeId =
	| 'emerald'
	| 'daylight'
	| 'matrix'
	| 'ocean'
	| 'sunset'
	| 'parchment'
	| 'gruvbox'
	| 'cyberpunk'
	| 'mono'
	| 'rose'
	| 'oled'
	| 'nord'
	| 'lavender'
	| 'ember'
	| 'forest';

export type ThemeDef = {
	id: ThemeId;
	/** i18n key for the short theme name. */
	labelKey: string;
	/** i18n key for the one-line description. */
	hintKey: string;
	/** Whether the theme is a dark theme (drives `color-scheme`). */
	dark: boolean;
};

/** All available themes, in picker order. The first entry is the default. */
export const THEMES: ThemeDef[] = [
	{ id: 'emerald', labelKey: 'theme.emerald', hintKey: 'theme.emeraldHint', dark: true },
	{ id: 'daylight', labelKey: 'theme.daylight', hintKey: 'theme.daylightHint', dark: false },
	{ id: 'matrix', labelKey: 'theme.matrix', hintKey: 'theme.matrixHint', dark: true },
	{ id: 'ocean', labelKey: 'theme.ocean', hintKey: 'theme.oceanHint', dark: true },
	{ id: 'sunset', labelKey: 'theme.sunset', hintKey: 'theme.sunsetHint', dark: true },
	{ id: 'parchment', labelKey: 'theme.parchment', hintKey: 'theme.parchmentHint', dark: false },
	{ id: 'gruvbox', labelKey: 'theme.gruvbox', hintKey: 'theme.gruvboxHint', dark: true },
	{ id: 'cyberpunk', labelKey: 'theme.cyberpunk', hintKey: 'theme.cyberpunkHint', dark: true },
	{ id: 'mono', labelKey: 'theme.mono', hintKey: 'theme.monoHint', dark: true },
	{ id: 'rose', labelKey: 'theme.rose', hintKey: 'theme.roseHint', dark: false },
	{ id: 'oled', labelKey: 'theme.oled', hintKey: 'theme.oledHint', dark: true },
	{ id: 'nord', labelKey: 'theme.nord', hintKey: 'theme.nordHint', dark: true },
	{ id: 'lavender', labelKey: 'theme.lavender', hintKey: 'theme.lavenderHint', dark: false },
	{ id: 'ember', labelKey: 'theme.ember', hintKey: 'theme.emberHint', dark: true },
	{ id: 'forest', labelKey: 'theme.forest', hintKey: 'theme.forestHint', dark: true }
];

export const THEME_STORAGE_KEY = 'serpentseek-theme';
export const DEFAULT_THEME: ThemeId = 'emerald';

const themeIds = new Set<string>(THEMES.map((item) => item.id));

function isThemeId(value: string | null): value is ThemeId {
	return value !== null && themeIds.has(value);
}

/** Reads the persisted theme, falling back to the default. */
export function initialTheme(): ThemeId {
	if (typeof localStorage === 'undefined') return DEFAULT_THEME;
	try {
		const stored = localStorage.getItem(THEME_STORAGE_KEY);
		return isThemeId(stored) ? stored : DEFAULT_THEME;
	} catch {
		return DEFAULT_THEME;
	}
}

export const theme = writable<ThemeId>(initialTheme());

/** Applies a theme to <html>, persists it and updates the store. */
export function applyTheme(id: ThemeId): void {
	const meta = THEMES.find((item) => item.id === id) ?? THEMES[0];
	if (typeof document !== 'undefined') {
		const root = document.documentElement;
		root.setAttribute('data-theme', meta.id);
		root.style.colorScheme = meta.dark ? 'dark' : 'light';
	}
	if (typeof localStorage !== 'undefined') {
		try {
			localStorage.setItem(THEME_STORAGE_KEY, meta.id);
		} catch {
			/* storage may be unavailable (private mode) — ignore */
		}
	}
	theme.set(meta.id);
}
