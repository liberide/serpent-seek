// Validates that api/openapi.yaml is well-formed YAML and declares a supported
// OpenAPI 3.x version. The router parity test only compares path/method keys, so
// whole-document validity (plain scalars included) is checked here as part of
// `npm run check` in CI.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { load } from 'js-yaml';

const file = path.join(
	path.dirname(fileURLToPath(import.meta.url)),
	'..',
	'..',
	'api',
	'openapi.yaml'
);

let doc;
try {
	doc = load(fs.readFileSync(file, 'utf8'));
} catch (error) {
	console.error(`openapi.yaml is not valid YAML: ${error.message}`);
	process.exit(1);
}

const version = doc && doc.openapi;
if (typeof version !== 'string' || !/^3\.\d+\.\d+/.test(version)) {
	console.error(`openapi.yaml has an invalid 'openapi' version field: ${JSON.stringify(version)}`);
	process.exit(1);
}
if (!doc.paths || typeof doc.paths !== 'object') {
	console.error('openapi.yaml has no paths object');
	process.exit(1);
}

console.log(`openapi.yaml OK (openapi ${version}, ${Object.keys(doc.paths).length} paths)`);
