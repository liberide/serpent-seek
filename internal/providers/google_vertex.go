package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// googleProvider implements the current official Google path: Gemini API
// "Grounding with Google Search", with an optional Vertex AI Search (Enterprise
// Search) mode behind driver_mode. References:
// https://ai.google.dev/gemini-api/docs/grounding
// https://cloud.google.com/generative-ai-app-builder/docs/reference/rest
type googleProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p googleProvider) Code() string { return "google_vertex" }

// Schema describes the provider configuration UI.
func (p googleProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "google_vertex",
		Name:        "Google / Vertex AI",
		Credentials: []CredentialField{{Key: "api_key"}, {Key: "project_id"}, {Key: "engine_id"}},
		Params: []ParamField{
			{Key: "driver_mode", Label: "Driver mode", Type: ParamTypeSelect, Default: "gemini", Options: []ParamOption{{"Gemini grounding", "gemini"}, {"Vertex AI Search", "vertex_search"}, {"Enterprise", "enterprise"}, {"Search", "search"}}},
			{Key: "model", Label: "Gemini model", Type: ParamTypeText, Default: "gemini-2.0-flash"},
			{Key: "fatal_http", Label: "Fatal HTTP codes", Type: ParamTypeText, Default: "400,401,403"},
			{Key: "retry_http_codes", Label: "Retry HTTP codes", Type: ParamTypeText, Default: "429,500,502,503,504"},
			{Key: "location", Label: "Location", Type: ParamTypeText, Default: "global", Hint: "For Vertex AI Search"},
		},
		Hints: []string{
			"Gemini mode uses the API key directly.",
			"Vertex/Enterprise mode needs project_id and engine_id.",
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
	key := c["api_key"]
	model := defaultStr(params["model"], "gemini-2.0-flash")
	base := defaultStr(params["base_url"], "https://generativelanguage.googleapis.com")
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	endpoint := strings.TrimRight(base, "/") + "/v1beta/models/" + url.PathEscape(model) + ":generateContent?key=" + url.QueryEscape(key)
	payload, _ := json.Marshal(map[string]any{
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": q.Text}}}},
		"tools":    []any{map[string]any{"google_search": map[string]any{}}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "google_vertex: build request: "+err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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

// searchEnterprise calls the Vertex AI Search servingConfigs:search endpoint.
func (p googleProvider) searchEnterprise(ctx context.Context, q Query, c Credentials, params Params) Result {
	key := c["api_key"]
	project := defaultStr(c["project_id"], params["project_id"])
	location := defaultStr(params["location"], "global")
	engineID := defaultStr(c["engine_id"], params["engine_id"])
	base := defaultStr(params["base_url"], "https://discoveryengine.googleapis.com")
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	if project == "" || engineID == "" {
		return fail(KindAPI, true, p.Code(), "google_vertex: project_id and engine_id are required for enterprise mode")
	}
	endpoint := strings.TrimRight(base, "/") + "/v1/projects/" + url.PathEscape(project) +
		"/locations/" + url.PathEscape(location) +
		"/collections/default_collection/engines/" + url.PathEscape(engineID) +
		"/servingConfigs/default_serving_config:search?key=" + url.QueryEscape(key)
	searchBody := map[string]any{"query": q.Text}
	if q.Count > 0 {
		searchBody["pageSize"] = q.Count // unset: upstream returns its default page (all found)
	}
	payload, _ := json.Marshal(searchBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fail(KindNet, false, p.Code(), "google_vertex: build request: "+err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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
		return Result{Kind: kind, Permanent: containsInt(fatalHTTP, resp.StatusCode), Provider: p.Code(),
			HTTPStatus: resp.StatusCode, Error: "google_vertex: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_vertex: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	return Result{OK: true, Kind: KindOK, Rows: extractEnterpriseRows(data), Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// extractGroundingText concatenates the generated text parts of the candidate.
func extractGroundingText(data map[string]any) string {
	candidates, _ := data["candidates"].([]any)
	for _, cand := range candidates {
		cm, ok := cand.(map[string]any)
		if !ok {
			continue
		}
		content, _ := cm["content"].(map[string]any)
		if content == nil {
			continue
		}
		parts, _ := content["parts"].([]any)
		var texts []string
		for _, part := range parts {
			pm, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if txt := asString(pm["text"]); strings.TrimSpace(txt) != "" {
				texts = append(texts, strings.TrimSpace(txt))
			}
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

// extractGroundingRows reads candidates[].groundingMetadata.groundingChunks[].web.
func extractGroundingRows(data map[string]any) Rows {
	candidates, _ := data["candidates"].([]any)
	var rows Rows
	seen := map[string]bool{}
	for _, cand := range candidates {
		cm, ok := cand.(map[string]any)
		if !ok {
			continue
		}
		meta, _ := cm["groundingMetadata"].(map[string]any)
		if meta == nil {
			continue
		}
		snippets := groundingSnippets(meta)
		chunks, _ := meta["groundingChunks"].([]any)
		for i, chunk := range chunks {
			chm, ok := chunk.(map[string]any)
			if !ok {
				continue
			}
			web, _ := chm["web"].(map[string]any)
			if web == nil {
				continue
			}
			link := firstString(web, "uri", "url")
			if link == "" || seen[link] {
				continue
			}
			seen[link] = true
			rows = append(rows, Row{
				Link:    link,
				Title:   firstString(web, "title"),
				Snippet: snippets[i],
			})
		}
	}
	return rows
}

// groundingSnippets maps chunk index to supporting segment text.
func groundingSnippets(meta map[string]any) map[int]string {
	out := map[int]string{}
	supports, _ := meta["groundingSupports"].([]any)
	for _, s := range supports {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}
		segment, _ := sm["segment"].(map[string]any)
		text := firstString(segment, "text")
		indices, _ := sm["groundingChunkIndices"].([]any)
		for _, idx := range indices {
			n := asInt(idx)
			if existing := out[n]; existing == "" {
				out[n] = text
			}
		}
	}
	return out
}

// extractEnterpriseRows reads results[].document.derivedStructData.
func extractEnterpriseRows(data map[string]any) Rows {
	results, _ := data["results"].([]any)
	var rows Rows
	for _, r := range results {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		doc, _ := rm["document"].(map[string]any)
		if doc == nil {
			continue
		}
		derived, _ := doc["derivedStructData"].(map[string]any)
		if derived == nil {
			continue
		}
		link := firstString(derived, "link", "url")
		if link == "" {
			continue
		}
		rows = append(rows, Row{
			Link:    link,
			Title:   firstString(derived, "title"),
			Snippet: enterpriseSnippet(derived),
		})
	}
	return rows
}

func enterpriseSnippet(derived map[string]any) string {
	snippets, _ := derived["snippets"].([]any)
	for _, s := range snippets {
		switch t := s.(type) {
		case map[string]any:
			if v := firstString(t, "snippet", "content"); v != "" {
				return v
			}
		case string:
			if strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		}
	}
	return firstString(derived, "snippet")
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
		Credentials: []CredentialField{{Key: "api_key"}, {Key: "cx"}, {Key: "project_id"}, {Key: "engine_id"}},
		Params: []ParamField{
			{Key: "driver", Label: "Driver", Type: ParamTypeSelect, Default: "vertex", Options: []ParamOption{{"Vertex / Gemini", "vertex"}, {"Custom Search (legacy)", "cse"}, {"Custom Search", "customsearch"}, {"Legacy", "legacy"}}},
			{Key: "model", Label: "Gemini model", Type: ParamTypeText, Default: "gemini-2.0-flash"},
		},
		Hints: []string{
			"Dispatcher picks the underlying driver based on the 'driver' parameter.",
			"Use google_vertex or google_cse directly if you do not need dispatch.",
		},
	}
}

func (p googleDispatcher) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	switch strings.ToLower(defaultStr(params["driver"], "vertex")) {
	case "cse", "customsearch", "legacy":
		return p.cse.Search(ctx, q, c, params)
	default:
		return p.vertex.Search(ctx, q, c, params)
	}
}
