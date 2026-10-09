package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// googleProvider implements the current official Google path: Gemini API
// "Grounding with Google Search" through the Interactions API
// (POST /v1beta/interactions), with an optional Vertex AI Search (Enterprise
// Search) mode behind driver_mode that delegates to the OAuth2 driver.
// References:
// https://ai.google.dev/gemini-api/docs/google-search
// https://cloud.google.com/generative-ai-app-builder/docs/reference/rest
type googleProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p googleProvider) Code() string { return "google_vertex" }

// Schema describes the provider configuration UI.
func (p googleProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "google_vertex",
		Name:           "Google / Vertex AI",
		DefaultBaseURL: "https://generativelanguage.googleapis.com",
		Credentials:    []CredentialField{{Key: "api_key"}, {Key: "service_account_json", Multiline: true}, {Key: "project_id"}, {Key: "engine_id"}},
		Params: []ParamField{
			{Key: "driver_mode", Label: "Driver mode", Type: ParamTypeSelect, Default: "gemini", Options: []ParamOption{{"Gemini grounding", "gemini"}, {"Vertex AI Search", "vertex_search"}, {"Enterprise", "enterprise"}, {"Search", "search"}}},
			{Key: "model", Label: "Gemini model", Type: ParamTypeText, Default: "gemini-3.8-flash"},
			{Key: "fatal_http", Label: "Fatal HTTP codes", Type: ParamTypeText, Default: "400,401,403"},
			{Key: "retry_http_codes", Label: "Retry HTTP codes", Type: ParamTypeText, Default: "429,500,502,503,504"},
			{Key: "location", Label: "Location", Type: ParamTypeText, Default: "global", Hint: "For Vertex AI Search"},
			{Key: "serving_config", Label: "Serving config", Type: ParamTypeText, Default: "default_search", Hint: "For Vertex AI Search"},
		},
		Hints: []string{
			"Gemini mode uses the api_key directly (Interactions API /v1beta/interactions).",
			"Vertex/Enterprise mode uses OAuth2: set service_account_json plus project_id/engine_id; Vertex AI Search does not accept API keys.",
		},
	}
}

func (p googleProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	mode := strings.ToLower(defaultStr(params["driver_mode"], defaultStr(params["driver"], "gemini")))
	if mode == "enterprise" || mode == "vertex_search" || mode == "search" {
		return p.searchEnterprise(ctx, q, c, params)
	}
	return p.searchGrounding(ctx, q, c, params)
}

// searchGrounding calls the Gemini API with the google_search tool.
func (p googleProvider) searchGrounding(ctx context.Context, q Query, c Credentials, params Params) Result {
	key := strings.TrimSpace(c["api_key"])
	model := defaultStr(params["model"], "gemini-3.8-flash")
	base := defaultStr(params["base_url"], "https://generativelanguage.googleapis.com")
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	endpoint := strings.TrimRight(base, "/") + "/v1beta/interactions"
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"input": q.Text,
		"tools": []any{map[string]any{"type": "google_search"}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "google_vertex: build request: "+err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", key)

	resp, err := p.http.Do(req)
	if err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "google_vertex: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_vertex: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, _ := HTTPClassify(resp.StatusCode, retryHTTP)
		permanent := containsInt(fatalHTTP, resp.StatusCode)
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_vertex: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_vertex: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	rows := extractGroundingRows(data)
	answer := extractGroundingText(data)
	if len(rows) == 0 && answer == "" {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: resp.StatusCode}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Answer: answer, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// searchEnterprise delegates to the OAuth2-based Vertex AI Search driver:
// Discovery Engine does not accept API keys, so project_id/engine_id are merged
// from the credentials when they are not provided as node params.
func (p googleProvider) searchEnterprise(ctx context.Context, q Query, c Credentials, params Params) Result {
	merged := make(Params, len(params)+2)
	for k, v := range params {
		merged[k] = v
	}
	if merged["project_id"] == "" {
		merged["project_id"] = c["project_id"]
	}
	if merged["engine_id"] == "" {
		merged["engine_id"] = c["engine_id"]
	}
	res := vertexSearchProvider{http: p.http}.Search(ctx, q, c, merged)
	res.Provider = p.Code()
	return res
}

// extractGroundingText joins the text parts of the model_output step.
func extractGroundingText(data map[string]any) string {
	steps, _ := data["steps"].([]any)
	var parts []string
	for _, s := range steps {
		sm, ok := s.(map[string]any)
		if !ok || asString(sm["type"]) != "model_output" {
			continue
		}
		content, _ := sm["content"].([]any)
		for _, c := range content {
			cm, ok := c.(map[string]any)
			if !ok || asString(cm["type"]) != "text" {
				continue
			}
			if txt := strings.TrimSpace(asString(cm["text"])); txt != "" {
				parts = append(parts, txt)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// extractGroundingRows maps url_citation annotations from steps[] to Rows.
func extractGroundingRows(data map[string]any) Rows {
	steps, _ := data["steps"].([]any)
	var rows Rows
	seen := map[string]bool{}
	for _, s := range steps {
		sm, ok := s.(map[string]any)
		if !ok || asString(sm["type"]) != "model_output" {
			continue
		}
		content, _ := sm["content"].([]any)
		for _, c := range content {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			text := asString(cm["text"])
			anns, _ := cm["annotations"].([]any)
			for _, a := range anns {
				am, ok := a.(map[string]any)
				if !ok || asString(am["type"]) != "url_citation" {
					continue
				}
				link := firstString(am, "url")
				if link == "" || seen[link] {
					continue
				}
				seen[link] = true
				rows = append(rows, Row{
					Link:    link,
					Title:   firstString(am, "title"),
					Snippet: citationSnippet(text, asInt(am["start_index"]), asInt(am["end_index"])),
				})
			}
		}
	}
	return rows
}

// citationSnippet slices the cited span out of the model output text. Citation
// indices are character offsets, so the text is sliced by runes.
func citationSnippet(text string, start, end int) string {
	if text == "" || start < 0 || end <= start {
		return ""
	}
	runes := []rune(text)
	if start >= len(runes) {
		return ""
	}
	if end > len(runes) {
		end = len(runes)
	}
	return strings.TrimSpace(string(runes[start:end]))
}

// googleDispatcher lets the node select the driver via params["driver"].
type googleDispatcher struct {
	vertex googleProvider
	cse    googleCSEProvider
}

// Code returns the provider identifier.
func (p googleDispatcher) Code() string { return "google" }

// Schema describes the provider configuration UI.
func (p googleDispatcher) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "google",
		Name:        "Google (dispatcher)",
		Credentials: []CredentialField{{Key: "api_key"}, {Key: "service_account_json", Multiline: true}, {Key: "cx"}, {Key: "project_id"}, {Key: "engine_id"}},
		Params: []ParamField{
			{Key: "driver", Label: "Driver", Type: ParamTypeSelect, Default: "vertex", Options: []ParamOption{{"Vertex / Gemini", "vertex"}, {"Vertex AI Search", "vertex_search"}, {"Custom Search (legacy)", "cse"}, {"Custom Search", "customsearch"}, {"Legacy", "legacy"}}},
			{Key: "model", Label: "Gemini model", Type: ParamTypeText, Default: "gemini-3.8-flash"},
		},
		Hints: []string{
			"Dispatcher picks the underlying driver based on the 'driver' parameter.",
			"Vertex AI Search uses OAuth2: set service_account_json with project_id/engine_id.",
		},
	}
}

func (p googleDispatcher) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	switch strings.ToLower(defaultStr(params["driver"], "vertex")) {
	case "cse", "customsearch", "legacy":
		return p.cse.Search(ctx, q, c, params)
	case "vertex_search", "enterprise", "search":
		merged := make(Params, len(params)+1)
		for k, v := range params {
			merged[k] = v
		}
		merged["driver_mode"] = "enterprise"
		return p.vertex.Search(ctx, q, c, merged)
	default:
		return p.vertex.Search(ctx, q, c, params)
	}
}
