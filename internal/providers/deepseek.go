package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// deepseekProvider implements DeepSeek Search (POST /v1/tools/search).
// need_content requests page content, which replaces summary in the row
// snippet when present.
type deepseekProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p deepseekProvider) Code() string { return "deepseek_search" }

// Schema describes the provider configuration UI.
func (p deepseekProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "deepseek_search",
		Name:           "DeepSeek Search",
		DefaultBaseURL: "https://api.deepseek.com",
		Credentials:    credentialFields("api_key"),
		Params: []ParamField{
			boolParam("need_content", "Include page content", false, "Send need_content=true; the returned page text replaces the summary in the snippet"),
			textParam("ori_question", "Original question", "", "Optional user question that triggered the search (1-128 chars)"),
			textParam("localization", "Localization IP", "", "Optional IP address used to localize results"),
			numberParam("max_rps", "Max requests/sec", "", "Per-driver rate limit override (Search allows 100 rps)"),
			textParam("fatal_http", "Fatal HTTP codes", "400,401,402,403,404", ""),
			textParam("retry_http_codes", "Retry HTTP codes", "408,429,500,502,503,504", ""),
		},
		Hints: []string{
			"Results use the long model-oriented summary as the snippet; enable \"Include page content\" to use the full page text instead.",
			"count is capped at 50; the API default is 10.",
			"Billed per successful request ($0.75 per 1,000); failed and empty requests are free.",
		},
	}
}

func (p deepseekProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://api.deepseek.com"), "/")
	apiKey := strings.TrimSpace(c["api_key"])
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 402, 403, 404})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{408, 429, 500, 502, 503, 504})

	if err := WaitLimit(ctx, p.Code(), maxRPSParam(params, 0)); err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "deepseek_search: rate limiter wait: " + err.Error()}
	}

	body := map[string]any{"query": q.Text}
	if q.Count > 0 {
		body["count"] = deepseekCount(q.Count)
	}
	if v := strings.TrimSpace(params["ori_question"]); v != "" {
		body["ori_question"] = v
	}
	if v := strings.TrimSpace(params["localization"]); v != "" {
		body["localization"] = v
	}
	needContent := asBool(params["need_content"], false)
	if needContent {
		body["need_content"] = true
	}

	payload, _ := json.Marshal(body)
	data, status, res, done := deepseekCall(ctx, p.Code(), p.http, base+"/v1/tools/search", apiKey, payload, fatalHTTP, retryHTTP)
	if done {
		return res
	}
	rows := deepseekRows(data, needContent)
	if len(rows) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: status}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: status}
}

// deepseekCall performs one Bearer-authenticated JSON POST and classifies
// the response.
func deepseekCall(ctx context.Context, code string, httpc *HTTPClient, endpoint, apiKey string, payload []byte, fatalHTTP, retryHTTP []int) (map[string]any, int, Result, bool) {
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

// deepseekCount caps count at the API maximum of 50.
func deepseekCount(count int) int {
	if count > 50 {
		return 50
	}
	return count
}

// deepseekRows maps results to rows: summary is preferred over snippet, and
// non-empty content replaces it when need_content is enabled.
func deepseekRows(data map[string]any, needContent bool) Rows {
	arr, _ := data["results"].([]any)
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
		snippet := firstString(m, "summary", "snippet")
		if needContent {
			if content := firstString(m, "content"); content != "" {
				snippet = content
			}
		}
		rows = append(rows, Row{Link: link, Title: firstString(m, "title"), Snippet: snippet})
	}
	return rows
}
