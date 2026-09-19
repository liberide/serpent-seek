package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
)

// yandexProvider calls the official Yandex Search API v2 (Yandex Cloud AI
// Studio) web search endpoint POST /v2/web/search. The response carries
// `rawData` = base64-encoded XML (the only web-search format); it is parsed
// with stdlib encoding/xml. Reference:
// https://yandex.cloud/en/docs/search-api/ (WebSearch.Search, FORMAT_XML).
//
// Verification status: mock-only (httptest fixtures built from the documented
// response shape) until a live api_key + folder_id run confirms it.
type yandexProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p yandexProvider) Code() string { return "yandex" }

// Schema describes the provider configuration UI.
func (p yandexProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "yandex",
		Name:        "Yandex Search API",
		Credentials: []CredentialField{{Key: "api_key"}, {Key: "folder_id"}},
		Params: []ParamField{
			{Key: "search_type", Label: "Search type", Type: ParamTypeSelect, Default: "SEARCH_TYPE_RU", Options: []ParamOption{{"RU", "SEARCH_TYPE_RU"}, {"TR", "SEARCH_TYPE_TR"}, {"COM", "SEARCH_TYPE_COM"}}},
			{Key: "family_mode", Label: "Family mode", Type: ParamTypeSelect, Default: "FAMILY_MODE_NONE", Options: []ParamOption{{"None", "FAMILY_MODE_NONE"}, {"Moderate", "FAMILY_MODE_MODERATE"}, {"Strict", "FAMILY_MODE_STRICT"}}},
			{Key: "max_passages", Label: "Max passages", Type: ParamTypeNumber, Default: "3", Hint: "1–5, controls the snippet length"},
			{Key: "region", Label: "Region", Type: ParamTypeText, Hint: "Yandex region code, e.g. 213 = Moscow"},
			{Key: "xml_empty_codes", Label: "XML empty codes", Type: ParamTypeText, Default: "15", Hint: "<error code> values treated as “no results”"},
			{Key: "xml_retry_codes", Label: "XML retry codes", Type: ParamTypeText, Default: "", Hint: "<error code> values treated as transient (quota/overload); empty = unknown codes are permanent"},
			{Key: "l10n", Label: "Localization", Type: ParamTypeSelect, Default: "", Options: []ParamOption{{"Auto", ""}, {"RU", "LOCALIZATION_RU"}, {"EN", "LOCALIZATION_EN"}, {"UK", "LOCALIZATION_UK"}, {"BE", "LOCALIZATION_BE"}, {"KK", "LOCALIZATION_KK"}, {"TR", "LOCALIZATION_TR"}}},
			{Key: "max_rps", Label: "Max requests/sec", Type: ParamTypeNumber, Hint: "Per-driver rate limit override"},
			{Key: "fatal_http", Label: "Fatal HTTP codes", Type: ParamTypeText, Default: "400,401,403"},
			{Key: "retry_http_codes", Label: "Retry HTTP codes", Type: ParamTypeText, Default: "429,500,502,503,504"},
		},
		Hints: []string{
			"Uses /v2/web/search with responseFormat=FORMAT_XML; the answer arrives as base64-encoded XML in rawData and is parsed locally.",
			"XML error code 15 means “no results” and maps to an empty result.",
		},
	}
}

// yandexXML mirrors the decoded rawData XML. Only Row-relevant fields are
// declared; unknown elements are skipped. <hlword> markup inside title/passage
// is flattened by xmlText.
type yandexXML struct {
	XMLName xml.Name `xml:"yandexsearch"`
	Error   *struct {
		Code int    `xml:"code,attr"`
		Text string `xml:",chardata"`
	} `xml:"response>error"`
	Docs []struct {
		URL      string    `xml:"url"`
		Title    xmlText   `xml:"title"`
		Passages []xmlText `xml:"passages>passage"`
	} `xml:"response>results>grouping>group>doc"`
}

func (p yandexProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://searchapi.api.cloud.yandex.net"), "/")
	key := c["api_key"]
	folder := c["folder_id"]
	if folder == "" {
		folder = params["folder_id"]
	}
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	if err := WaitLimit(ctx, p.Code(), maxRPSParam(params, 0)); err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "yandex: rate limiter wait: " + err.Error()}
	}

	payload, _ := json.Marshal(yandexSearchBody(q, params, folder))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/web/search", bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "yandex: build request: "+err.Error())
	}
	req.Header.Set("Authorization", "Api-Key "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		kind := ClassifyTransport(err)
		return Result{Kind: kind, Provider: p.Code(), Error: "yandex: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, permanent := HTTPClassify(resp.StatusCode, retryHTTP)
		if containsInt(fatalHTTP, resp.StatusCode) {
			permanent = true
		}
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			RetryAfterMS: retryAfterHeaderMS(resp),
			Error:        "yandex: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}

	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	rawB64 := asString(data["rawData"])
	if rawB64 == "" {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex: response has no rawData field"}
	}
	rawXML, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex: rawData is not valid base64"}
	}
	var parsed yandexXML
	if err := xml.Unmarshal(rawXML, &parsed); err != nil {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex: rawData XML parse failed: " + err.Error()}
	}

	if xerr := parsed.Error; xerr != nil {
		// Code 15 (default) = "nothing found" — a valid empty result, not a failure.
		emptyCodes := parseIntCSV(params["xml_empty_codes"], []int{15})
		if containsInt(emptyCodes, xerr.Code) {
			return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: resp.StatusCode}
		}
		// Error-code classification is configurable because the exact transient
		// Yandex XML codes (quota/overload) still need a live-key verification:
		// anything not listed in xml_retry_codes is treated as permanent.
		retryCodes := parseIntCSV(params["xml_retry_codes"], nil)
		return Result{Kind: KindAPI, Permanent: !containsInt(retryCodes, xerr.Code), Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex: XML error code=" + strconv.Itoa(xerr.Code) + " " + p.http.Redact(xerr.Text)}
	}

	var rows Rows
	for _, doc := range parsed.Docs {
		link := strings.TrimSpace(doc.URL)
		if link == "" {
			continue
		}
		var parts []string
		for _, pa := range doc.Passages {
			if s := strings.TrimSpace(pa.Text); s != "" {
				parts = append(parts, s)
			}
		}
		rows = append(rows, Row{Link: link, Title: strings.TrimSpace(doc.Title.Text), Snippet: strings.Join(parts, " … ")})
	}
	if len(rows) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: resp.StatusCode}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// yandexSearchBody builds the /v2/web/search payload. Numeric-ish fields are
// strings per the API reference ("groupsOnPage": "10", "page": "0"...).
func yandexSearchBody(q Query, params Params, folder string) map[string]any {
	groupsOnPage := "10"
	if q.Count > 0 {
		groupsOnPage = strconv.Itoa(q.Count)
	}
	query := map[string]any{
		"searchType": defaultStr(params["search_type"], "SEARCH_TYPE_RU"),
		"queryText":  q.Text,
		"familyMode": defaultStr(params["family_mode"], "FAMILY_MODE_NONE"),
		"page":       "0",
	}
	body := map[string]any{
		"query":          query,
		"groupSpec":      map[string]any{"groupMode": "DEEP", "groupsOnPage": groupsOnPage, "docsInGroup": "1"},
		"maxPassages":    defaultStr(params["max_passages"], "3"),
		"folderId":       folder,
		"responseFormat": "FORMAT_XML",
	}
	if v := strings.TrimSpace(params["region"]); v != "" {
		body["region"] = v
	}
	if v := strings.TrimSpace(params["l10n"]); v != "" {
		body["l10n"] = v
	}
	return body
}

// --- yandex_gen: generative answer endpoint (answer-type driver) ---

// yandexGenProvider calls POST /v2/gen/search (AI Studio generative search).
// The result is a generated answer; cited sources are mapped to (sparse) Rows
// when present. The answer text itself is kept in Result.Answer and never
// mixed into Row.Snippet (full `mode: answer` node support is a separate task).
type yandexGenProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p yandexGenProvider) Code() string { return "yandex_gen" }

// Schema describes the provider configuration UI.
func (p yandexGenProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "yandex_gen",
		Name:        "Yandex AI Studio (generative answer)",
		Credentials: []CredentialField{{Key: "api_key"}, {Key: "folder_id"}},
		Params: []ParamField{
			{Key: "search_type", Label: "Search type", Type: ParamTypeSelect, Default: "SEARCH_TYPE_RU", Options: []ParamOption{{"RU", "SEARCH_TYPE_RU"}, {"TR", "SEARCH_TYPE_TR"}, {"COM", "SEARCH_TYPE_COM"}}},
			{Key: "family_mode", Label: "Family mode", Type: ParamTypeSelect, Default: "FAMILY_MODE_NONE", Options: []ParamOption{{"None", "FAMILY_MODE_NONE"}, {"Moderate", "FAMILY_MODE_MODERATE"}, {"Strict", "FAMILY_MODE_STRICT"}}},
			{Key: "fatal_http", Label: "Fatal HTTP codes", Type: ParamTypeText, Default: "400,401,403"},
			{Key: "retry_http_codes", Label: "Retry HTTP codes", Type: ParamTypeText, Default: "429,500,502,503,504"},
		},
		Hints: []string{
			"Generative answer endpoint /v2/gen/search: put this driver on an answer-mode node so the generated text lands in the request answer panel.",
			"Cited sources are returned as sparse rows (link+title).",
		},
	}
}

func (p yandexGenProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://searchapi.api.cloud.yandex.net"), "/")
	folder := c["folder_id"]
	if folder == "" {
		folder = params["folder_id"]
	}
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	payload, _ := json.Marshal(map[string]any{
		"query": map[string]any{
			"searchType":        defaultStr(params["search_type"], "SEARCH_TYPE_RU"),
			"queryText":         q.Text,
			"familyMode":        defaultStr(params["family_mode"], "FAMILY_MODE_NONE"),
			"getPartialResults": false,
		},
		"folderId": folder,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/gen/search", bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "yandex_gen: build request: "+err.Error())
	}
	req.Header.Set("Authorization", "Api-Key "+c["api_key"])
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "yandex_gen: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex_gen: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, permanent := HTTPClassify(resp.StatusCode, retryHTTP)
		if containsInt(fatalHTTP, resp.StatusCode) {
			permanent = true
		}
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			RetryAfterMS: retryAfterHeaderMS(resp),
			Error:        "yandex_gen: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "yandex_gen: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}

	answer := yandexGenAnswer(data)
	rows := yandexGenSources(data)
	kind := KindOK
	if len(rows) == 0 && answer == "" {
		kind = KindEmpty
	}
	return Result{OK: true, Kind: kind, Rows: rows, Answer: answer, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// yandexGenAnswer tolerantly locates the generated text in the response.
func yandexGenAnswer(data map[string]any) string {
	if v := firstString(data, "answer", "text"); v != "" {
		return v
	}
	if msg, ok := data["message"].(map[string]any); ok {
		return firstString(msg, "text", "answer")
	}
	return ""
}

// yandexGenSources tolerantly maps cited sources (urls+titles) to sparse Rows.
func yandexGenSources(data map[string]any) Rows {
	for _, key := range []string{"sources", "references", "search_results", "searchResults"} {
		arr, ok := data[key].([]any)
		if !ok {
			continue
		}
		var rows Rows
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			link := firstString(m, "url", "link")
			if link == "" {
				continue
			}
			rows = append(rows, Row{Link: link, Title: firstString(m, "title", "name"), Snippet: firstString(m, "snippet", "description")})
		}
		if len(rows) > 0 {
			return rows
		}
	}
	return nil
}
