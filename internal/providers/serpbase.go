package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// serpbaseProvider queries serpbase.dev via POST /google/search.
// Code table (v11): permanent statuses 1000,1001,1004,1020; temporary 1029,1500,
// 1502,1503,1504 and any unknown nonzero status; permanent HTTP 401/403.
type serpbaseProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p serpbaseProvider) Code() string { return "serpbase" }

// Schema describes the provider configuration UI.
func (p serpbaseProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "serpbase",
		Name:           "SerpBase",
		DefaultBaseURL: "https://api.serpbase.dev",
		Credentials:    []CredentialField{{Key: "api_key"}},
		Params: []ParamField{
			{Key: "hl", Label: "Interface language", Type: ParamTypeText, Default: "en"},
			{Key: "gl", Label: "Country code", Type: ParamTypeText, Default: "us"},
			boolParam("resolve_google_links", "Resolve Google links", true,
				"Unwrap google.com/goto and /url result links via SerpBase /google/resolve (free, up to 50 links per search)."),
			{Key: "fatal_codes", Label: "Fatal status codes", Type: ParamTypeText, Default: "1000,1001,1004,1020"},
			{Key: "fatal_http", Label: "Fatal HTTP codes", Type: ParamTypeText, Default: "401,403"},
			{Key: "retry_http_codes", Label: "Retry HTTP codes", Type: ParamTypeText, Default: "429,500,502,503,504"},
		},
	}
}

func (p serpbaseProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://api.serpbase.dev"), "/")
	key := c["api_key"]

	gl := defaultStr(params["gl"], defaultStr(q.Country, "us"))
	hl := defaultStr(params["hl"], defaultStr(q.Lang, "en"))

	fatalCodes := parseIntCSV(params["fatal_codes"], []int{1000, 1001, 1004, 1020})
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	retryable := func(status *int, httpCode int) bool {
		if status == nil {
			return !containsInt(fatalHTTP, httpCode) && containsInt(retryHTTP, httpCode)
		}
		return !containsInt(fatalCodes, *status) && !containsInt(fatalHTTP, *status)
	}

	payload, _ := json.Marshal(map[string]any{
		"q":      q.Text,
		"hl":     hl,
		"gl":     gl,
		"page":   1,
		"device": "default",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/google/search", bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "serpbase: build request: "+err.Error())
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-SerpBase-Source", "serpentseek")

	resp, err := p.http.Do(req)
	if err != nil {
		kind := ClassifyTransport(err)
		return Result{Kind: kind, Provider: p.Code(), Error: "serpbase: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "serpbase: body read failed: " + p.http.Redact(readErr.Error())}
	}
	data, isJSON := parseJSONObject(body)
	status, hasStatus := serpbaseStatus(data)

	if !IsJSONStatus(resp.StatusCode) {
		fatalHTTPHit := containsInt(fatalHTTP, resp.StatusCode)
		kind := KindHTTP
		if isJSON && hasStatus && !fatalHTTPHit {
			kind = KindAPI
		}
		msg := "serpbase: HTTP " + itoa(resp.StatusCode) + " " + resp.Status
		if isJSON {
			if hasStatus {
				msg += " status=" + itoa(status) + " error=" + p.http.Redact(asString(data["error"]))
			} else {
				msg += " " + p.http.HTTPError(resp.StatusCode, "", body)
			}
		} else {
			msg += " " + p.http.HTTPError(resp.StatusCode, "", body)
		}
		var statusPtr *int
		if hasStatus {
			statusPtr = &status
		}
		permanent := fatalHTTPHit || !retryable(statusPtr, resp.StatusCode)
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: resp.StatusCode, Error: msg}
	}
	if !isJSON {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "serpbase: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	if hasStatus && status != 0 {
		var statusPtr = &status
		msg := "serpbase: HTTP " + itoa(resp.StatusCode) + " status=" + itoa(status) +
			" error=" + p.http.Redact(asString(data["error"])) +
			" request_id=" + asString(firstOf(data, "request_id"))
		return Result{Kind: KindAPI, Permanent: !retryable(statusPtr, resp.StatusCode),
			Provider: p.Code(), HTTPStatus: resp.StatusCode, Error: msg}
	}
	rows := collectRows(data["organic"], []string{"link", "url"}, "title", []string{"snippet", "description", "content"})
	if serpbaseResolveEnabled(params) {
		rows = p.resolveGoogleLinks(ctx, base, key, rows)
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// serpbaseResolveMaxLinks is the upstream limit of one /google/resolve call.
const serpbaseResolveMaxLinks = 50

// serpbaseResolveSearchTimeout covers both the search and the resolver call.
// SerpBase documents that resolving links can take about 25 seconds and uses
// a 60-second timeout in its official cURL example.
const serpbaseResolveSearchTimeout = 60 * time.Second

// MinimumSearchTimeout prevents the engine's 15/20-second defaults from
// cancelling the resolver before it can answer. A longer configured timeout
// still takes precedence in the engine.
func (p serpbaseProvider) MinimumSearchTimeout(params Params) time.Duration {
	if serpbaseResolveEnabled(params) {
		return serpbaseResolveSearchTimeout
	}
	return 0
}

// serpbaseResolveEnabled reports whether link resolution is enabled. Default: true.
func serpbaseResolveEnabled(params Params) bool {
	v := strings.TrimSpace(params["resolve_google_links"])
	if v == "" {
		return true
	}
	return asBool(v, true)
}

// resolveGoogleLinks unwraps Google result links through /google/resolve.
// Resolution is best effort: failures leave the original rows unchanged.
func (p serpbaseProvider) resolveGoogleLinks(ctx context.Context, base, key string, rows Rows) Rows {
	links := googleWrapperLinks(rows)
	if len(links) == 0 {
		return rows
	}
	payload, err := json.Marshal(map[string]any{"links": links})
	if err != nil {
		return rows
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/google/resolve", bytes.NewReader(payload))
	if err != nil {
		return rows
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-SerpBase-Source", "serpentseek")

	resp, err := p.http.Do(req)
	if err != nil {
		return rows
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil || !IsJSONStatus(resp.StatusCode) {
		return rows
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return rows
	}
	list, ok := data["resolved_links"].([]any)
	if !ok {
		return rows
	}
	resolved := make(map[string]string, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		link := asString(m["link"])
		target := asString(m["url"])
		reason := asString(m["reason"])
		if link == "" || target == "" {
			continue
		}
		if reason != "" && reason != "resolved" {
			continue
		}
		// Reject non-HTTP(S) targets from the resolver.
		if !isHTTPLink(target) {
			continue
		}
		resolved[link] = target
	}
	if len(resolved) == 0 {
		return rows
	}
	out := make(Rows, len(rows))
	copy(out, rows)
	for i := range out {
		if target, ok := resolved[strings.TrimSpace(out[i].Link)]; ok {
			out[i].Link = target
		}
	}
	return out
}

// googleWrapperLinks returns up to serpbaseResolveMaxLinks unique Google
// wrappers in row order.
func googleWrapperLinks(rows Rows) []string {
	seen := make(map[string]bool, len(rows))
	links := make([]string, 0, len(rows))
	for _, row := range rows {
		link := strings.TrimSpace(row.Link)
		if link == "" || seen[link] || !isGoogleResultWrapper(link) {
			continue
		}
		seen[link] = true
		links = append(links, link)
		if len(links) == serpbaseResolveMaxLinks {
			break
		}
	}
	return links
}

// isGoogleResultWrapper reports whether raw is a Google /url or /goto wrapper.
func isGoogleResultWrapper(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return false
	}
	if !isGoogleSearchHost(strings.ToLower(u.Hostname())) {
		return false
	}
	switch strings.ToLower(strings.TrimSuffix(u.Path, "/")) {
	case "/url", "/goto":
		return true
	}
	return false
}

// isGoogleSearchHost matches google.com and Google country domains while
// rejecting lookalike hosts.
func isGoogleSearchHost(host string) bool {
	host = strings.TrimSuffix(strings.TrimPrefix(host, "www."), ".")
	if host == "google.com" || host == "google.cat" {
		return true
	}
	if !strings.HasPrefix(host, "google.") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(host, "google."), ".")
	switch len(parts) {
	case 1:
		return len(parts[0]) == 2
	case 2:
		if parts[0] == "com" || parts[0] == "co" {
			return len(parts[1]) == 2
		}
		return len(parts[0]) == 2 && len(parts[1]) == 2
	default:
		return false
	}
}

// isHTTPLink reports whether raw is an absolute http(s) URL with a host.
func isHTTPLink(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	}
	return false
}

// serpbaseStatus extracts the numeric status field. The second result reports
// whether the field was present at all; an absent status is a valid response.
func serpbaseStatus(data map[string]any) (int, bool) {
	if data == nil {
		return 0, false
	}
	v, ok := data["status"]
	if !ok || v == nil {
		return 0, false
	}
	return asInt(v), true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func firstOf(data map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := data[k]; ok && v != nil {
			return v
		}
	}
	return nil
}
