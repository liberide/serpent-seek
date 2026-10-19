package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeepSeekSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing bearer auth")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["query"] != "deepseek" || body["count"] != float64(50) {
			t.Errorf("unexpected body: %+v", body)
		}
		if body["ori_question"] != "what is new?" || body["localization"] != "203.0.113.7" {
			t.Errorf("optional fields missing: %+v", body)
		}
		if body["need_content"] != true {
			t.Errorf("need_content must be sent as a bool: %+v", body)
		}
		_, _ = w.Write([]byte(`{"results":[
			{"url":"https://a","title":"A","snippet":"short","summary":"long summary","content":"full page","site_name":"a.com"},
			{"url":"https://b","title":"B","summary":"only summary"},
			{"title":"no url"}
		]}`))
	}))
	defer server.Close()

	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "deepseek", Count: 100}, Credentials{"api_key": "secret"},
		Params{"base_url": server.URL, "need_content": "true", "ori_question": "what is new?", "localization": "203.0.113.7"})
	if !res.OK || len(res.Rows) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Rows[0].Link != "https://a" || res.Rows[0].Title != "A" || res.Rows[0].Snippet != "full page" {
		t.Fatalf("row 0 mismatch: %+v", res.Rows[0])
	}
	if res.Rows[1].Link != "https://b" || res.Rows[1].Snippet != "only summary" {
		t.Fatalf("row 1 mismatch: %+v", res.Rows[1])
	}
}

func TestDeepSeekSummaryPreferred(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["need_content"]; ok {
			t.Errorf("need_content must be omitted when disabled: %+v", body)
		}
		if _, ok := body["count"]; ok {
			t.Errorf("count must be omitted when the query has no limit: %+v", body)
		}
		_, _ = w.Write([]byte(`{"results":[{"url":"https://a","title":"A","snippet":"short","summary":"long summary","content":"full page"}]}`))
	}))
	defer server.Close()

	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x"}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "long summary" {
		t.Fatalf("summary should be preferred without need_content: %+v", res)
	}
}

func TestDeepSeekSnippetFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"url":"https://a","title":"A","snippet":"short","summary":""}]}`))
	}))
	defer server.Close()
	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || res.Rows[0].Snippet != "short" {
		t.Fatalf("empty summary should fall back to snippet: %+v", res)
	}
}

func TestDeepSeekEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || res.Kind != KindEmpty || len(res.Rows) != 0 {
		t.Fatalf("expected empty result: %+v", res)
	}
}

func TestDeepSeekAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "bad"}, Params{"base_url": server.URL})
	if res.OK || res.HTTPStatus != http.StatusUnauthorized || !res.Permanent {
		t.Fatalf("401 must be a permanent failure: %+v", res)
	}
}

func TestDeepSeekInsufficientBalanceIsPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient balance","type":"invalid_request_error","param":null,"code":"invalid_request_error"}}`))
	}))
	defer server.Close()
	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if res.OK || res.HTTPStatus != http.StatusPaymentRequired || !res.Permanent {
		t.Fatalf("402 must be a permanent failure: %+v", res)
	}
}

func TestDeepSeekTimeoutIsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()
	p := deepseekProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if res.OK || res.Permanent || res.Kind != KindHTTP {
		t.Fatalf("408 must be a retryable http failure: %+v", res)
	}
}

func TestDeepSeekCountCap(t *testing.T) {
	if deepseekCount(100) != 50 {
		t.Fatalf("count must be capped at 50")
	}
	if deepseekCount(7) != 7 {
		t.Fatalf("count must pass through under the cap")
	}
}
