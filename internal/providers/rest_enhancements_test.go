package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests for the additive provider enhancements (extra parameters, modes and
// response mappings).

func TestAnthropicDomainFiltersAndWebFetch(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"content":[` +
			`{"type":"web_search_tool_result","content":[{"type":"web_search_result","url":"https://a","title":"A","snippet":"S"}]},` +
			`{"type":"text","text":"answer"}]}`))
	}))
	defer server.Close()
	p := anthropicProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"}, Params{
		"base_url": server.URL, "model": "claude-sonnet-4-5",
		"allowed_domains": "a.com,b.com", "max_fetches": "3",
	})
	if !res.OK || res.Answer != "answer" {
		t.Fatalf("unexpected anthropic result: %+v", res)
	}
	if len(res.Rows) != 1 || res.Rows[0].Snippet != "S" {
		t.Fatalf("web_search_result snippet must be mapped: %+v", res.Rows)
	}
	tools, _ := body["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected web_search + web_fetch, got %#v", body["tools"])
	}
	ws, _ := tools[0].(map[string]any)
	domains, _ := ws["allowed_domains"].([]any)
	if len(domains) != 2 || domains[0] != "a.com" {
		t.Fatalf("allowed_domains not forwarded: %#v", ws)
	}
	wf, _ := tools[1].(map[string]any)
	if wf["name"] != "web_fetch" {
		t.Fatalf("web_fetch tool missing: %#v", wf)
	}
}

func TestGitHubCodeSearchTextMatch(t *testing.T) {
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"items":[{"name":"main.go","path":"cmd/main.go","html_url":"https://github.com/o/r/blob/main/cmd/main.go","repository":{"full_name":"o/r"},"text_matches":[{"fragment":"func main() {}"}]}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "github")}
	res := p.Search(context.Background(), Query{Text: "main"}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "search_type": "code"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected github result: %+v", res)
	}
	if res.Rows[0].Snippet != "func main() {}" {
		t.Fatalf("text_matches fragment must become snippet: %+v", res.Rows[0])
	}
	if res.Rows[0].Title != "o/r/cmd/main.go" {
		t.Fatalf("unexpected title %q", res.Rows[0].Title)
	}
	if gotAccept != "application/vnd.github.text-match+json" {
		t.Fatalf("code search must request text-match, got %q", gotAccept)
	}
}

func TestSemanticScholarSortOffsetAndTLDR(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[{"title":"T","url":"https://a","tldr":{"text":"short summary"}}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "semanticscholar")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 5}, Credentials{},
		Params{"base_url": server.URL, "sort": "citationCount", "offset": "20"})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "short summary" {
		t.Fatalf("unexpected semanticscholar result: %+v", res)
	}
	if !strings.Contains(gotQuery, "sort=citationCount") || !strings.Contains(gotQuery, "offset=20") {
		t.Fatalf("sort/offset must be forwarded: %q", gotQuery)
	}
}

func TestAzureSearchModeForwarded(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "azure_search")}
	p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "service_name": "s", "index_name": "i", "searchMode": "all"})
	if body["searchMode"] != "all" {
		t.Fatalf("searchMode must be forwarded, got %#v", body)
	}
}

func TestBraveResultFilterForwarded(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "brave")}
	p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "result_filter": "news,discussions"})
	if !strings.Contains(gotQuery, "result_filter=news%2Cdiscussions") {
		t.Fatalf("result_filter must be forwarded: %q", gotQuery)
	}
}

func TestExaCategoryForwarded(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "exa")}
	p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "category": "research paper", "startPublishedDate": "2025-01-01"})
	if body["category"] != "research paper" || body["startPublishedDate"] != "2025-01-01" {
		t.Fatalf("exa category/dates must be forwarded: %#v", body)
	}
}

func TestLinkupDateFiltersForwarded(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "linkup")}
	p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "fromDate": "2025-01-01", "language": "en"})
	if body["fromDate"] != "2025-01-01" || body["language"] != "en" {
		t.Fatalf("linkup date/language must be forwarded: %#v", body)
	}
}

func TestHackerNewsFrontPageTag(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"hits":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "hn")}
	p.Search(context.Background(), Query{Text: ""}, Credentials{}, Params{"base_url": server.URL, "tags": "front_page"})
	if !strings.Contains(gotQuery, "tags=front_page") {
		t.Fatalf("front_page tag must be forwarded: %q", gotQuery)
	}
}

func TestStackExchangeDateFiltersForwarded(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "stackexchange")}
	p.Search(context.Background(), Query{Text: "x"}, Credentials{},
		Params{"base_url": server.URL, "fromdate": "1700000000", "todate": "1800000000"})
	if !strings.Contains(gotQuery, "fromdate=1700000000") || !strings.Contains(gotQuery, "todate=1800000000") {
		t.Fatalf("stackexchange date filters must be forwarded: %q", gotQuery)
	}
}

func TestMojeekDescSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"status":"OK","results":[{"title":"T","url":"https://a","desc":"D"}]}}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "mojeek")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "D" {
		t.Fatalf("mojeek desc must map to snippet: %+v", res)
	}
}

func TestKagiSearchAPIMode(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("q")
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"title":"T","url":"https://a","snippet":"S"}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "kagi")}
	res := p.Search(context.Background(), Query{Text: "hello", Count: 3}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "mode": "search"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected kagi search result: %+v", res)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/search" || gotQuery != "hello" {
		t.Fatalf("kagi search must be GET /api/v1/search?q=..., got %s %s q=%q", gotMethod, gotPath, gotQuery)
	}
	if gotAuth != "Bot k" {
		t.Fatalf("kagi search must use Bot auth, got %q", gotAuth)
	}
}

func TestAnthropicWebFetchBetaHeader(t *testing.T) {
	var gotBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.Header.Get("anthropic-beta")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"a"}]}`))
	}))
	defer server.Close()
	p := anthropicProvider{http: testClient()}
	p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "model": "m", "max_fetches": "2"})
	if gotBeta != "web-fetch-2025-09-10" {
		t.Fatalf("web_fetch beta header missing, got %q", gotBeta)
	}
}

func TestKimiRegionPresets(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{"default", Params{}, "https://api.moonshot.ai"},
		{"china", Params{"region": "cn"}, "https://api.moonshot.cn"},
		{"international", Params{"region": "intl"}, "https://api.moonshot.ai"},
		{"base_url override", Params{"base_url": "http://proxy"}, "http://proxy"},
	}
	for _, tc := range cases {
		if got := kimiBaseURL(tc.params); got != tc.want {
			t.Fatalf("%s: kimiBaseURL = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDataForSEODepthCappedAt200(t *testing.T) {
	var tasks []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&tasks)
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "dataforseo")}
	p.Search(context.Background(), Query{Text: "x", Count: 500}, Credentials{"login": "l", "password": "p"},
		Params{"base_url": server.URL})
	if len(tasks) != 1 || tasks[0]["depth"] != float64(200) {
		t.Fatalf("live SERP depth must be capped at 200, got %#v", tasks)
	}
}

func TestAnthropicDomainMutualExclusion(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"a"}]}`))
	}))
	defer server.Close()
	p := anthropicProvider{http: testClient()}
	p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"}, Params{
		"base_url": server.URL, "model": "m",
		"allowed_domains": "a.com", "blocked_domains": "b.com",
	})
	tools, _ := body["tools"].([]any)
	ws, _ := tools[0].(map[string]any)
	if _, ok := ws["allowed_domains"]; !ok {
		t.Fatalf("allowed_domains must win: %#v", ws)
	}
	if _, ok := ws["blocked_domains"]; ok {
		t.Fatalf("blocked_domains must be ignored when allowed is set: %#v", ws)
	}
}

func TestOllamaWebSearch(t *testing.T) {
	var gotPath, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"results":[{"title":"T","url":"https://a","content":"C"}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "ollama")}
	res := p.Search(context.Background(), Query{Text: "what is ollama?", Count: 50}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "C" {
		t.Fatalf("unexpected ollama result: %+v", res)
	}
	if gotPath != "/api/web_search" {
		t.Fatalf("ollama path = %q", gotPath)
	}
	if gotAuth != "Bearer k" {
		t.Fatalf("ollama auth = %q", gotAuth)
	}
	if body["query"] != "what is ollama?" || body["max_results"] != float64(10) {
		t.Fatalf("ollama body must cap max_results at 10: %#v", body)
	}
}
