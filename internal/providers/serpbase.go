package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: resp.StatusCode}
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
