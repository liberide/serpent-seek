package providers

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// googleCSEProvider implements the legacy Google Custom Search JSON API. The
// API is closed to new customers and shuts down on 2027-01-01; it is kept for
// existing keys, while google_vertex is the default path.
// Reference: https://developers.google.com/custom-search/v1/overview
type googleCSEProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p googleCSEProvider) Code() string { return "google_cse" }

// Schema describes the provider configuration UI.
func (p googleCSEProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "google_cse",
		Name:        "Google Custom Search (legacy)",
		Deprecated:  true,
		Credentials: []CredentialField{{Key: "api_key"}, {Key: "cx"}},
		Params: []ParamField{
			{Key: "fatal_http", Label: "Fatal HTTP codes", Type: ParamTypeText, Default: "400,401,403"},
			{Key: "retry_http_codes", Label: "Retry HTTP codes", Type: ParamTypeText, Default: "429,500,502,503,504"},
		},
		Hints: []string{
			"Legacy API closed to new customers, shuts down 2027-01-01.",
			"Hard limit 10 results per page; pagination is not implemented in this driver.",
		},
	}
}

func (p googleCSEProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := defaultStr(params["base_url"], "https://www.googleapis.com/customsearch/v1")
	key := c["api_key"]
	cx := c["cx"]
	if cx == "" {
		cx = params["cx"]
	}
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400, 401, 403})
	retryHTTP := parseIntCSV(params["retry_http_codes"], []int{429, 500, 502, 503, 504})

	num := q.Count
	if num > 10 {
		num = 10 // CSE upstream hard limit
	}
	values := url.Values{}
	values.Set("key", key)
	values.Set("cx", cx)
	values.Set("q", q.Text)
	if num > 0 {
		values.Set("num", strconv.Itoa(num))
	}
	// num=0: the parameter is omitted, CSE answers with its default page
	if q.Lang != "" {
		values.Set("hl", q.Lang)
	}
	if q.Country != "" {
		values.Set("gl", q.Country)
	}
	endpoint := base + "?" + values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fail(KindNet, false, p.Code(), "google_cse: build request: "+err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(), Error: "google_cse: " + p.http.Redact(err.Error())}
	}
	body, _, readErr := p.http.ReadBody(resp)
	if readErr != nil {
		return Result{Kind: KindRead, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_cse: body read failed: " + p.http.Redact(readErr.Error())}
	}
	if !IsJSONStatus(resp.StatusCode) {
		kind, _ := HTTPClassify(resp.StatusCode, retryHTTP)
		return Result{Kind: kind, Permanent: containsInt(fatalHTTP, resp.StatusCode), Provider: p.Code(),
			HTTPStatus: resp.StatusCode, Error: "google_cse: " + p.http.HTTPError(resp.StatusCode, resp.Status, body)}
	}
	data, ok := parseJSONObject(body)
	if !ok {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_cse: " + p.http.HTTPError(resp.StatusCode, "body is not a json object", body)}
	}
	if errObj, ok := data["error"].(map[string]any); ok {
		return Result{Kind: KindAPI, Permanent: true, Provider: p.Code(), HTTPStatus: resp.StatusCode,
			Error: "google_cse: error=" + p.http.Redact(asString(errObj["message"]))}
	}
	rows := collectRows(data["items"], []string{"link"}, "title", []string{"snippet"})
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
}
