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
	| 'forest'
	| 'dracula'
	| 'tokyonight'
	| 'mocha'
	| 'royal'
	| 'amber'
	| 'solarized'
	| 'sakura'
	| 'mint'
	| 'arctic'
	| 'honey'
	| 'halloween'
	| 'newyear'
	| 'lunarnewyear';

export type ThemeDef = {
	id: ThemeId;
	/** i18n key for the short theme name. */
	labelKey: string;
	/** i18n key for the one-line description. */
	hintKey: string;
	/** Whether the theme is a dark theme (drives `color-scheme`). */
	dark: boolean;
	/** Emoji plaque shown on the picker tile of holiday themes. */
	festive?: string;
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
	{ id: 'forest', labelKey: 'theme.forest', hintKey: 'theme.forestHint', dark: true },
	{ id: 'dracula', labelKey: 'theme.dracula', hintKey: 'theme.draculaHint', dark: true },
	{ id: 'tokyonight', labelKey: 'theme.tokyonight', hintKey: 'theme.tokyonightHint', dark: true },
	{ id: 'mocha', labelKey: 'theme.mocha', hintKey: 'theme.mochaHint', dark: true },
	{ id: 'royal', labelKey: 'theme.royal', hintKey: 'theme.royalHint', dark: true },
	{ id: 'amber', labelKey: 'theme.amber', hintKey: 'theme.amberHint', dark: true },
	{ id: 'solarized', labelKey: 'theme.solarized', hintKey: 'theme.solarizedHint', dark: false },
	{ id: 'sakura', labelKey: 'theme.sakura', hintKey: 'theme.sakuraHint', dark: false },
	{ id: 'mint', labelKey: 'theme.mint', hintKey: 'theme.mintHint', dark: false },
	{ id: 'arctic', labelKey: 'theme.arctic', hintKey: 'theme.arcticHint', dark: false },
	{ id: 'honey', labelKey: 'theme.honey', hintKey: 'theme.honeyHint', dark: false },
	// Holiday themes: auto-enabled in their festive windows (see activeSeason).
	{ id: 'halloween', labelKey: 'theme.halloween', hintKey: 'theme.halloweenHint', dark: true, festive: '🎃' },
	{ id: 'newyear', labelKey: 'theme.newyear', hintKey: 'theme.newyearHint', dark: true, festive: '🎄' },
	{
		id: 'lunarnewyear',
		labelKey: 'theme.lunarnewyear',
		hintKey: 'theme.lunarnewyearHint',
		dark: true,
		festive: '🏮'
	}
];

export const THEME_STORAGE_KEY = 'serpentseek-theme';
/** Marker left behind when a holiday theme was enabled automatically. */
export const SEASON_STORAGE_KEY = 'serpentseek-theme-season';
export const DEFAULT_THEME: ThemeId = 'emerald';

const themeIds = new Set<string>(THEMES.map((item) => item.id));

function isThemeId(value: unknown): value is ThemeId {
	return typeof value === 'string' && themeIds.has(value);
}

/* =========================================================================
   Holiday seasons
   -------------------------------------------------------------------------
   While a festive window is active its theme is enabled automatically —
   without overwriting the user's own pick (`serpentseek-theme`). Instead a
   small marker is stored under `serpentseek-theme-season`:

     { theme: <auto-enabled festive theme>, prev: <theme it replaced>, key }

   When the window ends, the theme silently rolls back to `prev`. Picking
   any theme by hand (applyTheme) clears the marker: the season then never
   touches the user's choice — e.g. someone who set Halloween themselves
   keeps it after November 1st.
   The preload script in app.html contains a mirror of this logic so the
   first paint already uses the resolved theme. Keep both in sync.
   ========================================================================= */

export type SeasonalWindow = {
	/** The festive theme to enable. */
	theme: ThemeId;
	/** Unique per occurrence (includes the year), so next year re-triggers. */
	key: string;
	/** Last day of the window (inclusive), shown in the "temporary" note. */
	ends: Date;
};

/** Gregorian dates of Chinese New Year, 2024–2035. */
const LUNAR_NEW_YEAR: Record<number, [month: number, day: number]> = {
	2024: [2, 10],
	2025: [1, 29],
	2026: [2, 17],
	2027: [2, 6],
	2028: [1, 26],
	2029: [2, 13],
	2030: [2, 3],
	2031: [1, 23],
	2032: [2, 11],
	2033: [1, 31],
	2034: [2, 19],
	2035: [2, 8]
};

/** Returns the festive window covering `now`, or null outside holidays. */
export function activeSeason(now: Date = new Date()): SeasonalWindow | null {
	const year = now.getFullYear();
	const mmdd = (now.getMonth() + 1) * 100 + now.getDate();

	// Halloween week: October 24 – November 1.
	if (mmdd >= 1024 && mmdd <= 1101) {
		return { theme: 'halloween', key: `halloween-${year}`, ends: new Date(year, 10, 1) };
	}
	// Winter New Year: December 24 – January 7.
	if (mmdd >= 1224) {
		return { theme: 'newyear', key: `newyear-${year}`, ends: new Date(year + 1, 0, 7) };
	}
	if (mmdd <= 107) {
		return { theme: 'newyear', key: `newyear-${year - 1}`, ends: new Date(year, 0, 7) };
	}
	// Chinese (Lunar) New Year: from 3 days before to 7 days after.
	// Its festive theme is a New Year theme too — it auto-enables like the others.
	const cny = LUNAR_NEW_YEAR[year];
	if (cny) {
		const start = new Date(year, cny[0] - 1, cny[1] - 3);
		const ends = new Date(year, cny[0] - 1, cny[1] + 7);
		if (now.getTime() >= start.getTime() && now.getTime() <= ends.getTime() + 86_399_999) {
			return { theme: 'lunarnewyear', key: `lunarnewyear-${year}`, ends };
		}
	}
	return null;
}

type SeasonMarker = {
	theme: ThemeId;
	prev: ThemeId;
	key: string;
};

/** Stored under SEASON_STORAGE_KEY: either an active auto-enabled season… */
type SeasonRecord =
	| ({ kind: 'auto' } & SeasonMarker)
	/** …or a dismissal left when the user picked another theme mid-season. */
	| { kind: 'dismissed'; key: string };

function readChosen(): ThemeId {
	try {
		const stored = localStorage.getItem(THEME_STORAGE_KEY);
		return isThemeId(stored) ? stored : DEFAULT_THEME;
	} catch {
		return DEFAULT_THEME;
	}
}

function readSeasonRecord(): SeasonRecord | null {
	try {
		const raw = localStorage.getItem(SEASON_STORAGE_KEY);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Record<string, unknown>;
		if (typeof parsed.dismissed === 'string') return { kind: 'dismissed', key: parsed.dismissed };
		if (isThemeId(parsed.theme) && isThemeId(parsed.prev) && typeof parsed.key === 'string') {
			return { kind: 'auto', theme: parsed.theme, prev: parsed.prev, key: parsed.key };
		}
	} catch {
		/* storage unavailable or corrupted — treat as no record */
	}
	return null;
}

function writeSeasonRecord(record: SeasonRecord): void {
	try {
		const payload =
			record.kind === 'dismissed'
				? { dismissed: record.key }
				: { theme: record.theme, prev: record.prev, key: record.key };
		localStorage.setItem(SEASON_STORAGE_KEY, JSON.stringify(payload));
	} catch {
		/* storage may be unavailable (private mode) — ignore */
	}
}

function clearSeasonRecord(): void {
	try {
		localStorage.removeItem(SEASON_STORAGE_KEY);
	} catch {
		/* ignore */
	}
}

export type SeasonalAuto = {
	marker: SeasonMarker;
	window: SeasonalWindow;
};

function resolveInitial(): { active: ThemeId; auto: SeasonalAuto | null } {
	if (typeof localStorage === 'undefined') return { active: DEFAULT_THEME, auto: null };

	const season = activeSeason();
	let chosen = readChosen();
	let record = readSeasonRecord();

	// Drop records whose window has ended; an auto record also rolls the
	// interface back to the theme it had replaced.
	if (record && (!season || season.key !== record.key)) {
		if (record.kind === 'auto') chosen = record.prev;
		clearSeasonRecord();
		record = null;
	}

	if (!season) return { active: chosen, auto: null };

	// The user picked the festive theme themselves — the season must not touch it.
	if (chosen === season.theme) {
		clearSeasonRecord();
		return { active: chosen, auto: null };
	}
	// The user already overrode this season occurrence once — respect that.
	if (record?.kind === 'dismissed') return { active: chosen, auto: null };
	// Already auto-enabled for this occurrence: keep it until the window ends.
	if (record?.kind === 'auto') {
		const marker: SeasonMarker = { theme: record.theme, prev: record.prev, key: record.key };
		return { active: record.theme, auto: { marker, window: season } };
	}
	// First visit inside the window: auto-enable, remembering what it replaced.
	const fresh: SeasonRecord = { kind: 'auto', theme: season.theme, prev: chosen, key: season.key };
	writeSeasonRecord(fresh);
	const marker: SeasonMarker = { theme: fresh.theme, prev: fresh.prev, key: fresh.key };
	return { active: season.theme, auto: { marker, window: season } };
}

const boot = resolveInitial();

export const theme = writable<ThemeId>(boot.active);
/** Non-null while the current theme was auto-enabled for an ongoing holiday. */
export const seasonalAuto = writable<SeasonalAuto | null>(boot.auto);

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
	// Any manual choice ends the seasonal auto-mode — the user is in control.
	// If the season is still running, remember the dismissal so the next visit
	// does not auto-enable it again; picking the festive theme itself instead
	// makes it permanent (it is now the user's stored choice).
	const record = readSeasonRecord();
	const season = activeSeason();
	if (record?.kind === 'auto' && season && season.key === record.key && meta.id !== season.theme) {
		writeSeasonRecord({ kind: 'dismissed', key: season.key });
	} else {
		clearSeasonRecord();
	}
	seasonalAuto.set(null);
	theme.set(meta.id);
}
