// Original, hand-drawn 24×24 stroke icons used by the sidebar (and a few
// buttons). Kept as plain data so they can be rendered by <Icon /> without any
// runtime dependency: every glyph is a set of simple shapes on a 24×24 grid.
//
// Conventions:
//   * The viewBox is always 0 0 24 24.
//   * Strokes are supplied by <Icon /> (currentColor), so paths only carry `d`.
//   * A shape may opt out of the stroke with `filled: true`, in which case it is
//     painted with the current text colour instead.

export type IconShape =
	| { t: 'path'; d: string; filled?: boolean }
	| { t: 'circle'; cx: number; cy: number; r: number; filled?: boolean }
	| { t: 'rect'; x: number; y: number; w: number; h: number; rx?: number };

export type IconName =
	| 'dashboard'
	| 'analytics'
	| 'history'
	| 'chains'
	| 'providers'
	| 'playground'
	| 'mcp'
	| 'logs'
	| 'audit'
	| 'users'
	| 'developer'
	| 'settings'
	| 'heart'
	| 'collapse'
	| 'expand';

export const ICONS: Record<IconName, IconShape[]> = {
	dashboard: [
		{ t: 'rect', x: 3.5, y: 3.5, w: 7, h: 7, rx: 1.6 },
		{ t: 'rect', x: 13.5, y: 3.5, w: 7, h: 7, rx: 1.6 },
		{ t: 'rect', x: 3.5, y: 13.5, w: 7, h: 7, rx: 1.6 },
		{ t: 'rect', x: 13.5, y: 13.5, w: 7, h: 7, rx: 1.6 }
	],
	analytics: [
		{ t: 'path', d: 'M4 20V11' },
		{ t: 'path', d: 'M9.4 20V5' },
		{ t: 'path', d: 'M14.6 20v-6' },
		{ t: 'path', d: 'M20 20V8.5' },
		{ t: 'path', d: 'M2.8 20.5h18.4' }
	],
	history: [
		{ t: 'path', d: 'M3.6 11.8a8.4 8.4 0 1 0 2.5-5.6' },
		{ t: 'path', d: 'M3 3.4v3.4h3.4' },
		{ t: 'path', d: 'M12 7.6V12l2.8 1.8' }
	],
	chains: [
		{ t: 'path', d: 'M9.5 14.5l5-5' },
		{ t: 'path', d: 'M8.2 11.6l-1.9 1.9a3.3 3.3 0 0 0 4.7 4.7l1.9-1.9' },
		{ t: 'path', d: 'M15.8 12.4l1.9-1.9a3.3 3.3 0 0 0-4.7-4.7l-1.9 1.9' }
	],
	// Three provider modules and a "new" slot.
	providers: [
		{ t: 'rect', x: 3.5, y: 3.5, w: 8, h: 8, rx: 1.7 },
		{ t: 'rect', x: 12.5, y: 3.5, w: 8, h: 8, rx: 1.7 },
		{ t: 'rect', x: 3.5, y: 12.5, w: 8, h: 8, rx: 1.7 },
		{ t: 'path', d: 'M16.5 12.7v7.6M12.7 16.5h7.6' }
	],
	// Lab flask — the request playground.
	playground: [
		{ t: 'path', d: 'M9.5 3h5' },
		{ t: 'path', d: 'M10 3v6.2l-4.6 7.6A1.6 1.6 0 0 0 6.8 19h10.4a1.6 1.6 0 0 0 1.4-2.2L14 9.2V3' },
		{ t: 'path', d: 'M7.4 14.5h9.2' }
	],
	// CPU chip — the MCP server surface.
	mcp: [
		{ t: 'rect', x: 7, y: 7, w: 10, h: 10, rx: 2 },
		{ t: 'rect', x: 10.4, y: 10.4, w: 3.2, h: 3.2, rx: 1 },
		{
			t: 'path',
			d: 'M10 3.5v3.5M14 3.5v3.5M10 17v3.5M14 17v3.5M3.5 10H7M3.5 14H7M17 10h3.5M17 14h3.5'
		}
	],
	logs: [
		{ t: 'path', d: 'M13.5 3H6.5A1.5 1.5 0 0 0 5 4.5v15A1.5 1.5 0 0 0 6.5 21h11a1.5 1.5 0 0 0 1.5-1.5V8.5z' },
		{ t: 'path', d: 'M13.5 3v5.5H19' },
		{ t: 'path', d: 'M8.5 13h7M8.5 16.5h7' }
	],
	audit: [
		{ t: 'rect', x: 5, y: 4, w: 14, h: 17, rx: 2 },
		{ t: 'path', d: 'M9 4V3.3A1.3 1.3 0 0 1 10.3 2h3.4A1.3 1.3 0 0 1 15 3.3V4' },
		{ t: 'path', d: 'M9 13.2l2 2 4-4.2' }
	],
	users: [
		{ t: 'circle', cx: 9, cy: 8, r: 3.2 },
		{ t: 'path', d: 'M3.5 20.5a5.5 5.5 0 0 1 11 0' },
		{ t: 'path', d: 'M16 5.4a3 3 0 0 1 0 5.7' },
		{ t: 'path', d: 'M17 14.6a5.5 5.5 0 0 1 3.5 5.9' }
	],
	developer: [
		{ t: 'path', d: 'M9 7.5 3.8 12 9 16.5' },
		{ t: 'path', d: 'M15 7.5 20.2 12 15 16.5' },
		{ t: 'path', d: 'M13.3 4.5 10.7 19.5' }
	],
	settings: [
		{ t: 'path', d: 'M4 7.5h8M16.2 7.5H20M4 16.5h3M11.2 16.5H20' },
		{ t: 'circle', cx: 14.1, cy: 7.5, r: 2.1 },
		{ t: 'circle', cx: 9.1, cy: 16.5, r: 2.1 }
	],
	heart: [
		{ t: 'path', d: 'M12 20.2S4.7 15.6 4.7 10.3A4.3 4.3 0 0 1 12 7.2a4.3 4.3 0 0 1 7.3 3.1c0 5.3-7.3 9.9-7.3 9.9z' }
	],
	collapse: [
		{ t: 'path', d: 'M11.5 6 5.5 12l6 6' },
		{ t: 'path', d: 'M18.5 6 12.5 12l6 6' }
	],
	expand: [
		{ t: 'path', d: 'M12.5 6l6 6-6 6' },
		{ t: 'path', d: 'M5.5 6l6 6-6 6' }
	]
};
