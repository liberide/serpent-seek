package providers

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// searxngProvider queries an externally hosted SearXNG instance. The address is
// always a plain URL (http://host:port/search) or a template containing
// {query}/<query>, matching the SEARXNG_QUERY_URL convention used by Open WebUI.
// SerpentSeek does not ship a SearXNG container: the instance is external and its
// URL is edited in the UI. The optional X-API-Key is a gate/proxy token, NOT
// SearXNG's server.secret_key.
type searxngProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p searxngProvider) Code() string { return "searxng" }

// Schema describes the provider configuration UI.
func (p searxngProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "searxng",
		Name:        "SearXNG",
		Credentials: []CredentialField{{Key: "api_key"}},
		Params: []ParamField{
			{Key: "language", Label: "Language", Type: ParamTypeText, Hint: "Optional interface language code"},
		},
		Hints: []string{
			"The optional X-API-Key is a gate/proxy token, not SearXNG's server.secret_key.",
			"Instance must enable JSON format: search.formats: [html, json].",
		},
	}
}

func (p searxngProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimSpace(params["base_url"])
	if base == "" {
		return fail(KindAPI, true, p.Code(), "searxng: external URL is not configured (set base_url or SEARXNG_URL)")
	}
	key := strings.TrimSpace(c["api_key"])
	if key == "" {
		key = strings.TrimSpace(params["api_key"])
	}
	lang := defaultStr(params["language"], q.Lang)

	endpoint := buildSearxngURL(base, q.Text, lang)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fail(KindNet, false, p.Code(), "searxng: build request: "+err.Error())
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Serpent-Source", "serpentseek")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}

	resp, err := p.http.Do(req)
	if err != nil {
		kind := ClassifyTransport(err)
		return Result{Kind: kind, Provider: p.Code(), Error: "searxng: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "searxng: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		msg := "searxng: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			if key != "" {
				msg += " -> SearXNG gate rejected X-API-Key (check SEARXNG_API_KEY)"
			} else {
				msg += " -> search requires auth: set SEARXNG_API_KEY or disable auth on SearXNG"
			}
		case http.StatusForbidden:
			if key != "" {
				msg += " -> gate rejected X-API-Key, verify SEARXNG_API_KEY"
			} else {
				msg += " -> enable json in SearXNG settings.yml (search.formats: [html, json])"
			}
		case http.StatusTooManyRequests:
			msg += " -> SearXNG limiter (botdetection) is enabled for internal requests (set limiter: false)"
		}
		// SearXNG never causes a permanent provider death: the chain may retry.
		return Result{Kind: KindHTTP, Permanent: false, Provider: p.Code(), HTTPStatus: resp.StatusCode, Error: msg}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "searxng: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	results, hasResults := data["results"].([]any)
	if !hasResults && asString(data["error"]) != "" {
		return Result{Kind: KindAPI, Permanent: false, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "searxng: error=" + p.http.Redact(asString(data["error"]))}
	}
	rows := collectRows(results, []string{"url", "link"}, "title", []string{"content", "snippet"})
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}

// buildSearxngURL expands the configured address into a JSON search URL.
func buildSearxngURL(base, query, lang string) string {
	quoted := url.QueryEscape(query)
	expanded := false
	for _, ph := range []string{"{query}", "<query>"} {
		if strings.Contains(base, ph) {
			base = strings.ReplaceAll(base, ph, quoted)
			expanded = true
			break
		}
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	values := u.Query()
	if !expanded {
		values.Set("q", query)
	}
	if values.Get("format") == "" {
		values.Set("format", "json")
	}
	if lang != "" && values.Get("language") == "" {
		values.Set("language", lang)
	}
	u.RawQuery = values.Encode()
	return u.String()
}
