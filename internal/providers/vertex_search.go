package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// vertexSearchProvider queries Vertex AI Search (Discovery Engine). The API
// does NOT support API keys: every request needs a short-lived OAuth2 access
// token minted from a service-account JSON (JWT-bearer grant, RS256, stdlib —
// see tokensource.go). On 401 the cached token is invalidated once and
// refreshed.
type vertexSearchProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p vertexSearchProvider) Code() string { return "vertex_search" }

// Schema describes the provider configuration UI.
func (p vertexSearchProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "vertex_search",
		Name:        "Vertex AI Search",
		Credentials: []CredentialField{{Key: "service_account_json", Multiline: true}},
		Params: []ParamField{
			requiredTextParam("project_id", "Project ID", "", "GCP project hosting the engine"),
			requiredTextParam("engine_id", "Engine ID", "", "Discovery Engine engine id"),
			textParam("location", "Location", "global", ""),
			textParam("collection", "Collection", "default_collection", ""),
			textParam("serving_config", "Serving config", "default_search", ""),
			selectParam("queryExpansionSpec.condition", "Query expansion", "AUTO", ParamOption{"Auto", "AUTO"}, ParamOption{"Disabled", "DISABLED"}),
			textParam("max_rps", "Max requests/sec", "", "Per-driver rate limit override"),
		},
		Hints: []string{
			"Vertex AI Search does not support API keys: put the full service-account JSON into credentials.",
			"The access token is cached for its lifetime (~1h) and refreshed on 401.",
		},
	}
}

func (p vertexSearchProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	project := strings.TrimSpace(params["project_id"])
	engineID := strings.TrimSpace(params["engine_id"])
	if project == "" || engineID == "" {
		return fail(KindAPI, true, p.Code(), "vertex_search: project_id and engine_id params are required")
	}
	ts, err := vertexTokenSource(c["service_account_json"])
	if err != nil {
		return fail(KindAPI, true, p.Code(), "vertex_search: "+err.Error())
	}

	location := defaultStr(params["location"], "global")
	collection := defaultStr(params["collection"], "default_collection")
	serving := defaultStr(params["serving_config"], "default_search")
	base := strings.TrimRight(defaultStr(params["base_url"], "https://discoveryengine.googleapis.com"), "/")
	endpoint := fmt.Sprintf("%s/v1/projects/%s/locations/%s/collections/%s/engines/%s/servingConfigs/%s:search",
		base, url.PathEscape(project), url.PathEscape(location), url.PathEscape(collection),
		url.PathEscape(engineID), url.PathEscape(serving))

	payload, _ := json.Marshal(vertexSearchBody(q, params))

	// doCall performs one upstream attempt with the given token.
	doCall := func(token string) (int, []byte, error) {
		if err := WaitLimit(ctx, p.Code(), maxRPSParam(params, 0)); err != nil {
			return 0, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := p.http.Do(req)
		if err != nil {
			return 0, nil, err
		}
		body, _, readErr := p.http.ReadBody(resp)
		return resp.StatusCode, body, readErr
	}

	token, err := ts.Token(ctx)
	if err != nil {
		return fail(KindAPI, true, p.Code(), "vertex_search: token: "+err.Error())
	}
	status, body, callErr := doCall(token)
	if status == http.StatusUnauthorized {
		// Cached token rejected early: drop it and re-authenticate exactly once.
		ts.Invalidate()
		if token, err = ts.Token(ctx); err != nil {
			return fail(KindAPI, true, p.Code(), "vertex_search: token refresh: "+err.Error())
		}
		status, body, callErr = doCall(token)
	}
	if callErr != nil {
		kind := ClassifyTransport(callErr)
		return Result{Kind: kind, Provider: p.Code(), HTTPStatus: status,
			Error: "vertex_search: " + p.http.Redact(callErr.Error())}
	}

	if !IsJSONStatus(status) {
		kind, permanent := HTTPClassify(status, parseIntCSV(params["retry_http_codes"], commonRetry()))
		if containsInt(parseIntCSV(params["fatal_http"], []int{400, 401, 403, 404}), status) {
			permanent = true
		}
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: status,
			Error: "vertex_search: " + p.http.HTTPError(status, "", body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: status,
			Error: "vertex_search: " + p.http.HTTPError(status, "body is not a json object", body)}
	}
	rows := extractVertexRows(data)
	if len(rows) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: status}
	}
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: status}
}

// vertexSearchBody builds the Discovery Engine :search request payload.
func vertexSearchBody(q Query, params Params) map[string]any {
	body := map[string]any{
		"query":             q.Text,
		"contentSearchSpec": map[string]any{"snippetSpec": map[string]any{"returnSnippet": true}},
		"safeSearch":        false,
	}
	if q.Count > 0 {
		body["pageSize"] = q.Count
	}
	if v := params["queryExpansionSpec.condition"]; v != "" {
		body["queryExpansionSpec"] = map[string]any{"condition": v}
	}
	if v := params["spellCorrectionSpec.mode"]; v != "" {
		body["spellCorrectionSpec"] = map[string]any{"mode": v}
	}
	return body
}

// extractVertexRows maps results[].document.derivedStructData to Rows.
func extractVertexRows(data map[string]any) Rows {
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
		snippet := ""
		if sarr, ok := derived["snippets"].([]any); ok && len(sarr) > 0 {
			if sm, ok := sarr[0].(map[string]any); ok {
				snippet = firstString(sm, "snippet")
			}
		}
		rows = append(rows, Row{Link: link, Title: firstString(derived, "title"), Snippet: snippet})
	}
	return rows
}
