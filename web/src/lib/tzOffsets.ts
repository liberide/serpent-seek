/**
 * Fixed UTC offsets used by the per-user quota reset boundary.
 *
 * The backend stores tz_offset as a fixed minute offset (no IANA name, no
 * tzdata dependency, DST is not observed). This list covers every fixed
 * offset that exists in the real world today (including the half-hour and
 * 45-minute zones) so the admin picks a real timezone instead of typing
 * minutes. `label` pairs the offset with a few well-known cities; rare
 * offsets carry no city hint.
 */

export type TzOffset = {
	/** Offset from UTC in minutes (e.g. 330 for UTC+05:30). */
	offset: number;
	/** Display label, e.g. "(UTC+05:30) India / Sri Lanka". */
	label: string;
};

function fmt(minutes: number): string {
	const sign = minutes < 0 ? '-' : '+';
	const abs = Math.abs(minutes);
	const h = Math.floor(abs / 60);
	const m = abs % 60;
	return `UTC${sign}${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

function entry(offset: number, cities = ''): TzOffset {
	const base = `(${fmt(offset)})`;
	return { offset, label: cities ? `${base} ${cities}` : base };
}

/** All real-world fixed UTC offsets, ordered from west to east. */
export const TZ_OFFSETS: TzOffset[] = [
	entry(-720, 'Baker Island'), // UTC-12:00
	entry(-660, 'American Samoa'), // UTC-11:00
	entry(-600, 'Hawaii'), // UTC-10:00
	entry(-570, 'Marquesas Islands'), // UTC-09:30
	entry(-540, 'Alaska'), // UTC-09:00
	entry(-480, 'US/Canada Pacific'), // UTC-08:00
	entry(-420, 'US/Canada Mountain'), // UTC-07:00
	entry(-360, 'US/Canada Central'), // UTC-06:00
	entry(-300, 'US/Canada East, Mexico'), // UTC-05:00
	entry(-270, 'Venezuela'), // UTC-04:30
	entry(-240, 'Atlantic Canada, Caribbean'), // UTC-04:00
	entry(-210, 'Newfoundland'), // UTC-03:30
	entry(-180, 'Brazil, Argentina'), // UTC-03:00
	entry(-120, 'Mid-Atlantic'), // UTC-02:00
	entry(-60, 'Azores, Cape Verde'), // UTC-01:00
	entry(0, 'UTC / London (winter), Lisbon'), // UTC±00:00
	entry(60, 'Central Europe (winter), Lagos'), // UTC+01:00
	entry(120, 'Eastern Europe (winter), Cairo'), // UTC+02:00
	entry(180, 'Moscow, East Africa'), // UTC+03:00
	entry(210, 'Iran'), // UTC+03:30
	entry(240, 'Dubai, Caucasus'), // UTC+04:00
	entry(270, 'Kabul'), // UTC+04:30
	entry(300, 'Pakistan, Central Asia'), // UTC+05:00
	entry(330, 'India / Sri Lanka'), // UTC+05:30
	entry(345, 'Nepal'), // UTC+05:45
	entry(360, 'Bangladesh, Kazakhstan'), // UTC+06:00
	entry(390, 'Myanmar'), // UTC+06:30
	entry(420, 'Indochina, Mongolia (west)'), // UTC+07:00
	entry(480, 'China / Singapore / Perth'), // UTC+08:00
	entry(525, 'Eucla (SE Western Australia)'), // UTC+08:45
	entry(540, 'Japan / Korea'), // UTC+09:00
	entry(570, 'Central Australia (Adelaide)'), // UTC+09:30
	entry(600, 'Eastern Australia, PNG'), // UTC+10:00
	entry(630, 'Lord Howe Island'), // UTC+10:30
	entry(660, 'Solomon Islands, New Caledonia'), // UTC+11:00
	entry(720, 'New Zealand (winter), Fiji'), // UTC+12:00
	entry(765, 'Chatham Islands'), // UTC+12:45
	entry(780, 'Tonga, Phoenix Islands'), // UTC+13:00
	entry(840, 'Line Islands (Kiritimati)') // UTC+14:00
];

/** Formats an offset in minutes as "(UTC+05:30)" for arbitrary values. */
export function formatTzOffset(minutes: number): string {
	return `(${fmt(minutes)})`;
}
