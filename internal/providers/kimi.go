package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// kimiProvider calls the Kimi (Moonshot) Web Search Basic API
// (POST /v1/tools/search). It returns the classic title/url/snippet list and,
// when include_content is on, the full page text in the snippet field.
// Reference: https://platform.kimi.ai/docs/api/tools-search
type kimiProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p kimiProvider) Code() string { return "kimi" }

// Schema describes the provider configuration UI.
func (p kimiProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "kimi",
		Name:           "Kimi (Moonshot) Web Search",
		DefaultBaseURL: "https://api.moonshot.ai",
		Credentials:    credentialFields("api_key"),
		Params: []ParamField{
			numberParam("timeout_seconds", "Timeout seconds", "", "1-60; empty = no per-request timeout"),
			boolParam("include_content", "Include full content", false, "Put the full page text into the result snippet"),
			numberParam("max_rps", "Max requests/sec", "", "Per-driver rate limit override"),
			textParam("fatal_http", "Fatal HTTP codes", "400,401,403", ""),
			textParam("retry_http_codes", "Retry HTTP codes", "429,500,502,503,504", ""),
		},
		Hints: []string{
			"Web Search Basic returns title/url/snippet; enable \"Include full content\" to fetch page text.",
			"Billed per call returning a non-empty result list; empty results are free.",
		},
	}
}

// kimiProProvider calls the Kimi (Moonshot) Web Search Pro API
// (POST /v1/tools/search_pro). Unlike Basic it returns ranked content chunks
// (the most relevant passages) plus site and date-range filtering.
// Reference: https://platform.kimi.ai/docs/api/tools-search-pro
type kimiProProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p kimiProProvider) Code() string { return "kimi_pro" }

// Schema describes the provider configuration UI.
func (p kimiProProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "kimi_pro",
		Name:           "Kimi (Moonshot) Web Search Pro",
		DefaultBaseURL: "https://api.moonshot.ai",
		Credentials:    credentialFields("api_key"),
		Params: []ParamField{
			textParam("sites", "Sites", "", "Comma-separated domains (max 5), OR-combined"),
			textParam("time_window_start", "Date from", "", "YYYY / YYYY-MM / YYYY-MM-DD"),
			textParam("time_window_end", "Date to", "", "YYYY / YYYY-MM / YYYY-MM-DD"),
			numberParam("timeout_seconds", "Timeout seconds", "", "1-60; empty = no per-request timeout"),
			numberParam("max_rps", "Max requests/sec", "", "Per-driver rate limit override"),
			textParam("fatal_http", "Fatal HTTP codes", "400,401,403", ""),
			textParam("retry_http_codes", "Retry HTTP codes", "429,500,502,503,504", ""),
		},
		Hints: []string{
			"Web Search Pro returns ranked content chunks (most relevant passages) per result.",
			"sites and time_window narrow the search; billed per call returning a non-empty result list.",
		},
	}
}

func (p kimiProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://api.moonshot.ai"), "/")
	apiKey := strings.TrimSpace(c["api_key"])
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], commonRetry())

	if err := WaitLimit(ctx, p.Code(), maxRPSParam(params, 0)); err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "kimi: rate limiter wait: " + err.Error()}
	}

	body := map[string]any{"text_query": q.Text}
	if q.Count > 0 {
		body["limit"] = kimiCount(q.Count)
	}
	kimiAddTimeout(body, params["timeout_seconds"])
	includeContent := asBool(params["include_content"], false)
	if includeContent {
		body["include_content"] = true
	}

	payload, _ := json.Marshal(body)
	data, status, res, done := kimiCall(ctx, p.Code(), p.http, base+"/v1/tools/search", apiKey, payload, fatalHTTP, retryHTTP)
	if done {
		return res
	}
	rows := kimiBasicRows(data, includeContent)
	if len(rows) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: status}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: status}
}

func (p kimiProProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://api.moonshot.ai"), "/")
	apiKey := strings.TrimSpace(c["api_key"])
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], commonRetry())

	if err := WaitLimit(ctx, p.Code(), maxRPSParam(params, 0)); err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "kimi_pro: rate limiter wait: " + err.Error()}
	}

	body := map[string]any{"text_query": q.Text}
	if q.Count > 0 {
		body["limit"] = kimiCount(q.Count)
	}
	kimiAddTimeout(body, params["timeout_seconds"])
	if sites := kimiSites(params["sites"]); len(sites) > 0 {
		body["sites"] = sites
	}
	if tw := kimiTimeWindow(params["time_window_start"], params["time_window_end"]); tw != nil {
		body["time_window"] = tw
	}

	payload, _ := json.Marshal(body)
	data, status, res, done := kimiCall(ctx, p.Code(), p.http, base+"/v1/tools/search_pro", apiKey, payload, fatalHTTP, retryHTTP)
	if done {
		return res
	}
	rows := kimiProRows(data)
	if len(rows) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: status}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: status}
}

// kimiCount caps the result count to the API limit (1-20).
func kimiCount(count int) int {
	if count > 20 {
		return 20
	}
	return count
}

// kimiAddTimeout adds timeout_seconds when it is a valid 1-60 value.
func kimiAddTimeout(body map[string]any, raw string) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 60 {
		body["timeout_seconds"] = n
	}
}

// kimiSites splits a comma-separated domain list, capped at 5 entries.
func kimiSites(raw string) []string {
	var out []string
	for _, s := range parseCSV(raw) {
		if len(out) >= 5 {
			break
		}
		out = append(out, s)
	}
	return out
}

// kimiTimeWindow builds the time_window object when at least one bound is set.
func kimiTimeWindow(start, end string) map[string]any {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" && end == "" {
		return nil
	}
	tw := map[string]any{}
	if start != "" {
		tw["start"] = start
	}
	if end != "" {
		tw["end"] = end
	}
	return tw
}

// kimiCall performs one POST and classifies the outcome. On success it returns
// the parsed JSON object and done=false; otherwise a failed Result and done=true.
func kimiCall(ctx context.Context, code string, httpc *HTTPClient, endpoint, apiKey string, payload []byte, fatalHTTP, retryHTTP []int) (map[string]any, int, Result, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fail(KindNet, false, code, code+": build request: "+err.Error()), true
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, 0, Result{Kind: ClassifyTransport(err), Provider: code, Error: code + ": " + httpc.Redact(err.Error())}, true
	}
	body, _, readErr := httpc.ReadBody(resp)
	if readErr != nil {
		return nil, resp.StatusCode, Result{Kind: KindRead, Provider: code, HTTPStatus: resp.StatusCode,
			Error: code + ": body read failed: " + httpc.Redact(readErr.Error())}, true
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, permanent := HTTPClassify(resp.StatusCode, retryHTTP)
		if containsInt(fatalHTTP, resp.StatusCode) {
			permanent = true
		}
		return nil, resp.StatusCode, Result{Kind: kind, Permanent: permanent, Provider: code, HTTPStatus: resp.StatusCode,
			RetryAfterMS: retryAfterHeaderMS(resp),
			Error:        code + ": " + httpc.HTTPError(resp.StatusCode, resp.Status, body)}, true
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return nil, resp.StatusCode, Result{Kind: KindJSON, Provider: code, HTTPStatus: resp.StatusCode,
			Error: code + ": " + httpc.HTTPError(resp.StatusCode, "body is not a json object", body)}, true
	}
	return data, resp.StatusCode, Result{}, false
}

// kimiBasicRows maps the Basic search_results array into Rows.
func kimiBasicRows(data map[string]any, includeContent bool) Rows {
	arr, _ := data["search_results"].([]any)
	rows := make(Rows, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		link := firstString(m, "url")
		if link == "" {
			continue
		}
		snippet := firstString(m, "snippet")
		if includeContent {
			if txt := asString(m["text"]); txt != "" {
				snippet = txt
			}
		}
		rows = append(rows, Row{Link: link, Title: firstString(m, "title"), Snippet: snippet})
	}
	return rows
}

// kimiProRows maps the Pro search_results array into Rows, joining the ranked
// chunks into the snippet and falling back to the plain snippet when absent.
func kimiProRows(data map[string]any) Rows {
	arr, _ := data["search_results"].([]any)
	rows := make(Rows, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		link := firstString(m, "url")
		if link == "" {
			continue
		}
		snippet := kimiChunkSnippet(m)
		if snippet == "" {
			snippet = firstString(m, "snippet")
		}
		rows = append(rows, Row{Link: link, Title: firstString(m, "title"), Snippet: snippet})
	}
	return rows
}

// kimiChunkSnippet joins the text of every chunk in relevance order.
func kimiChunkSnippet(m map[string]any) string {
	chunks, _ := m["chunks"].([]any)
	var parts []string
	for _, ch := range chunks {
		cm, ok := ch.(map[string]any)
		if !ok {
			continue
		}
		if s := asString(cm["text"]); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " … ")
}
