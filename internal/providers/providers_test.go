package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient() *HTTPClient {
	client := NewHTTPClient("test-agent")
	client.SetRedactor(func(s string) string { return s })
	return client
}

func TestAPISerpentSuccessAndEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search/quick" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "secret" {
			t.Errorf("missing api key header")
		}
		switch r.URL.Query().Get("q") {
		case "empty":
			_, _ = w.Write([]byte(`{"success":true,"results":{"organic":[]}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"results":{"organic":[{"url":"https://a","title":"A","snippet":"S"}]}}`))
		}
	}))
	defer server.Close()

	p := apiserpentProvider{http: testClient()}
	params := Params{"base_url": server.URL}
	res := p.Search(context.Background(), Query{Text: "hello", Count: 5}, Credentials{"api_key": "secret"}, params)
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Link != "https://a" {
		t.Fatalf("unexpected result: %+v", res)
	}
	empty := p.Search(context.Background(), Query{Text: "empty", Count: 5}, Credentials{"api_key": "secret"}, params)
	if !empty.OK || len(empty.Rows) != 0 {
		t.Fatalf("expected empty result, got %+v", empty)
	}
}

func TestAPISerpentAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"code":"ENGINE_BUSY","message":"busy"}`))
	}))
	defer server.Close()
	p := apiserpentProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "ap_retry_codes": "ENGINE_BUSY"})
	if res.Kind != KindAPI || res.Permanent {
		t.Fatalf("ENGINE_BUSY should be a retryable api error: %+v", res)
	}
	// Without a retry list any api error is retryable.
	res = p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if res.Permanent {
		t.Fatalf("empty retry list should not mark permanent: %+v", res)
	}
	// With a non-matching retry list the error is permanent.
	res = p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "ap_retry_codes": "RATE_LIMIT"})
	if !res.Permanent {
		t.Fatalf("non-listed code should be permanent: %+v", res)
	}
}

func TestAPISerpentHTTPAndJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "bad":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("nope"))
		case "ratelimit":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte("<html>not json</html>"))
		}
	}))
	defer server.Close()
	p := apiserpentProvider{http: testClient()}
	params := Params{"base_url": server.URL}

	if res := p.Search(context.Background(), Query{Text: "bad", Count: 1}, Credentials{"api_key": "k"}, params); res.Kind != KindHTTP || !res.Permanent {
		t.Fatalf("400 should be permanent http: %+v", res)
	}
	if res := p.Search(context.Background(), Query{Text: "ratelimit", Count: 1}, Credentials{"api_key": "k"}, params); res.Permanent {
		t.Fatalf("429 should be retryable: %+v", res)
	}
	if res := p.Search(context.Background(), Query{Text: "junk", Count: 1}, Credentials{"api_key": "k"}, params); res.Kind != KindJSON {
		t.Fatalf("non-json should be kind=json: %+v", res)
	}
}

func TestAPISerpentTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()
	p := apiserpentProvider{http: testClient()}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	res := p.Search(ctx, Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if res.Kind != KindTimeout {
		t.Fatalf("expected timeout, got %s (%s)", res.Kind, res.Error)
	}
}

func TestSerpBaseStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["q"] {
		case "ok":
			_, _ = w.Write([]byte(`{"status":0,"organic":[{"link":"https://a","title":"A","snippet":"S"}]}`))
		case "fatal":
			_, _ = w.Write([]byte(`{"status":1020,"error":"insufficient credits"}`))
		case "temp":
			_, _ = w.Write([]byte(`{"status":1029,"error":"rate limited"}`))
		default:
			_, _ = w.Write([]byte(`{"status":0,"organic":[]}`))
		}
	}))
	defer server.Close()
	p := serpbaseProvider{http: testClient()}
	params := Params{"base_url": server.URL}

	if res := p.Search(context.Background(), Query{Text: "ok", Count: 5}, Credentials{"api_key": "k"}, params); !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected ok result: %+v", res)
	}
	if res := p.Search(context.Background(), Query{Text: "fatal", Count: 5}, Credentials{"api_key": "k"}, params); !res.Permanent {
		t.Fatalf("1020 must be permanent: %+v", res)
	}
	if res := p.Search(context.Background(), Query{Text: "temp", Count: 5}, Credentials{"api_key": "k"}, params); res.Permanent {
		t.Fatalf("1029 must be temporary: %+v", res)
	}
}

func TestSerpBaseHTTPFatal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":1001,"error":"unauthorized"}`))
	}))
	defer server.Close()
	p := serpbaseProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.Permanent || res.Kind != KindHTTP {
		t.Fatalf("403 should be a permanent http error: %+v", res)
	}
}

func TestSearxngSuccessTemplateAndEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("format=json missing: %s", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("q") {
		case "empty":
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			_, _ = w.Write([]byte(`{"results":[{"url":"https://a","title":"A","content":"S"}]}`))
		}
	}))
	defer server.Close()
	p := searxngProvider{http: testClient()}
	params := Params{"base_url": server.URL + "/search"}
	if res := p.Search(context.Background(), Query{Text: "hello", Count: 5}, nil, params); !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected searxng result: %+v", res)
	}
	if res := p.Search(context.Background(), Query{Text: "empty", Count: 5}, nil, params); !res.OK || len(res.Rows) != 0 {
		t.Fatalf("unexpected searxng empty result: %+v", res)
	}
	template := Params{"base_url": server.URL + "/search?q={query}"}
	if res := p.Search(context.Background(), Query{Text: "hello", Count: 5}, nil, template); !res.OK {
		t.Fatalf("template url failed: %+v", res)
	}
}

func TestSearxngErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "err":
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		case "401":
			w.WriteHeader(http.StatusUnauthorized)
		case "403":
			w.WriteHeader(http.StatusForbidden)
		case "429":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte(`{"results":[]}`))
		}
	}))
	defer server.Close()
	p := searxngProvider{http: testClient()}
	params := Params{"base_url": server.URL + "/search"}
	if res := p.Search(context.Background(), Query{Text: "err", Count: 1}, nil, params); res.Kind != KindAPI {
		t.Fatalf("error body should be kind=api: %+v", res)
	}
	for _, q := range []string{"401", "403", "429"} {
		res := p.Search(context.Background(), Query{Text: q, Count: 1}, nil, params)
		if res.Kind != KindHTTP || res.Permanent {
			t.Fatalf("searxng %s must be non-permanent http: %+v", q, res)
		}
	}
}

func TestYandex(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="utf-8"?>
<yandexsearch version="1.0"><response date="20260918T101500"><reqid>1</reqid>
<results><grouping><group><doc>
<url>https://a.example</url>
<title>How to choose a <hlword>coffee machine</hlword></title>
<passages><passage>The main criterion is the <hlword>coffee machine</hlword> type</passage></passages>
</doc></group></grouping></results></response></yandexsearch>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Api-Key secret" {
			t.Errorf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v2/web/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["responseFormat"] != "FORMAT_XML" {
			t.Errorf("responseFormat must be FORMAT_XML, got %v", reqBody["responseFormat"])
		}
		if reqBody["folderId"] != "f" {
			t.Errorf("folderId missing: %v", reqBody["folderId"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"rawData": base64.StdEncoding.EncodeToString([]byte(xmlBody))})
	}))
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3},
		Credentials{"api_key": "secret", "folder_id": "f"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected yandex result: %+v", res)
	}
	row := res.Rows[0]
	if row.Link != "https://a.example" || row.Title != "How to choose a coffee machine" {
		t.Fatalf("hlword tags must be stripped: %+v", row)
	}
	if row.Snippet != "The main criterion is the coffee machine type" {
		t.Fatalf("passages must fill the snippet: %+v", row)
	}
}

func TestYandexForbiddenPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if !res.Permanent {
		t.Fatalf("403 must be permanent: %+v", res)
	}
}

func TestGoogleGrounding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "secret" {
			t.Errorf("api key missing in query")
		}
		_, _ = w.Write([]byte(`{"candidates":[{"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://a","title":"A"}}],"groundingSupports":[{"segment":{"text":"supporting"},"groundingChunkIndices":[0]}]}}]}`))
	}))
	defer server.Close()
	p := googleProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "secret"},
		Params{"base_url": server.URL, "driver_mode": "gemini"})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "supporting" {
		t.Fatalf("unexpected grounding result: %+v", res)
	}
}

func TestGoogleGroundingEmptyAndPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") == "bad" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"candidates":[{"groundingMetadata":{"groundingChunks":[]}}]}`))
	}))
	defer server.Close()
	p := googleProvider{http: testClient()}
	goodParams := Params{"base_url": server.URL, "driver_mode": "gemini"}
	if res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "good"}, goodParams); !res.OK || len(res.Rows) != 0 {
		t.Fatalf("expected empty grounding, got %+v", res)
	}
	if res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "bad"}, goodParams); !res.Permanent {
		t.Fatalf("400 must be permanent: %+v", res)
	}
}

func TestGoogleCSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "secret" || r.URL.Query().Get("cx") != "cx1" {
			t.Errorf("missing key/cx: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"items":[{"link":"https://a","title":"A","snippet":"S"}]}`))
	}))
	defer server.Close()
	p := googleCSEProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "secret", "cx": "cx1"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected cse result: %+v", res)
	}
}

func TestGoogleCSEForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"accessNotConfigured"}}`))
	}))
	defer server.Close()
	p := googleCSEProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "k", "cx": "c"}, Params{"base_url": server.URL})
	if !res.Permanent {
		t.Fatalf("403 must be permanent: %+v", res)
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry(testClient())
	for _, code := range []string{"apiserpent", "serpbase", "searxng", "yandex", "yandex_gen", "google", "google_vertex", "google_cse", "vertex_search", "pubmed", "kendra"} {
		if _, ok := reg.Get(code); !ok {
			t.Fatalf("provider %q is not registered", code)
		}
	}
	if len(reg.Codes()) < 6 {
		t.Fatalf("expected at least 6 providers, got %v", reg.Codes())
	}
	// Kendra is closed to new customers (2026-07-30) and must stay deprecated.
	if k, ok := reg.Get("kendra"); ok && !k.Schema().Deprecated {
		t.Fatalf("kendra must be marked deprecated")
	}
}

func TestBuildSearxngURL(t *testing.T) {
	got := buildSearxngURL("http://host:8080/search", "hello world", "ru")
	if !contains(got, "format=json") || !contains(got, "q=hello+world") || !contains(got, "language=ru") {
		t.Fatalf("unexpected url %s", got)
	}
	template := buildSearxngURL("http://host/search?q={query}&format=json", "a b", "")
	if contains(template, "{query}") || !contains(template, "q=a+b") {
		t.Fatalf("template not expanded: %s", template)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
