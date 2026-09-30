// Verifies that every locale dictionary exposes exactly the same key set as en.json.
// README ("Interface and localization") requires the key sets to match.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'src', 'lib', 'i18n');
const locales = ['ru', 'pl', 'de', 'fr', 'zh'];
const base = 'en';

function flatten(obj, prefix = '', out = new Set()) {
	for (const [key, value] of Object.entries(obj)) {
		const next = prefix ? `${prefix}.${key}` : key;
		if (value && typeof value === 'object' && !Array.isArray(value)) {
			flatten(value, next, out);
		} else {
			out.add(next);
		}
	}
	return out;
}

function loadKeys(code) {
	return flatten(JSON.parse(fs.readFileSync(path.join(dir, `${code}.json`), 'utf8')));
}

const expected = loadKeys(base);
let failed = false;

for (const code of locales) {
	const actual = loadKeys(code);
	const missing = [...expected].filter((key) => !actual.has(key));
	const extra = [...actual].filter((key) => !expected.has(key));
	if (missing.length || extra.length) {
		failed = true;
		console.error(`i18n ${code}.json mismatch (missing=${missing.length}, extra=${extra.length})`);
		if (missing.length) console.error('  missing:', missing.join(', '));
		if (extra.length) console.error('  extra:', extra.join(', '));
	}
}

if (failed) process.exit(1);
console.log(`i18n key parity OK (${expected.size} keys × ${locales.length + 1} locales)`);
