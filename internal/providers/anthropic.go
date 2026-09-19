package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// anthropicProvider is an answer-type driver: it calls the Anthropic Messages
// API with the server-side web_search tool and returns the generated text plus
// the cited sources. Sources are sparse Rows (link+title; snippet only when the
// model cited the passage). The answer itself is never mixed into Row.Snippet.
//
// Reference: https://docs.anthropic.com/en/docs/build-with-claude/tool-use/web-search-tool
type anthropicProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p anthropicProvider) Code() string { return "anthropic" }

// Schema describes the provider configuration UI.
func (p anthropicProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "anthropic",
		Name:           "Anthropic (web search)",
		DefaultBaseURL: "https://api.anthropic.com",
		Credentials:    credentialFields("api_key"),
		Params: []ParamField{
			textParam("model", "Model", "claude-sonnet-4-5", "Anthropic model id"),
			numberParam("max_tokens", "Max tokens", "1024", "Answer length budget"),
			numberParam("max_uses", "Max web searches", "5", "web_search tool max_uses"),
			numberParam("max_rps", "Max requests/sec", "", "Per-driver rate limit override"),
			textParam("fatal_http", "Fatal HTTP codes", "400,401,403", ""),
			textParam("retry_http_codes", "Retry HTTP codes", "429,500,502,503,504", ""),
		},
		Hints: []string{
			"Use this driver on an answer-mode node: the generated text goes to the request answer panel.",
			"Sources are returned as sparse rows (link+title); a snippet is present only for cited passages.",
		},
	}
}

func (p anthropicProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://api.anthropic.com"), "/")
	model := strings.TrimSpace(params["model"])
	if model == "" {
		return fail(KindAPI, true, p.Code(), "anthropic: model is required")
	}
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], commonRetry())

	if err := WaitLimit(ctx, p.Code(), maxRPSParam(params, 0)); err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "anthropic: rate limiter wait: " + err.Error()}
	}

	maxTokens := parseInt(defaultStr(params["max_tokens"], "1024"), 1024)
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	maxUses := parseInt(defaultStr(params["max_uses"], "5"), 5)
	if maxUses <= 0 {
		maxUses = 5
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   []any{map[string]any{"role": "user", "content": q.Text}},
		"tools": []any{map[string]any{
			"type": "web_search_20250305", "name": "web_search", "max_uses": maxUses,
		}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "anthropic: build request: "+err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if v := strings.TrimSpace(c["api_key"]); v != "" {
		req.Header.Set("x-api-key", v)
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "anthropic: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "anthropic: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, permanent := HTTPClassify(resp.StatusCode, retryHTTP)
		if containsInt(fatalHTTP, resp.StatusCode) {
			permanent = true
		}
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			RetryAfterMS: retryAfterHeaderMS(resp),
			Error:        "anthropic: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "anthropic: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	answer := anthropicAnswer(data)
	rows := anthropicSources(data)
	if answer == "" {
		return Result{OK: true, Kind: KindEmpty, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
	}
	return Result{OK: true, Kind: KindOK, Answer: answer, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// anthropicAnswer joins the text blocks of the response.
func anthropicAnswer(data map[string]any) string {
	blocks, _ := data["content"].([]any)
	var parts []string
	for _, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if asString(bm["type"]) != "text" {
			continue
		}
		if txt := asString(bm["text"]); txt != "" {
			parts = append(parts, txt)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// anthropicSources merges cited text locations and web_search_tool_result
// entries into deduplicated sparse Rows.
func anthropicSources(data map[string]any) Rows {
	seen := map[string]bool{}
	var rows Rows
	add := func(link, title, snippet string) {
		link = strings.TrimSpace(link)
		if link == "" || seen[link] {
			return
		}
		seen[link] = true
		rows = append(rows, Row{Link: link, Title: strings.TrimSpace(title), Snippet: strings.TrimSpace(snippet)})
	}
	blocks, _ := data["content"].([]any)
	for _, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		switch asString(bm["type"]) {
		case "text":
			citations, _ := bm["citations"].([]any)
			for _, cit := range citations {
				cm, ok := cit.(map[string]any)
				if !ok {
					continue
				}
				add(firstString(cm, "url"), firstString(cm, "title"), firstString(cm, "cited_text"))
			}
		case "web_search_tool_result":
			results, _ := bm["content"].([]any)
			for _, r := range results {
				rm, ok := r.(map[string]any)
				if !ok {
					continue
				}
				if asString(rm["type"]) != "web_search_result" {
					continue
				}
				add(firstString(rm, "url"), firstString(rm, "title"), "")
			}
		}
	}
	return rows
}
