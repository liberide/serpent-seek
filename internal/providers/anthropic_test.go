package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicAnswerAndSources(t *testing.T) {
	body := `{
		"content": [
			{"type":"text","text":"Go 1.26 is out.","citations":[
				{"type":"web_search_result_location","url":"https://go.dev/blog","title":"Go Blog","cited_text":"Go 1.26 released"}
			]},
			{"type":"web_search_tool_result","content":[
				{"type":"web_search_result","url":"https://go.dev/blog","title":"Go Blog"},
				{"type":"web_search_result","url":"https://example.com","title":"Example"}
			]}
		]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic auth headers")
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	p := anthropicProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "go", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL, "model": "claude-test"})
	if !res.OK || res.Answer != "Go 1.26 is out." {
		t.Fatalf("unexpected answer: %+v", res)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 deduplicated sources, got %+v", res.Rows)
	}
	if res.Rows[0].Link != "https://go.dev/blog" || res.Rows[0].Snippet != "Go 1.26 released" {
		t.Fatalf("citation must carry the link and cited_text snippet: %+v", res.Rows[0])
	}
}

func TestAnthropicMissingModel(t *testing.T) {
	p := anthropicProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{})
	if res.Kind != KindAPI || !res.Permanent {
		t.Fatalf("missing model must be a permanent api error: %+v", res)
	}
}

func TestAnthropicHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer server.Close()
	p := anthropicProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "bad"}, Params{"base_url": server.URL, "model": "claude-test"})
	if res.OK || res.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("401 must be a failed result: %+v", res)
	}
}
