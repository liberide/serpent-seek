package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These tests cover the generic REST driver fixes: Mojeek host/status, Kagi
// FastGPT body, Vectara corpus path, the Perplexity search_type param and the
// Hacker News by-date endpoint.

func TestMojeekHostAndStatus(t *testing.T) {
	if got := findSpec(t, "mojeek").baseURL; got != "https://www.mojeek.com" {
		t.Fatalf("mojeek baseURL must be https://www.mojeek.com, got %q", got)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"status":"ERROR_INVALID_KEY","results":[]}}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "mojeek")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || res.Kind != KindEmpty {
		t.Fatalf("response.status=ERROR must map to empty, got %+v", res)
	}
}

func TestKagiFastGPTQueryBody(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"references":[{"title":"T","url":"https://a","snippet":"S"}]}}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "kagi")}
	res := p.Search(context.Background(), Query{Text: "hello"}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected kagi result: %+v", res)
	}
	if gotBody["query"] != "hello" {
		t.Fatalf("kagi FastGPT must send query, got %#v", gotBody)
	}
}

func TestPerplexitySearchTypeParam(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "perplexity")}
	p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"}, Params{"base_url": server.URL, "search_type": "people"})
	if gotBody["search_type"] != "people" {
		t.Fatalf("search_type param must be honoured, got %#v", gotBody)
	}
}

func TestVectaraCorpusPathAndMetadata(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"search_results":[{"text":"body text","document_metadata":{"title":"Doc","url":"https://a"}}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "vectara")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 2}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "corpus_key": "c1"})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Link != "https://a" || res.Rows[0].Title != "Doc" {
		t.Fatalf("unexpected vectara result: %+v", res)
	}
	if gotPath != "/v2/corpora/c1/query" {
		t.Fatalf("wrong vectara path %q", gotPath)
	}
	search, _ := gotBody["search"].(map[string]any)
	if search["limit"] != float64(2) {
		t.Fatalf("limit missing from vectara body: %#v", gotBody)
	}
}

func TestHackerNewsByDate(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Query().Get("search_type") != "" {
			t.Errorf("search_type must not reach the upstream, raw query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"hits":[{"objectID":"1","title":"T","url":"https://a"}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "hn")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{},
		Params{"base_url": server.URL, "search_type": "date"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected hn result: %+v", res)
	}
	if gotPath != "/api/v1/search_by_date" {
		t.Fatalf("date sort must switch endpoint, got %q", gotPath)
	}
}

func TestAPISerpentDeepMode(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success":true,"results":{"organic":[{"title":"T","url":"https://a","snippet":"S"}]}}`))
	}))
	defer server.Close()
	p := apiserpentProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 5}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "mode": "deep"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected apiserpent result: %+v", res)
	}
	if gotPath != "/api/search" {
		t.Fatalf("deep mode must use /api/search, got %q", gotPath)
	}
}

func TestGoogleCSEStartParam(t *testing.T) {
	var gotStart string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStart = r.URL.Query().Get("start")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	p := googleCSEProvider{http: testClient()}
	p.Search(context.Background(), Query{Text: "x", Count: 5}, Credentials{"api_key": "k", "cx": "c"},
		Params{"base_url": server.URL, "start": "11"})
	if gotStart != "11" {
		t.Fatalf("cse start must be forwarded, got %q", gotStart)
	}
}
