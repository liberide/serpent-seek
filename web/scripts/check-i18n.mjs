// Verifies that every locale dictionary exposes exactly the same key set as en.json.
// README ("Interface and localization") requires the key sets to match.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'src', 'lib', 'i18n');
const locales = ['ru', 'pl', 'de', 'fr', 'ja', 'zh'];
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

// JSON.parse silently keeps the last duplicate key, which would mask a missing
// translation. The dictionaries are pretty-printed and contain no arrays, so a
// simple indentation-based scan can catch sibling keys with the same name.
function duplicateKeys(text) {
	const dups = [];
	const stack = [];
	for (const raw of text.split('\n')) {
		const m = raw.match(/^(\t*)"((?:[^"\\]|\\.)*)":/);
		if (!m) continue;
		const indent = m[1].length;
		while (stack.length && stack[stack.length - 1].indent > indent) stack.pop();
		if (!stack.length || stack[stack.length - 1].indent !== indent) {
			stack.push({ indent, keys: new Set() });
		}
		const frame = stack[stack.length - 1];
		if (frame.keys.has(m[2])) dups.push(m[2]);
		frame.keys.add(m[2]);
	}
	return dups;
}

const expected = loadKeys(base);
let failed = false;

// Duplicate sibling keys in any dictionary (masked by JSON.parse).
for (const code of [base, ...locales]) {
	const dups = [...new Set(duplicateKeys(fs.readFileSync(path.join(dir, `${code}.json`), 'utf8')))];
	if (dups.length) {
		failed = true;
		console.error(`i18n ${code}.json has duplicate keys: ${dups.join(', ')}`);
	}
}

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
