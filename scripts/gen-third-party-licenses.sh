#!/usr/bin/env bash
# gen-third-party-licenses.sh regenerates THIRD_PARTY_LICENSES.md.
#
# It enumerates the Go modules actually compiled into the serpentseek binary
# (go list -deps ./cmd/serpentseek) and the npm packages whose code ends up in
# the embedded web bundle (non-dev entries of web/package-lock.json), then
# reproduces their license texts verbatim from the local module cache /
# node_modules. Both must be present locally (run `go mod download` and
# `npm ci` first).
#
# Usage: scripts/gen-third-party-licenses.sh > THIRD_PARTY_LICENSES.md
set -euo pipefail
cd "$(dirname "$0")/.."

lic_name() { # best-effort SPDX id from the license text
	local f="$1"
	if head -c 4000 "$f" | grep -qi "apache license"; then echo "Apache-2.0"; return; fi
	if head -c 4000 "$f" | grep -qi "permission is hereby granted, free of charge"; then echo "MIT"; return; fi
	if head -c 4000 "$f" | grep -qi "permission to use, copy, modify, and" ; then echo "ISC"; return; fi
	if head -c 4000 "$f" | grep -qi "redistribution and use in source and binary forms"; then echo "BSD-3-Clause"; return; fi
	echo "see text below"
}

emit_license_files() { # dump every LICENSE*/COPYING* file at a package root
	local dir="$1" f
	find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' \) | sort | while IFS= read -r f; do
		if [ "$(basename "$f")" != "LICENSE" ] && [ "$(basename "$f")" != "LICENSE.md" ] && [ "$(basename "$f")" != "LICENSE.txt" ]; then
			printf '\n_%s:_\n\n' "$(basename "$f")"
		fi
		printf '```\n'
		cat "$f"
		printf '\n```\n'
	done
}

cat <<'EOF'
# Third-Party Licenses

SerpentSeek itself is distributed under the terms of the GNU General Public
License v3.0 (see [LICENSE](LICENSE)).

The SerpentSeek binary (and the Docker image built from this repository)
statically links the Go modules below and embeds the compiled web-UI bundle,
which contains the npm packages below. Their licenses require that the
copyright notices and license texts accompany distributions; they are
reproduced here.

This file is generated — do not edit by hand. Regenerate after dependency
changes with:

    scripts/gen-third-party-licenses.sh > THIRD_PARTY_LICENSES.md

## Go modules (compiled into the binary)
EOF

go list -m -f '{{.Path}}|{{.Version}}|{{.Dir}}' \
	$(go list -f '{{with .Module}}{{.Path}}@{{.Version}}{{end}}' \
		$(go list -deps ./cmd/serpentseek | grep -v 'serpent-seek') | sort -u) \
	2>/dev/null | sort | while IFS='|' read -r path ver dir; do
	[ -d "$dir" ] || continue
	main="$dir/LICENSE"
	[ -f "$main" ] || main=$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' \) | sort | head -1)
	if [ -n "$main" ]; then lic=$(lic_name "$main"); else lic="unknown"; fi
	printf '\n### %s %s — %s\n\n' "$path" "$ver" "$lic"
	if [ -n "$main" ]; then emit_license_files "$dir"; else echo '_No license file shipped in the module archive._'; fi
done

cat <<'EOF'

## Web UI packages (embedded in the frontend bundle)
EOF

node -e '
const lock = require("./web/package-lock.json");
for (const [k, v] of Object.entries(lock.packages || {})) {
	if (!k || v.dev) continue;
	const name = k.replace(/^node_modules\//, "").replace(/.*node_modules\//, "");
	if (name.startsWith("@types/")) continue;
	console.log([name, v.version, "web/" + k].join("|"));
}' | sort | while IFS='|' read -r name ver dir; do
	[ -d "$dir" ] || continue
	pkg_lic=$(node -e "const p=require('./$dir/package.json');console.log((typeof p.license==='object'?p.license.type:p.license)||(p.licenses&&p.licenses[0]&&p.licenses[0].type)||'unknown')")
	main=$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' \) | sort | head -1)
	printf '\n### %s %s — %s\n\n' "$name" "$ver" "$pkg_lic"
	if [ -n "$main" ]; then
		emit_license_files "$dir"
	elif [ "$pkg_lic" = "MIT" ]; then
		# The npm tarball ships no LICENSE file; reproduce the canonical MIT
		# text with the copyright holder taken from package.json.
		author=$(node -e "const p=require('./$dir/package.json');const a=p.author;console.log(typeof a==='object'?a.name||'':a||'')")
		printf '```\nMIT License\n\nCopyright (c) %s\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files (the "Software"), to deal\nin the Software without restriction, including without limitation the rights\nto use, copy, modify, merge, publish, distribute, sublicense, and/or sell\ncopies of the Software, and to permit persons to whom the Software is\nfurnished to do so, subject to the following conditions:\n\nThe above copyright notice and this permission notice shall be included in all\ncopies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR\nIMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,\nFITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE\nAUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER\nLIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,\nOUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE\nSOFTWARE.\n```\n' "$author"
	else
		echo '_No license file shipped in the npm package._'
	fi
done
