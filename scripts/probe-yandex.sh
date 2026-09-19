#!/usr/bin/env bash
# probe-yandex.sh — one live probe of the Yandex Search API v2 web endpoint.
#
# Usage:
#   YANDEX_API_KEY=... YANDEX_FOLDER_ID=... scripts/probe-yandex.sh "query text"
#
# Prints the raw HTTP status plus the first three parsed rows (link/title/snippet)
# so the driver's XML element names can be confirmed against the live service.
set -euo pipefail

: "${YANDEX_API_KEY:?set YANDEX_API_KEY}"
: "${YANDEX_FOLDER_ID:?set YANDEX_FOLDER_ID}"
QUERY="${1:-test}"
BASE="${YANDEX_BASE_URL:-https://searchapi.api.cloud.yandex.net}"

curl --compressed -sS -o /tmp/probe-yandex.json -w 'HTTP %{http_code}\n' \
	-X POST "${BASE}/v2/web/search" \
	-H "Authorization: Api-Key ${YANDEX_API_KEY}" \
	-H 'Content-Type: application/json' \
	-d "{\"query\":{\"searchType\":\"SEARCH_TYPE_RU\",\"queryText\":\"${QUERY}\",\"familyMode\":\"FAMILY_MODE_NONE\",\"page\":\"0\"},\"groupSpec\":{\"groupMode\":\"DEEP\",\"groupsOnPage\":\"10\",\"docsInGroup\":\"1\"},\"maxPassages\":\"3\",\"folderId\":\"${YANDEX_FOLDER_ID}\",\"responseFormat\":\"FORMAT_XML\"}"

python3 - <<'PY'
import base64, json, sys, xml.etree.ElementTree as ET

with open('/tmp/probe-yandex.json', 'rb') as fh:
    data = json.load(fh)

raw = data.get('rawData')
if not raw:
    print('no rawData field:', json.dumps(data, ensure_ascii=False)[:500])
    sys.exit(1)

root = ET.fromstring(base64.b64decode(raw))
err = root.find('./response/error')
if err is not None:
    print('XML error code=%s: %s' % (err.get('code'), (err.text or '').strip()))
    sys.exit(2)

docs = root.findall('./response/results/grouping/group/doc')
print('docs:', len(docs))
for doc in docs[:3]:
    url = (doc.findtext('url') or '').strip()
    title = ''.join(doc.find('title').itertext()).strip() if doc.find('title') is not None else ''
    passages = [''.join(p.itertext()).strip() for p in doc.findall('passages/passage')]
    snippet = ' … '.join(p for p in passages if p)
    print('-', title)
    print(' ', url)
    print(' ', snippet[:200])
PY
