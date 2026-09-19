package providers

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// apiserpentProvider queries the apiserpent.com quick search API.
// Upstream contract (v11): GET {base}/api/search/quick?q=&num=&country=&engine=
// with the X-API-Key header. The response carries results.organic[] (url) or
// results as a plain array; success=false signals a provider-level error.
// Each provider instance is pinned to a single `engine` (default google);
// different engines are created as separate instances.
type apiserpentProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p apiserpentProvider) Code() string { return "apiserpent" }

// Schema describes the provider configuration UI.
func (p apiserpentProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:           "apiserpent",
		Name:           "ApiSerpent",
		DefaultBaseURL: "https://apiserpent.com",
		Credentials:    []CredentialField{{Key: "api_key"}},
		Params: []ParamField{
			{Key: "engine", Label: "Engine", Type: ParamTypeSelect, Default: "google", Options: []ParamOption{{"Google", "google"}, {"Bing", "bing"}, {"Yahoo", "yahoo"}, {"DuckDuckGo", "ddg"}, {"Brave", "brave"}}},
			{Key: "country", Label: "Country", Type: ParamTypeText, Default: "us"},
			{Key: "ap_retry_codes", Label: "Retry error codes", Type: ParamTypeText, Hint: "Temporary API error codes; empty = retry any"},
		},
		Hints: []string{
			"Create a separate provider instance for each engine.",
			"num/country/language are taken from the request when available.",
		},
	}
}

func (p apiserpentProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://apiserpent.com"), "/")
	key := c["api_key"]
	country := defaultStr(params["country"], defaultStr(q.Country, "us"))
	engine := defaultStr(params["engine"], "google")

	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})
	apRetry := parseUpperCSV(params["ap_retry_codes"])

	endpoint, err := url.Parse(base + "/api/search/quick")
	if err != nil {
		return fail(KindAPI, true, p.Code(), "apiserpent: invalid base url: "+err.Error())
	}
	values := endpoint.Query()
	values.Set("q", q.Text)
	if q.Count > 0 {
		values.Set("num", strconv.Itoa(q.Count)) // num=0/unset: upstream returns everything it found
	}
	values.Set("country", country)
	values.Set("engine", engine)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fail(KindNet, false, p.Code(), "apiserpent: build request: "+err.Error())
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		kind := ClassifyTransport(err)
		return Result{Kind: kind, Provider: p.Code(), Engine: engine,
			Error: "apiserpent: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), Engine: engine, HTTPStatus: resp.StatusCode,
			Error: "apiserpent: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, permanent := HTTPClassify(resp.StatusCode, retryHTTP)
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), Engine: engine,
			HTTPStatus: resp.StatusCode, Error: "apiserpent: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), Engine: engine, HTTPStatus: resp.StatusCode,
			Error: "apiserpent: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	if !asBool(data["success"], true) {
		code := strings.ToUpper(asString(data["code"]))
		permanent := len(apRetry) > 0 && !containsStr(apRetry, code)
		msg := "apiserpent: success=false code=" + code + " message=" + p.http.Redact(asString(data["message"]))
		return Result{Kind: KindAPI, Permanent: permanent, Provider: p.Code(), Engine: engine,
			HTTPStatus: resp.StatusCode, Error: msg}
	}
	rows := extractAPISerpentRows(data)
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), Engine: engine, HTTPStatus: resp.StatusCode}
}

// extractAPISerpentRows accepts both results.organic[] and a top-level array.
func extractAPISerpentRows(data map[string]any) Rows {
	results := data["results"]
	switch t := results.(type) {
	case map[string]any:
		return collectRows(t["organic"], []string{"url", "link"}, "title", []string{"snippet", "description", "content"})
	case []any:
		return collectRows(t, []string{"url", "link"}, "title", []string{"snippet", "description", "content"})
	default:
		// Some deployments return organic[] at the top level.
		return collectRows(data["organic"], []string{"url", "link"}, "title", []string{"snippet", "description", "content"})
	}
}

// readAllLimited is a small helper kept for provider tests.
func readAllLimited(r io.Reader, limit int64) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, limit))
	return b
}
