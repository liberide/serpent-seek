package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKimiBasicSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing bearer auth")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["text_query"] != "kimi" || body["limit"] != float64(5) {
			t.Errorf("unexpected body: %+v", body)
		}
		_, _ = w.Write([]byte(`{"search_results":[
			{"url":"https://a","title":"A","snippet":"S","site_name":"a.com"},
			{"url":"https://b","title":"B","snippet":"","text":"full B text"}
		]}`))
	}))
	defer server.Close()

	p := kimiProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "kimi", Count: 5}, Credentials{"api_key": "secret"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Rows[0].Link != "https://a" || res.Rows[0].Snippet != "S" {
		t.Fatalf("row 0 mismatch: %+v", res.Rows[0])
	}
	// Without include_content, the full text is ignored and an empty snippet stays empty.
	if res.Rows[1].Snippet != "" {
		t.Fatalf("row 1 snippet should be empty without include_content: %+v", res.Rows[1])
	}
}

func TestKimiBasicIncludeContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["include_content"] != true {
			t.Errorf("include_content must be sent when enabled: %+v", body)
		}
		_, _ = w.Write([]byte(`{"search_results":[{"url":"https://a","title":"A","snippet":"S","text":"full content"}]}`))
	}))
	defer server.Close()

	p := kimiProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "include_content": "true"})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "full content" {
		t.Fatalf("full content should become the snippet: %+v", res)
	}
}

func TestKimiProChunksAndFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/search_pro" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sites, _ := body["sites"].([]any)
		if len(sites) != 2 || sites[0] != "moonshot.ai" {
			t.Errorf("unexpected sites: %+v", body["sites"])
		}
		tw, ok := body["time_window"].(map[string]any)
		if !ok || tw["start"] != "2026-01" || tw["end"] != "2026-09" {
			t.Errorf("unexpected time_window: %+v", body["time_window"])
		}
		_, _ = w.Write([]byte(`{"search_results":[{
			"url":"https://a","title":"A","snippet":"S",
			"chunks":[{"text":"passage one","score":1.2},{"text":"passage two","score":0.9}]
		}]}`))
	}))
	defer server.Close()

	p := kimiProProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "sites": "moonshot.ai, kimi.com", "time_window_start": "2026-01", "time_window_end": "2026-09"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Rows[0].Snippet != "passage one … passage two" {
		t.Fatalf("chunks should be joined into the snippet: %+v", res.Rows[0])
	}
}

func TestKimiProFallsBackToSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"search_results":[{"url":"https://a","title":"A","snippet":"plain","chunks":[]}]}`))
	}))
	defer server.Close()
	p := kimiProProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || res.Rows[0].Snippet != "plain" {
		t.Fatalf("empty chunks should fall back to snippet: %+v", res)
	}
}

func TestKimiEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"search_results":[]}`))
	}))
	defer server.Close()
	p := kimiProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || res.Kind != KindEmpty || len(res.Rows) != 0 {
		t.Fatalf("expected empty result: %+v", res)
	}
}

func TestKimiAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	p := kimiProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "bad"}, Params{"base_url": server.URL})
	if res.OK || res.HTTPStatus != http.StatusUnauthorized || !res.Permanent {
		t.Fatalf("401 must be a permanent failure: %+v", res)
	}
}

func TestKimiCountCapAndSitesCap(t *testing.T) {
	if kimiCount(50) != 20 {
		t.Fatalf("count must be capped at 20")
	}
	if kimiCount(3) != 3 {
		t.Fatalf("count must pass through under the cap")
	}
	if got := kimiSites("a, b, c, d, e, f"); len(got) != 5 {
		t.Fatalf("sites must be capped at 5, got %d", len(got))
	}
	if kimiTimeWindow("", "") != nil {
		t.Fatalf("empty time window bounds must yield nil")
	}
}
