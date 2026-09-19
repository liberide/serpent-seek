package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// restProvider is a configurable JSON/REST driver used for providers whose
// request/response shape is straightforward enough to describe in a spec.
// Providers with unusual auth, XML payloads, multi-step flows or SigV4 may
// still need their own dedicated type.
type restProvider struct {
	http *HTTPClient
	spec restSpec
}

func (p restProvider) Code() string { return p.spec.code }

func (p restProvider) Schema() ProviderSchema { return p.spec.schema }

func (p restProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	req, err := p.buildRequest(ctx, q, c, params)
	if err != nil {
		return fail(KindAPI, true, p.Code(), fmt.Sprintf("%s: build request: %v", p.Code(), err))
	}

	if err := WaitLimit(ctx, p.spec.code, maxRPSParam(params, p.spec.maxRPS)); err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(),
			Error: fmt.Sprintf("%s: rate limiter wait: %v", p.Code(), err)}
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(),
			Error: fmt.Sprintf("%s: %s", p.Code(), p.http.Redact(err.Error()))}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: fmt.Sprintf("%s: body read failed: %v", p.Code(), p.http.Redact(readErr.Error()))}
	}

	// Per-method throttle hints (Stack Exchange `backoff`): the API contract
	// requires waiting the given number of seconds; the field can arrive even
	// on HTTP 200, so it is checked before the status classification.
	successRetryMS := 0
	if p.spec.throttleKey != "" {
		if data, isJSON := parseJSONObject(body); isJSON {
			if backoff, isNum := data[p.spec.throttleKey].(float64); isNum && backoff > 0 {
				retryMS := int(backoff * 1000)
				if !IsJSONStatus(resp.StatusCode) || data["error_id"] != nil || data["error_name"] != nil {
					msg := fmt.Sprintf("%s: %s backs off for %ds", p.Code(), asString(data["error_name"]), int(backoff))
					if em := asString(data["error_message"]); em != "" {
						msg += " (" + em + ")"
					}
					if quota, ok := data["quota_remaining"].(float64); ok {
						msg += fmt.Sprintf(" quota_remaining=%d", int(quota))
					}
					return Result{Kind: KindAPI, Permanent: false, Provider: p.Code(),
						HTTPStatus: resp.StatusCode, RetryAfterMS: retryMS, Error: msg}
				}
				successRetryMS = retryMS // honoured by the engine before the next attempt
			}
		}
	}

	if !IsJSONStatus(resp.StatusCode) {
		fatal := parseIntCSV(params["fatal_http"], p.spec.fatalHTTP)
		kind, permanent := HTTPClassify(resp.StatusCode, p.retryHTTP(params))
		if containsInt(fatal, resp.StatusCode) {
			permanent = true
		}
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			RetryAfterMS: retryAfterHeaderMS(resp),
			Error:        fmt.Sprintf("%s: %s", p.Code(), p.http.HTTPError(resp.StatusCode, resp.Status, body))}
	}

	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: fmt.Sprintf("%s: %s", p.Code(), p.http.HTTPError(resp.StatusCode, "body is not a json object", body))}
	}

	rows := p.extractRows(data, params)
	// Upstream quota counters (Stack Exchange): surfaced to the engine so it can
	// warn when the remaining quota drops below 10 %.
	quotaRemaining, hasQuota := 0, false
	if q, ok := data["quota_remaining"].(float64); ok {
		quotaRemaining, hasQuota = int(q), true
	}
	quotaMax := 0
	if q, ok := data["quota_max"].(float64); ok {
		quotaMax = int(q)
	}
	if p.spec.isEmpty != nil {
		if p.spec.isEmpty(data, rows) {
			return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: resp.StatusCode, RetryAfterMS: successRetryMS,
				QuotaRemaining: quotaRemaining, QuotaMax: quotaMax, HasQuota: hasQuota}
		}
	} else if len(rows) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: resp.StatusCode, RetryAfterMS: successRetryMS,
			QuotaRemaining: quotaRemaining, QuotaMax: quotaMax, HasQuota: hasQuota}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode, RetryAfterMS: successRetryMS,
		QuotaRemaining: quotaRemaining, QuotaMax: quotaMax, HasQuota: hasQuota}
}

func (p restProvider) buildRequest(ctx context.Context, q Query, c Credentials, params Params) (*http.Request, error) {
	if p.spec.build != nil {
		return p.spec.build(ctx, q, c, params)
	}
	return p.defaultBuild(ctx, q, c, params)
}

func (p restProvider) defaultBuild(ctx context.Context, q Query, c Credentials, params Params) (*http.Request, error) {
	base := strings.TrimRight(defaultStr(params["base_url"], p.spec.baseURL), "/")
	endpoint, err := url.Parse(base + p.spec.path)
	if err != nil {
		return nil, err
	}

	values := endpoint.Query()
	body := map[string]any{}

	add := func(key, value string) {
		if value == "" {
			return
		}
		if p.spec.body {
			body[key] = value
		} else {
			values.Set(key, value)
		}
	}

	if p.spec.queryKey != "" {
		add(p.spec.queryKey, q.Text)
	}
	if p.spec.countField != "" && q.Count > 0 {
		count := q.Count
		if p.spec.countCap > 0 && count > p.spec.countCap {
			count = p.spec.countCap
		}
		add(p.spec.countField, strconv.Itoa(count))
	}
	if p.spec.langField != "" && q.Lang != "" {
		add(p.spec.langField, q.Lang)
	}
	if p.spec.countryField != "" && q.Country != "" {
		add(p.spec.countryField, q.Country)
	}

	for k, v := range params {
		if isInternalParam(k) || v == "" {
			continue
		}
		add(k, v)
	}

	var bodyReader io.Reader
	if p.spec.body {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
		endpoint.RawQuery = values.Encode()
	} else {
		endpoint.RawQuery = values.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, p.spec.method, endpoint.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if p.spec.body {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range p.spec.headers {
		req.Header.Set(k, v)
	}
	p.setAuth(req, c)
	return req, nil
}

func (p restProvider) setAuth(req *http.Request, c Credentials) {
	switch p.spec.auth.kind {
	case "header":
		v := strings.TrimSpace(c[p.spec.auth.credKey])
		if v != "" {
			req.Header.Set(p.spec.auth.header, v)
		}
	case "bearer":
		v := strings.TrimSpace(c[p.spec.auth.credKey])
		if v != "" {
			req.Header.Set("Authorization", "Bearer "+v)
		}
	case "api-key":
		v := strings.TrimSpace(c[p.spec.auth.credKey])
		if v != "" {
			req.Header.Set("Authorization", "Api-Key "+v)
		}
	case "query":
		v := strings.TrimSpace(c[p.spec.auth.credKey])
		if v != "" {
			q := req.URL.Query()
			q.Set(p.spec.auth.queryKey, v)
			req.URL.RawQuery = q.Encode()
		}
	case "basic":
		login := strings.TrimSpace(c[p.spec.auth.loginKey])
		password := strings.TrimSpace(c[p.spec.auth.passwordKey])
		if login != "" {
			req.SetBasicAuth(login, password)
		}
	}
}

func (p restProvider) retryHTTP(params Params) []int {
	return parseIntCSV(params["retry_http_codes"], p.spec.retryHTTP)
}

func (p restProvider) extractRows(data map[string]any, params Params) Rows {
	if p.spec.extractP != nil {
		return p.spec.extractP(data, params)
	}
	if p.spec.extract != nil {
		return p.spec.extract(data)
	}
	if len(p.spec.mergePaths) > 0 {
		var rows Rows
		for _, path := range p.spec.mergePaths {
			arr := getArrayAtPath(data, path)
			rows = append(rows, collectRows(arr, p.spec.linkKeys, p.spec.titleKey, p.spec.snippetKeys)...)
		}
		return rows
	}
	arr := getArrayAtPath(data, p.spec.resultPath)
	return collectRows(arr, p.spec.linkKeys, p.spec.titleKey, p.spec.snippetKeys)
}

// restSpec describes a provider that fits the generic REST/JSON pattern.
type restSpec struct {
	code         string
	name         string
	baseURL      string
	schema       ProviderSchema
	method       string
	path         string
	auth         restAuth
	headers      map[string]string
	body         bool
	queryKey     string
	countField   string
	countCap     int
	langField    string
	countryField string
	resultPath   []string
	linkKeys     []string
	titleKey     string
	snippetKeys  []string
	mergePaths   [][]string
	build        func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error)
	extract      func(data map[string]any) Rows
	// extractP, when set, wins over extract and receives the instance params
	// (needed for configurable field maps like Azure AI Search).
	extractP  func(data map[string]any, params Params) Rows
	isEmpty   func(data map[string]any, rows Rows) bool
	fatalHTTP []int
	retryHTTP []int
	// maxRPS is the driver-level default rate limit (0 = unlimited),
	// overridable per instance via the `max_rps` param.
	maxRPS float64
	// throttleKey is a JSON field in the response carrying a mandatory wait in
	// seconds (Stack Exchange `backoff`); empty disables throttle parsing.
	throttleKey string
}

type restAuth struct {
	kind        string // header | bearer | api-key | query | basic | none
	header      string
	queryKey    string
	credKey     string
	loginKey    string
	passwordKey string
}

func isInternalParam(key string) bool {
	switch key {
	case "base_url", "fatal_http", "retry_http_codes", "ap_retry_codes", "fatal_codes",
		"driver", "driver_mode", "model", "treat_empty_as_fail", "include_gen_answer", "max_rps":
		return true
	}
	return false
}

// retryAfterHeaderMS parses the Retry-After header (delta-seconds or HTTP-date).
func retryAfterHeaderMS(resp *http.Response) int {
	h := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil {
		if n < 0 {
			return 0
		}
		return n * 1000
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return int(d / time.Millisecond)
		}
	}
	return 0
}

func getArrayAtPath(data map[string]any, path []string) []any {
	cur := any(data)
	for _, seg := range path {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[seg]
		default:
			return nil
		}
	}
	if arr, ok := cur.([]any); ok {
		return arr
	}
	return nil
}

func credentialFields(keys ...string) []CredentialField {
	out := make([]CredentialField, 0, len(keys))
	for _, k := range keys {
		out = append(out, CredentialField{Key: k})
	}
	return out
}

func selectParam(key, label, def string, opts ...ParamOption) ParamField {
	return ParamField{Key: key, Label: label, Type: ParamTypeSelect, Default: def, Options: opts}
}

func textParam(key, label, def, hint string) ParamField {
	return ParamField{Key: key, Label: label, Type: ParamTypeText, Default: def, Hint: hint}
}

// requiredTextParam is a text parameter that must be filled before saving.
func requiredTextParam(key, label, def, hint string) ParamField {
	p := textParam(key, label, def, hint)
	p.Required = true
	return p
}

func numberParam(key, label, def, hint string) ParamField {
	return ParamField{Key: key, Label: label, Type: ParamTypeNumber, Default: def, Hint: hint}
}

func boolParam(key, label string, def bool, hint string) ParamField {
	d := "false"
	if def {
		d = "true"
	}
	return ParamField{Key: key, Label: label, Type: ParamTypeBoolean, Default: d, Hint: hint}
}

func commonRetry() []int { return []int{429, 500, 502, 503, 504} }
func commonFatal() []int { return []int{400, 401, 403, 404} }

// newRestRegistry returns the providers described in the integration notes.
func newRestRegistry(client *HTTPClient) []Provider {
	var providers []Provider
	for _, spec := range restSpecs() {
		if spec.schema.DefaultBaseURL == "" {
			spec.schema.DefaultBaseURL = spec.baseURL
		}
		providers = append(providers, restProvider{http: client, spec: spec})
	}
	return providers
}

// restSpecs defines the catalog of generic REST/JSON providers.
func restSpecs() []restSpec {
	return []restSpec{
		// Part 1/5 — classic SERP
		{
			code: "brave", name: "Brave Search", baseURL: "https://api.search.brave.com",
			method: http.MethodGet, path: "/res/v1/web/search",
			auth:     restAuth{kind: "header", header: "X-Subscription-Token", credKey: "api_key"},
			queryKey: "q", countField: "count", countCap: 20,
			langField: "search_lang", countryField: "country",
			resultPath: []string{"web", "results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"description"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			schema: ProviderSchema{
				Code: "brave", Name: "Brave Search",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("country", "Country", "us", "ISO country code"),
					textParam("search_lang", "Search language", "en", "ISO language code"),
					selectParam("safesearch", "Safe search", "off", ParamOption{"Off", "off"}, ParamOption{"Strict", "strict"}),
					selectParam("freshness", "Freshness", "", ParamOption{"Any", ""}, ParamOption{"Past day", "pd"}, ParamOption{"Past week", "pw"}, ParamOption{"Past month", "pm"}, ParamOption{"Past year", "py"}),
					textParam("offset", "Offset", "", "Pagination offset"),
				},
			},
		},
		{
			code: "serper", name: "Serper", baseURL: "https://google.serper.dev",
			method: http.MethodPost, path: "/search", body: true,
			auth:     restAuth{kind: "header", header: "X-API-KEY", credKey: "api_key"},
			queryKey: "q", countField: "num",
			countryField: "gl", langField: "hl",
			resultPath: []string{"organic"},
			linkKeys:   []string{"link", "url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			schema: ProviderSchema{
				Code: "serper", Name: "Serper",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("gl", "Country", "us", "Country code (gl)"),
					textParam("hl", "Language", "en", "Language code (hl)"),
					textParam("location", "Location", "", "Optional location"),
					textParam("page", "Page", "1", "Page number"),
				},
			},
		},

		// Part 2/5 — multi-engine / scrapers
		{
			code: "serpapi", name: "SerpApi", baseURL: "https://serpapi.com",
			method: http.MethodGet, path: "/search",
			auth:     restAuth{kind: "query", queryKey: "api_key", credKey: "api_key"},
			queryKey: "q", countField: "num",
			countryField: "gl", langField: "hl",
			resultPath: []string{"organic_results"},
			linkKeys:   []string{"link", "url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				if meta, ok := data["search_metadata"].(map[string]any); ok {
					if asString(meta["status"]) == "Error" {
						return nil
					}
				}
				return collectRows(data["organic_results"], []string{"link", "url"}, "title", []string{"snippet"})
			},
			isEmpty: func(data map[string]any, rows Rows) bool {
				return len(rows) == 0
			},
			schema: ProviderSchema{
				Code: "serpapi", Name: "SerpApi",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("engine", "Engine", "google", ParamOption{"Google", "google"}, ParamOption{"Bing", "bing"}, ParamOption{"Yandex", "yandex"}, ParamOption{"Yahoo", "yahoo"}, ParamOption{"Baidu", "baidu"}, ParamOption{"Naver", "naver"}, ParamOption{"eBay", "ebay"}, ParamOption{"Walmart", "walmart"}, ParamOption{"YouTube", "youtube"}),
					textParam("gl", "Country", "us", "Country code"),
					textParam("hl", "Language", "en", "Language code"),
					textParam("location", "Location", "", "Optional location/uule"),
					textParam("device", "Device", "", "desktop | mobile | tablet"),
				},
				Hints: []string{"multi-engine provider: use the engine parameter to switch targets."},
			},
		},
		{
			code: "searchapi", name: "SearchApi.io", baseURL: "https://www.searchapi.io",
			method: http.MethodGet, path: "/api/v1/search",
			auth:     restAuth{kind: "query", queryKey: "api_key", credKey: "api_key"},
			queryKey: "q", countField: "num",
			countryField: "gl", langField: "hl",
			resultPath: []string{"organic_results"},
			linkKeys:   []string{"link", "url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				if meta, ok := data["search_metadata"].(map[string]any); ok {
					if asString(meta["status"]) != "Success" && asString(meta["status"]) != "" {
						return nil
					}
				}
				return collectRows(data["organic_results"], []string{"link", "url"}, "title", []string{"snippet"})
			},
			schema: ProviderSchema{
				Code: "searchapi", Name: "SearchApi.io",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("engine", "Engine", "google", ParamOption{"Google", "google"}, ParamOption{"Bing", "bing"}, ParamOption{"Yandex", "yandex"}, ParamOption{"Yahoo", "yahoo"}, ParamOption{"DuckDuckGo", "duckduckgo"}),
					textParam("gl", "Country", "us", ""),
					textParam("hl", "Language", "en", ""),
					textParam("page", "Page", "1", ""),
					textParam("link", "Link resolution", "", "resolved to unwrap Google redirects"),
				},
				Hints: []string{"Google returns 10 results per page; use page for pagination."},
			},
		},
		{
			code: "dataforseo", name: "DataForSEO", baseURL: "https://api.dataforseo.com",
			method: http.MethodPost, path: "/v3/serp/google/organic/live/advanced", body: true,
			auth:     restAuth{kind: "basic", loginKey: "login", passwordKey: "password"},
			queryKey: "keyword", countField: "depth", countCap: 200,
			langField:  "language_code",
			resultPath: []string{}, // custom extract
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"description", "extended_snippet"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.dataforseo.com"), "/")
				task := map[string]any{"keyword": q.Text}
				if q.Count > 0 {
					depth := q.Count
					if depth > 200 {
						depth = 200
					}
					task["depth"] = depth
				}
				if q.Lang != "" {
					task["language_code"] = q.Lang
				}
				if v := p["location_code"]; v != "" {
					task["location_code"] = v
				}
				if v := p["device"]; v != "" {
					task["device"] = v
				}
				body, _ := json.Marshal([]any{task})
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v3/serp/google/organic/live/advanced", bytes.NewReader(body))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				req.SetBasicAuth(c["login"], c["password"])
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				tasks, _ := data["tasks"].([]any)
				var rows Rows
				for _, t := range tasks {
					tm, _ := t.(map[string]any)
					res, _ := tm["result"].([]any)
					for _, r := range res {
						rm, _ := r.(map[string]any)
						items, _ := rm["items"].([]any)
						for _, it := range items {
							im, ok := it.(map[string]any)
							if !ok || asString(im["type"]) != "organic" {
								continue
							}
							if row, ok := toRow(im, []string{"url"}, "title", []string{"description", "extended_snippet"}); ok {
								rows = append(rows, row)
							}
						}
					}
				}
				return rows
			},
			isEmpty: func(data map[string]any, rows Rows) bool {
				return len(rows) == 0
			},
			schema: ProviderSchema{
				Code: "dataforseo", Name: "DataForSEO",
				Credentials: credentialFields("login", "password"),
				Params: []ParamField{
					textParam("location_code", "Location code", "", "DataForSEO location code"),
					selectParam("device", "Device", "", ParamOption{"Default", ""}, ParamOption{"Desktop", "desktop"}, ParamOption{"Mobile", "mobile"}),
				},
				Hints: []string{"Uses HTTP Basic auth.", "Live endpoint can be slow; set a generous node timeout."},
			},
		},
		{
			code: "youcom", name: "You.com", baseURL: "https://ydc-index.io",
			method: http.MethodPost, path: "/v1/search", body: true,
			auth:     restAuth{kind: "header", header: "X-API-Key", credKey: "api_key"},
			queryKey: "query", countField: "count",
			countryField: "country", langField: "language",
			resultPath: []string{}, // custom merge
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"description"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				var rows Rows
				for _, sec := range []string{"web", "news"} {
					if arr, ok := data["results"].(map[string]any)[sec].([]any); ok {
						for _, item := range arr {
							im, ok := item.(map[string]any)
							if !ok {
								continue
							}
							snippet := firstString(im, "description")
							if snippet == "" {
								if sarr, ok := im["snippets"].([]any); ok && len(sarr) > 0 {
									snippet = asString(sarr[0])
								}
							}
							if row, ok := toRow(map[string]any{"url": firstString(im, "url"), "title": firstString(im, "title"), "snippet": snippet}, []string{"url"}, "title", []string{"snippet"}); ok {
								rows = append(rows, row)
							}
						}
					}
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "youcom", Name: "You.com",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("country", "Country", "us", ""),
					textParam("language", "Language", "", ""),
					textParam("freshness", "Freshness", "", ""),
					textParam("offset", "Offset", "", ""),
				},
			},
		},
		{
			code: "jina", name: "Jina AI Search", baseURL: "https://s.jina.ai",
			method: http.MethodGet, path: "/",
			auth:       restAuth{kind: "bearer", credKey: "api_key"},
			headers:    map[string]string{"Accept": "application/json", "X-Respond-With": "no-content"},
			queryKey:   "", // query is encoded into path
			resultPath: []string{"data"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"description", "content"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://s.jina.ai"), "/")
				endpoint := base + "/" + url.PathEscape(q.Text)
				if v := p["site"]; v != "" {
					endpoint += "?site=" + url.QueryEscape(v)
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Accept", "application/json")
				req.Header.Set("X-Respond-With", "no-content")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("Authorization", "Bearer "+v)
				}
				return req, nil
			},
			schema: ProviderSchema{
				Code: "jina", Name: "Jina AI Search",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("site", "Site filter", "", "Restrict search to a domain"),
				},
				Hints: []string{"Query is placed in the URL path; set Accept: application/json for structured results."},
			},
		},
		{
			code: "firecrawl", name: "Firecrawl", baseURL: "https://api.firecrawl.dev",
			method: http.MethodPost, path: "/v2/search", body: true,
			auth:     restAuth{kind: "bearer", credKey: "api_key"},
			queryKey: "query", countField: "limit",
			countryField: "country",
			resultPath:   []string{}, // custom
			linkKeys:     []string{"url"}, titleKey: "title", snippetKeys: []string{"description"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				if d, ok := data["data"].(map[string]any); ok {
					if arr, ok := d["web"].([]any); ok {
						return collectRows(arr, []string{"url"}, "title", []string{"description", "markdown"})
					}
				}
				return nil
			},
			schema: ProviderSchema{
				Code: "firecrawl", Name: "Firecrawl",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("sources", "Sources", "web", "Comma-separated: web, news"),
					textParam("country", "Country", "us", ""),
					textParam("includeDomains", "Include domains", "", ""),
					textParam("excludeDomains", "Exclude domains", "", ""),
					textParam("tbs", "Time filter", "", "Google-style time filter"),
				},
				Hints: []string{"Leave sources as 'web' unless you need news."},
			},
		},
		{
			code: "mojeek", name: "Mojeek", baseURL: "https://api.mojeek.com",
			method: http.MethodGet, path: "/search",
			auth:     restAuth{kind: "query", queryKey: "api_key", credKey: "api_key"},
			queryKey: "q", countField: "t",
			resultPath: []string{"response", "results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"description"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				if s := asString(data["status"]); strings.HasPrefix(s, "ERROR") {
					return nil
				}
				return collectRows(getArrayAtPath(data, []string{"response", "results"}), []string{"url"}, "title", []string{"description"})
			},
			schema: ProviderSchema{
				Code: "mojeek", Name: "Mojeek",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("fmt", "Format", "json", "Use json"),
					textParam("lb", "Language boost", "", ""),
					textParam("lbb", "Language boost weight", "", ""),
					textParam("rb", "Region boost", "", ""),
					textParam("rbb", "Region boost weight", "", ""),
					textParam("safe", "Safe search", "", ""),
					textParam("dlen", "Snippet length", "", ""),
				},
				Hints: []string{"Key is sent in the query string and is redacted from logs automatically."},
			},
		},
		{
			code: "marginalia", name: "Marginalia", baseURL: "https://api2.marginalia-search.com",
			method: http.MethodGet, path: "/search",
			auth:     restAuth{kind: "header", header: "API-Key", credKey: "api_key"},
			queryKey: "query", countField: "count", countCap: 100,
			resultPath: []string{"results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"description"},
			fatalHTTP: commonFatal(), retryHTTP: append(commonRetry(), http.StatusServiceUnavailable),
			schema: ProviderSchema{
				Code: "marginalia", Name: "Marginalia",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					numberParam("count", "Count", "10", "1–100"),
					numberParam("timeout", "Engine timeout", "", "50–250 ms"),
					numberParam("dc", "Max per domain", "", ""),
					textParam("nsfw", "NSFW", "", ""),
					textParam("filter", "Filter", "", ""),
				},
				Hints: []string{"Public API-Key is rate-limited; 503 is retryable."},
			},
		},

		// Part 3/5 — AI/RAG search
		{
			code: "tavily", name: "Tavily", baseURL: "https://api.tavily.com",
			method: http.MethodPost, path: "/search", body: true,
			auth:     restAuth{kind: "bearer", credKey: "api_key"},
			queryKey: "query", countField: "max_results", countCap: 20,
			countryField: "country", langField: "language",
			resultPath: []string{"results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"content"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			schema: ProviderSchema{
				Code: "tavily", Name: "Tavily",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("search_depth", "Search depth", "basic", ParamOption{"Basic", "basic"}, ParamOption{"Advanced", "advanced"}),
					selectParam("topic", "Topic", "general", ParamOption{"General", "general"}, ParamOption{"News", "news"}),
					selectParam("time_range", "Time range", "", ParamOption{"Any", ""}, ParamOption{"Day", "day"}, ParamOption{"Week", "week"}, ParamOption{"Month", "month"}, ParamOption{"Year", "year"}),
					textParam("include_domains", "Include domains", "", "Comma-separated"),
					textParam("exclude_domains", "Exclude domains", "", "Comma-separated"),
				},
				Hints: []string{"Advanced search costs 2 credits per query.", "Long content is truncated automatically."},
			},
		},
		{
			code: "exa", name: "Exa", baseURL: "https://api.exa.ai",
			method: http.MethodPost, path: "/search", body: true,
			auth:     restAuth{kind: "bearer", credKey: "api_key"},
			queryKey: "query", countField: "numResults",
			resultPath: []string{"results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"highlights"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				rows := collectRows(data["results"], []string{"url"}, "title", []string{"highlights"})
				// highlights is an array; collectRows sees the first element only via firstString.
				// Exa often returns highlights[] as a real array, so we handle it explicitly.
				if results, ok := data["results"].([]any); ok {
					rows = rows[:0]
					for _, r := range results {
						rm, ok := r.(map[string]any)
						if !ok {
							continue
						}
						link := firstString(rm, "url")
						if link == "" {
							continue
						}
						snippet := ""
						if h, ok := rm["highlights"].([]any); ok && len(h) > 0 {
							snippet = asString(h[0])
						}
						if snippet == "" {
							snippet = firstString(rm, "summary")
						}
						if snippet == "" {
							snippet = truncateString(firstString(rm, "text"), 400)
						}
						rows = append(rows, Row{Link: link, Title: firstString(rm, "title"), Snippet: snippet})
					}
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "exa", Name: "Exa",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("type", "Query type", "auto", ParamOption{"Auto", "auto"}, ParamOption{"Neural", "neural"}, ParamOption{"Keyword", "keyword"}),
					textParam("includeDomains", "Include domains", "", ""),
					textParam("excludeDomains", "Exclude domains", "", ""),
				},
				Hints: []string{"Highlights are requested by default; avoid text:true unless you need full page content."},
			},
		},
		{
			code: "linkup", name: "Linkup", baseURL: "https://api.linkup.so",
			method: http.MethodPost, path: "/v1/search", body: true,
			auth:     restAuth{kind: "bearer", credKey: "api_key"},
			queryKey: "q", countField: "maxResults",
			resultPath: []string{"results"},
			linkKeys:   []string{"url"}, titleKey: "name", snippetKeys: []string{"content"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.linkup.so"), "/")
				body := map[string]any{
					"q":          q.Text,
					"outputType": "searchResults",
				}
				if q.Count > 0 {
					body["maxResults"] = q.Count
				}
				for k, v := range p {
					if isInternalParam(k) || v == "" || k == "outputType" {
						continue
					}
					body[k] = v
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/search", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("Authorization", "Bearer "+v)
				}
				return req, nil
			},
			schema: ProviderSchema{
				Code: "linkup", Name: "Linkup",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("depth", "Depth", "standard", ParamOption{"Standard", "standard"}, ParamOption{"Deep", "deep"}),
					textParam("includeDomains", "Include domains", "", ""),
					textParam("excludeDomains", "Exclude domains", "", ""),
				},
				Hints: []string{"outputType is fixed to searchResults for the Row contract."},
			},
		},
		{
			code: "perplexity_search", name: "Perplexity Search API", baseURL: "https://api.perplexity.ai",
			method: http.MethodPost, path: "/search", body: true,
			auth:     restAuth{kind: "bearer", credKey: "api_key"},
			queryKey: "query", countField: "max_results", countCap: 50,
			countryField: "country",
			resultPath:   []string{"results"},
			linkKeys:     []string{"url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.perplexity.ai"), "/")
				body := map[string]any{
					"query":       q.Text,
					"search_type": "web",
				}
				if q.Count > 0 {
					body["max_results"] = q.Count
				}
				for k, v := range p {
					if isInternalParam(k) || v == "" || k == "search_type" {
						continue
					}
					body[k] = v
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/search", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("Authorization", "Bearer "+v)
				}
				return req, nil
			},
			schema: ProviderSchema{
				Code: "perplexity_search", Name: "Perplexity Search API",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("search_context_size", "Context size", "medium", ParamOption{"Low", "low"}, ParamOption{"Medium", "medium"}, ParamOption{"High", "high"}),
					selectParam("search_type", "Search type", "web", ParamOption{"Web", "web"}, ParamOption{"People", "people"}),
					textParam("search_recency_filter", "Recency filter", "", "month, week, day, hour"),
					textParam("country", "Country", "us", ""),
				},
				Hints: []string{"This is the standalone /search endpoint, NOT the Sonar chat completions endpoint."},
			},
		},
		{
			code: "valyu", name: "Valyu", baseURL: "https://api.valyu.ai",
			method: http.MethodPost, path: "/v1/search", body: true,
			auth:     restAuth{kind: "header", header: "X-API-Key", credKey: "api_key"},
			queryKey: "query", countField: "max_num_results", countCap: 20,
			countryField: "country_code",
			resultPath:   []string{"results"},
			linkKeys:     []string{"url"}, titleKey: "title", snippetKeys: []string{"content", "description"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			schema: ProviderSchema{
				Code: "valyu", Name: "Valyu",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("search_type", "Search type", "all", ParamOption{"All", "all"}, ParamOption{"Web", "web"}, ParamOption{"Proprietary", "proprietary"}, ParamOption{"News", "news"}),
					selectParam("response_length", "Response length", "short", ParamOption{"Short", "short"}, ParamOption{"Medium", "medium"}, ParamOption{"Long", "long"}),
					textParam("included_sources", "Included sources", "", "Comma-separated"),
					textParam("excluded_sources", "Excluded sources", "", "Comma-separated"),
					textParam("start_date", "Start date", "", "YYYY-MM-DD"),
					textParam("end_date", "End date", "", "YYYY-MM-DD"),
				},
				Hints: []string{"Proprietary search requires a subscription."},
			},
		},
		{
			code: "parallel", name: "Parallel", baseURL: "https://api.parallel.ai",
			method: http.MethodPost, path: "/v1/search", body: true,
			auth:     restAuth{kind: "header", header: "x-api-key", credKey: "api_key"},
			queryKey: "objective", countField: "",
			resultPath: []string{"results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"excerpts"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.parallel.ai"), "/")
				body := map[string]any{
					"objective":      q.Text,
					"search_queries": []string{q.Text},
					"mode":           "advanced",
				}
				for k, v := range p {
					if isInternalParam(k) || v == "" || k == "search_queries" {
						continue
					}
					body[k] = v
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/search", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("x-api-key", v)
				}
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				rows := collectRows(data["results"], []string{"url"}, "title", []string{"excerpts"})
				if results, ok := data["results"].([]any); ok {
					rows = rows[:0]
					for _, r := range results {
						rm, ok := r.(map[string]any)
						if !ok {
							continue
						}
						link := firstString(rm, "url")
						if link == "" {
							continue
						}
						snippet := ""
						if ex, ok := rm["excerpts"].([]any); ok {
							parts := make([]string, 0, len(ex))
							for _, e := range ex {
								parts = append(parts, asString(e))
							}
							snippet = truncateString(strings.Join(parts, " … "), 400)
						}
						rows = append(rows, Row{Link: link, Title: firstString(rm, "title"), Snippet: snippet})
					}
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "parallel", Name: "Parallel",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("mode", "Mode", "advanced", ParamOption{"Advanced", "advanced"}, ParamOption{"Standard", "standard"}),
					numberParam("max_chars_total", "Max total chars", "", ""),
				},
				Hints: []string{"search_queries is built automatically from the query text."},
			},
		},

		// Part 4/5 — Yandex XML, Kagi and vertical APIs
		{
			code: "kagi", name: "Kagi", baseURL: "https://kagi.com",
			method: http.MethodPost, path: "/api/v0/fastgpt", body: true,
			auth:     restAuth{kind: "header", header: "Authorization", credKey: "api_key"},
			queryKey: "q", countField: "",
			resultPath: []string{},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: commonFatal(), retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://kagi.com"), "/")
				body := map[string]any{"q": q.Text}
				b, _ := json.Marshal(body)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v0/fastgpt", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("Authorization", "Bot "+v)
				}
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				if d, ok := data["data"].(map[string]any); ok {
					if arr, ok := d["references"].([]any); ok {
						return collectRows(arr, []string{"url"}, "title", []string{"snippet"})
					}
				}
				if arr, ok := data["data"].([]any); ok {
					return collectRows(arr, []string{"url"}, "title", []string{"snippet"})
				}
				return nil
			},
			schema: ProviderSchema{
				Code: "kagi", Name: "Kagi",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("endpoint", "Endpoint", "fastgpt", ParamOption{"FastGPT", "fastgpt"}, ParamOption{"Enrich Web", "enrich_web"}, ParamOption{"Enrich News", "enrich_news"}, ParamOption{"Search", "search"}),
				},
				Hints: []string{"FastGPT returns references[]; data[] is used for enrich endpoints."},
			},
		},
		{
			code: "openalex", name: "OpenAlex", baseURL: "https://api.openalex.org",
			method: http.MethodGet, path: "/works",
			auth:     restAuth{kind: "none"},
			queryKey: "search", countField: "per-page", countCap: 200,
			resultPath: []string{"results"},
			linkKeys:   []string{"doi", "primary_location.landing_page_url", "id"}, titleKey: "title", snippetKeys: []string{"display_name"},
			fatalHTTP: []int{400}, retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				results, _ := data["results"].([]any)
				var rows Rows
				for _, r := range results {
					rm, ok := r.(map[string]any)
					if !ok {
						continue
					}
					link := firstString(rm, "doi")
					if link != "" && !strings.HasPrefix(link, "http") {
						link = "https://doi.org/" + link
					}
					if link == "" {
						if pl, ok := rm["primary_location"].(map[string]any); ok {
							link = firstString(pl, "landing_page_url")
						}
					}
					if link == "" {
						link = asString(rm["id"])
					}
					if link == "" {
						continue
					}
					title := firstString(rm, "title")
					if title == "" {
						title = firstString(rm, "display_name")
					}
					rows = append(rows, Row{Link: link, Title: title, Snippet: openAlexAbstract(rm)})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "openalex", Name: "OpenAlex",
				Credentials: []CredentialField{},
				Params: []ParamField{
					textParam("filter", "Filter", "", "OpenAlex filter string"),
					textParam("sort", "Sort", "", ""),
					textParam("mailto", "Email", "", "Polite pool email"),
				},
				Hints: []string{"Abstract is reconstructed from the inverted index when available."},
			},
		},
		{
			code: "semanticscholar", name: "Semantic Scholar", baseURL: "https://api.semanticscholar.org",
			method: http.MethodGet, path: "/graph/v1/paper/search",
			auth:     restAuth{kind: "header", header: "x-api-key", credKey: "api_key"},
			queryKey: "query", countField: "limit", countCap: 100,
			resultPath: []string{"data"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"abstract"},
			fatalHTTP: []int{400, 403}, retryHTTP: commonRetry(),
			// Semantic Scholar's shared unauthenticated pool is ~1 rps.
			maxRPS: 1,
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.semanticscholar.org"), "/")
				endpoint, err := url.Parse(base + "/graph/v1/paper/search")
				if err != nil {
					return nil, err
				}
				values := endpoint.Query()
				values.Set("query", q.Text)
				fields := defaultStr(p["fields"], "title,abstract,url,year")
				values.Set("fields", fields)
				if q.Count > 0 {
					limit := q.Count
					if limit > 100 {
						limit = 100
					}
					values.Set("limit", strconv.Itoa(limit))
				}
				endpoint.RawQuery = values.Encode()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Accept", "application/json")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("x-api-key", v)
				}
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				rows := collectRows(data["data"], []string{"url"}, "title", []string{"abstract"})
				if arr, ok := data["data"].([]any); ok {
					rows = rows[:0]
					for _, r := range arr {
						rm, ok := r.(map[string]any)
						if !ok {
							continue
						}
						link := firstString(rm, "url")
						if link == "" {
							if ext, ok := rm["externalIds"].(map[string]any); ok {
								if doi := asString(ext["DOI"]); doi != "" {
									link = doi
									if !strings.HasPrefix(link, "http") {
										link = "https://doi.org/" + link
									}
								}
							}
						}
						if link == "" {
							continue
						}
						rows = append(rows, Row{Link: link, Title: firstString(rm, "title"), Snippet: firstString(rm, "abstract")})
					}
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "semanticscholar", Name: "Semantic Scholar",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("fields", "Fields", "title,abstract,url,year", "Comma-separated paper fields"),
					textParam("offset", "Offset", "", ""),
				},
				Hints: []string{"An API key increases rate limits but is optional."},
			},
		},
		{
			code: "crossref", name: "Crossref", baseURL: "https://api.crossref.org",
			method: http.MethodGet, path: "/works",
			auth:     restAuth{kind: "none"},
			queryKey: "query", countField: "rows", countCap: 1000,
			resultPath: []string{"message", "items"},
			linkKeys:   []string{"URL"}, titleKey: "title", snippetKeys: []string{"abstract"},
			fatalHTTP: []int{400, 404}, retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				items := getArrayAtPath(data, []string{"message", "items"})
				var rows Rows
				for _, it := range items {
					im, ok := it.(map[string]any)
					if !ok {
						continue
					}
					link := asString(im["URL"])
					if link == "" {
						if doi := asString(im["DOI"]); doi != "" {
							link = "https://doi.org/" + doi
						}
					}
					if link == "" {
						continue
					}
					title := ""
					if tarr, ok := im["title"].([]any); ok && len(tarr) > 0 {
						title = asString(tarr[0])
					}
					snippet := firstString(im, "abstract")
					if snippet == "" {
						ct := ""
						if c, ok := im["container-title"].([]any); ok && len(c) > 0 {
							ct = asString(c[0])
						}
						year := ""
						if dp, ok := im["published"].(map[string]any); ok {
							if parts, ok := dp["date-parts"].([]any); ok && len(parts) > 0 {
								if yearArr, ok := parts[0].([]any); ok && len(yearArr) > 0 {
									year = asString(yearArr[0])
								}
							}
						}
						if ct != "" || year != "" {
							snippet = strings.TrimSpace(ct + " " + year)
						}
					}
					rows = append(rows, Row{Link: link, Title: title, Snippet: snippet})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "crossref", Name: "Crossref",
				Credentials: []CredentialField{},
				Params: []ParamField{
					textParam("filter", "Filter", "", "Crossref filter query"),
					textParam("sort", "Sort", "", ""),
					textParam("order", "Order", "", "asc | desc"),
					textParam("mailto", "Email", "", "Polite pool email"),
				},
				Hints: []string{"Title is a list; only the first item is used.", "DOI links are normalized to https://doi.org/."},
			},
		},
		{
			code: "github", name: "GitHub", baseURL: "https://api.github.com",
			method: http.MethodGet, path: "/search/repositories",
			auth:     restAuth{kind: "header", header: "Authorization", credKey: "api_key"},
			queryKey: "q", countField: "per_page", countCap: 100,
			resultPath: []string{"items"},
			linkKeys:   []string{"html_url"}, titleKey: "full_name", snippetKeys: []string{"description"},
			fatalHTTP: []int{401, 422}, retryHTTP: commonRetry(),
			// GitHub Search API: 30 authenticated requests/minute (0.5 rps) is the
			// documented primary limit; unauthenticated callers are lower.
			maxRPS: 0.5,
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.github.com"), "/")
				searchType := defaultStr(p["search_type"], "repositories")
				endpoint, err := url.Parse(base + "/search/" + url.PathEscape(searchType))
				if err != nil {
					return nil, err
				}
				values := endpoint.Query()
				values.Set("q", q.Text)
				if q.Count > 0 {
					perPage := q.Count
					if perPage > 100 {
						perPage = 100
					}
					values.Set("per_page", strconv.Itoa(perPage))
				}
				for k, v := range p {
					if isInternalParam(k) || v == "" || k == "search_type" {
						continue
					}
					values.Set(k, v)
				}
				endpoint.RawQuery = values.Encode()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Accept", "application/vnd.github+json")
				req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("Authorization", "Bearer "+v)
				}
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				items, _ := data["items"].([]any)
				var rows Rows
				for _, it := range items {
					im, ok := it.(map[string]any)
					if !ok {
						continue
					}
					link := firstString(im, "html_url")
					if link == "" {
						continue
					}
					title := firstString(im, "full_name")
					if title == "" {
						title = firstString(im, "name")
					}
					if title == "" {
						title = firstString(im, "title")
					}
					snippet := firstString(im, "description")
					if snippet == "" {
						snippet = firstString(im, "body")
					}
					rows = append(rows, Row{Link: link, Title: title, Snippet: snippet})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "github", Name: "GitHub Search",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					selectParam("search_type", "Search type", "repositories", ParamOption{"Repositories", "repositories"}, ParamOption{"Code", "code"}, ParamOption{"Issues", "issues"}, ParamOption{"Commits", "commits"}, ParamOption{"Users", "users"}),
					textParam("sort", "Sort", "", ""),
					textParam("order", "Order", "", "asc | desc"),
				},
				Hints: []string{"Code search snippets require text-match media type and are not fetched here."},
			},
		},
		{
			code: "hn", name: "Hacker News", baseURL: "https://hn.algolia.com",
			method: http.MethodGet, path: "/api/v1/search",
			auth:     restAuth{kind: "none"},
			queryKey: "query", countField: "hitsPerPage", countCap: 1000,
			resultPath: []string{"hits"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"story_text"},
			fatalHTTP: []int{400}, retryHTTP: commonRetry(),
			extract: func(data map[string]any) Rows {
				hits, _ := data["hits"].([]any)
				var rows Rows
				for _, h := range hits {
					hm, ok := h.(map[string]any)
					if !ok {
						continue
					}
					link := asString(hm["url"])
					if link == "" {
						link = "https://news.ycombinator.com/item?id=" + asString(hm["objectID"])
					}
					if link == "" {
						continue
					}
					title := firstString(hm, "title")
					if title == "" {
						title = firstString(hm, "story_title")
					}
					snippet := firstString(hm, "story_text")
					if snippet == "" {
						snippet = firstString(hm, "comment_text")
					}
					rows = append(rows, Row{Link: link, Title: title, Snippet: snippet})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "hn", Name: "Hacker News",
				Credentials: []CredentialField{},
				Params: []ParamField{
					selectParam("tags", "Tags", "", ParamOption{"Any", ""}, ParamOption{"Story", "story"}, ParamOption{"Comment", "comment"}, ParamOption{"Ask HN", "ask_hn"}, ParamOption{"Show HN", "show_hn"}),
					textParam("numericFilters", "Numeric filters", "", "points>10,num_comments>5"),
					numberParam("page", "Page", "0", "0-based pagination"),
				},
			},
		},
		{
			code: "stackexchange", name: "Stack Exchange", baseURL: "https://api.stackexchange.com",
			method: http.MethodGet, path: "/2.3/search/advanced",
			auth:     restAuth{kind: "query", queryKey: "key", credKey: "api_key"},
			queryKey: "q", countField: "pagesize", countCap: 100,
			resultPath: []string{"items"},
			linkKeys:   []string{"link"}, titleKey: "title", snippetKeys: []string{"body"},
			fatalHTTP: []int{400, 401, 403}, retryHTTP: commonRetry(),
			throttleKey: "backoff",
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.stackexchange.com"), "/")
				endpoint, err := url.Parse(base + "/2.3/search/advanced")
				if err != nil {
					return nil, err
				}
				values := endpoint.Query()
				values.Set("q", q.Text)
				site := defaultStr(p["site"], "stackoverflow")
				values.Set("site", site)
				values.Set("filter", "withbody")
				if q.Count > 0 {
					pagesize := q.Count
					if pagesize > 100 {
						pagesize = 100
					}
					values.Set("pagesize", strconv.Itoa(pagesize))
				}
				for k, v := range p {
					if isInternalParam(k) || v == "" || k == "site" {
						continue
					}
					values.Set(k, v)
				}
				endpoint.RawQuery = values.Encode()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Accept", "application/json")
				// Responses are gzip by default; the Go transport handles it transparently.
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				items, _ := data["items"].([]any)
				var rows Rows
				for _, it := range items {
					im, ok := it.(map[string]any)
					if !ok {
						continue
					}
					link := firstString(im, "link")
					if link == "" {
						link = "https://stackoverflow.com/q/" + asString(im["question_id"])
					}
					if link == "" {
						continue
					}
					body := stripHTML(firstString(im, "body"))
					rows = append(rows, Row{Link: link, Title: firstString(im, "title"), Snippet: truncateString(body, 400)})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "stackexchange", Name: "Stack Exchange",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					textParam("site", "Site", "stackoverflow", "e.g. stackoverflow, serverfault"),
					textParam("tagged", "Tagged", "", "Semicolon-separated tags"),
					textParam("nottagged", "Not tagged", "", ""),
					selectParam("sort", "Sort", "relevance", ParamOption{"Relevance", "relevance"}, ParamOption{"Activity", "activity"}, ParamOption{"Votes", "votes"}, ParamOption{"Creation", "creation"}),
					selectParam("order", "Order", "desc", ParamOption{"Descending", "desc"}, ParamOption{"Ascending", "asc"}),
				},
				Hints: []string{"Responses are gzip; the HTTP client handles decompression."},
			},
		},

		// Part 5/5 — enterprise engines
		{
			code: "azure_search", name: "Azure AI Search", baseURL: "https://{}.search.windows.net",
			method: http.MethodPost, path: "", body: true,
			auth:     restAuth{kind: "header", header: "api-key", credKey: "api_key"},
			queryKey: "search", countField: "top",
			resultPath: []string{"value"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: []int{400, 401, 403, 404, 413}, retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				service := defaultStr(p["service_name"], p["service"])
				index := defaultStr(p["index_name"], p["index"])
				if service == "" || index == "" {
					return nil, fmt.Errorf("service_name and index_name are required")
				}
				base := strings.TrimRight(defaultStr(p["base_url"], fmt.Sprintf("https://%s.search.windows.net", service)), "/")
				apiVersion := defaultStr(p["api_version"], "2024-07-01")
				endpoint := fmt.Sprintf("%s/indexes/%s/docs/search?api-version=%s", base, url.PathEscape(index), url.QueryEscape(apiVersion))
				body := map[string]any{
					"search":    q.Text,
					"queryType": defaultStr(p["query_type"], "simple"),
				}
				if q.Count > 0 {
					body["top"] = q.Count
				}
				// Auto-select: request only the mapped fields so responses stay small.
				if v := p["select"]; v != "" {
					body["select"] = v
				} else {
					fields := []string{azureTitleField(p), azureURLField(p), azureSnippetField(p)}
					body["select"] = strings.Join(fields, ",")
				}
				for k, val := range p {
					if isInternalParam(k) || val == "" || k == "service_name" || k == "index_name" || k == "service" || k == "index" || k == "api_version" || k == "query_type" || k == "semanticConfiguration" || k == "select" || k == "auth_mode" || k == "title_field" || k == "url_field" || k == "snippet_field" {
						continue
					}
					body[k] = val
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				switch strings.ToLower(defaultStr(p["auth_mode"], "api_key")) {
				case "aad", "entra", "oauth2":
					// Microsoft Entra ID (RBAC): the service needs Search Index Data Reader.
					tok, err := entraToken(ctx, c)
					if err != nil {
						return nil, fmt.Errorf("entra token: %w", err)
					}
					req.Header.Set("Authorization", "Bearer "+tok)
				default:
					if v := strings.TrimSpace(c["api_key"]); v != "" {
						req.Header.Set("api-key", v)
					}
				}
				return req, nil
			},
			extractP: func(data map[string]any, p Params) Rows {
				titleField := azureTitleField(p)
				urlField := azureURLField(p)
				snippetField := azureSnippetField(p)
				values, _ := data["value"].([]any)
				var rows Rows
				for _, v := range values {
					vm, ok := v.(map[string]any)
					if !ok {
						continue
					}
					link := asString(vm[urlField])
					if link == "" {
						continue
					}
					// Semantic captions win; fall back to the mapped snippet field.
					snippet := ""
					if caps, ok := vm["@search.captions"].([]any); ok && len(caps) > 0 {
						if cm, ok := caps[0].(map[string]any); ok {
							snippet = firstString(cm, "text")
						}
					}
					if snippet == "" {
						snippet = asString(vm[snippetField])
					}
					rows = append(rows, Row{Link: link, Title: asString(vm[titleField]), Snippet: truncateString(snippet, 400)})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "azure_search", Name: "Azure AI Search",
				Credentials: credentialFields("api_key", "tenant_id", "client_id", "client_secret"),
				Params: []ParamField{
					requiredTextParam("service_name", "Service name", "", "Azure Search service name"),
					requiredTextParam("index_name", "Index name", "", ""),
					selectParam("auth_mode", "Auth mode", "api_key", ParamOption{"API key", "api_key"}, ParamOption{"Microsoft Entra (RBAC)", "aad"}),
					selectParam("query_type", "Query type", "simple", ParamOption{"Simple", "simple"}, ParamOption{"Full", "full"}, ParamOption{"Semantic", "semantic"}),
					textParam("semanticConfiguration", "Semantic config", "", "Required for semantic"),
					textParam("title_field", "Title field", "title", "Index field mapped to Row.title"),
					textParam("url_field", "URL field", "url", "Index field mapped to Row.link"),
					textParam("snippet_field", "Snippet field", "description", "Index field mapped to Row.snippet (captions win when semantic)"),
					textParam("select", "Select fields", "", "Override; default: the three mapped fields"),
					textParam("api_version", "API version", "2024-07-01", ""),
				},
				Hints: []string{"Index field names are configurable: set title_field/url_field/snippet_field to your index schema. Entra mode requires the Search Index Data Reader role."},
			},
		},
		{
			code: "vectara", name: "Vectara", baseURL: "https://api.vectara.io",
			method: http.MethodPost, path: "/v2/query", body: true,
			auth:     restAuth{kind: "header", header: "x-api-key", credKey: "api_key"},
			queryKey: "query", countField: "limit",
			resultPath: []string{"search_results"},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"text"},
			fatalHTTP: []int{400, 403, 404}, retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				base := strings.TrimRight(defaultStr(p["base_url"], "https://api.vectara.io"), "/")
				corpus := defaultStr(p["corpus_key"], "")
				if corpus == "" {
					return nil, fmt.Errorf("corpus_key is required")
				}
				body := map[string]any{
					"query": q.Text,
					"search": map[string]any{
						"corpora": []any{map[string]any{"corpus_key": corpus}},
					},
					"generation": nil,
				}
				if q.Count > 0 {
					body["search"].(map[string]any)["limit"] = q.Count
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/query", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json")
				if v := strings.TrimSpace(c["api_key"]); v != "" {
					req.Header.Set("x-api-key", v)
				}
				return req, nil
			},
			extract: func(data map[string]any) Rows {
				results, _ := data["search_results"].([]any)
				var rows Rows
				for _, r := range results {
					rm, ok := r.(map[string]any)
					if !ok {
						continue
					}
					meta, _ := rm["metadata"].(map[string]any)
					if meta == nil {
						continue
					}
					link := firstString(meta, "url", "source")
					if link == "" {
						continue
					}
					rows = append(rows, Row{Link: link, Title: firstString(meta, "title"), Snippet: truncateString(firstString(rm, "text"), 400)})
				}
				return rows
			},
			schema: ProviderSchema{
				Code: "vectara", Name: "Vectara",
				Credentials: credentialFields("api_key"),
				Params: []ParamField{
					requiredTextParam("corpus_key", "Corpus key", "", "Required corpus key"),
					textParam("metadata_filter", "Metadata filter", "", ""),
				},
				Hints: []string{"generation is disabled so the response stays in Row format."},
			},
		},
		{
			code: "kendra", name: "Amazon Kendra", baseURL: "https://kendra.us-east-1.amazonaws.com",
			method: http.MethodPost, path: "", body: true,
			auth:       restAuth{kind: "none"},
			queryKey:   "", // SigV4 not implemented
			resultPath: []string{},
			linkKeys:   []string{"url"}, titleKey: "title", snippetKeys: []string{"snippet"},
			fatalHTTP: []int{400}, retryHTTP: commonRetry(),
			build: func(ctx context.Context, q Query, c Credentials, p Params) (*http.Request, error) {
				return nil, fmt.Errorf("Amazon Kendra requires AWS SigV4 signing and is not yet implemented")
			},
			extract: func(data map[string]any) Rows { return nil },
			schema: ProviderSchema{
				Code: "kendra", Name: "Amazon Kendra", Deprecated: true,
				Credentials: credentialFields("access_key_id", "secret_access_key"),
				Params: []ParamField{
					textParam("index_id", "Index ID", "", ""),
					textParam("region", "Region", "us-east-1", ""),
					selectParam("QueryResultTypeFilter", "Result type filter", "DOCUMENT", ParamOption{"Document", "DOCUMENT"}, ParamOption{"QUESTION_ANSWER", "QUESTION_ANSWER"}, ParamOption{"ANSWER", "ANSWER"}),
				},
				Hints: []string{"AWS moved Kendra to maintenance mode (2026-06-30) and closed it to new customers (2026-07-30) → use Bedrock Knowledge Bases. AWS SigV4 signing is intentionally not implemented."},
			},
		},
	}
}

func openAlexAbstract(data map[string]any) string {
	index, _ := data["abstract_inverted_index"].(map[string]any)
	if len(index) == 0 {
		return ""
	}
	positions := map[int]string{}
	for word, arr := range index {
		if arr == nil {
			continue
		}
		list, ok := arr.([]any)
		if !ok {
			continue
		}
		for _, pos := range list {
			n := asInt(pos)
			if n >= 0 {
				positions[n] = word
			}
		}
	}
	if len(positions) == 0 {
		return ""
	}
	maxPos := 0
	for n := range positions {
		if n > maxPos {
			maxPos = n
		}
	}
	words := make([]string, 0, maxPos+1)
	for i := 0; i <= maxPos; i++ {
		if w, ok := positions[i]; ok {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

func truncateString(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	// Trim to last space before max to avoid cutting a word.
	cut := s[:max]
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

func stripHTML(s string) string {
	// Minimal HTML tag removal; enough for Stack Exchange body snippets.
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}
