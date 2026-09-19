package providers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSerpBaseNonJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>Cloudflare</html>"))
	}))
	defer server.Close()
	p := serpbaseProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if res.Kind != KindJSON {
		t.Fatalf("expected kind=json, got %+v", res)
	}
}

func TestSerpBaseUnknownStatusTemporary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":9999,"error":"mystery"}`))
	}))
	defer server.Close()
	p := serpbaseProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if res.Kind != KindAPI || res.Permanent {
		t.Fatalf("unknown status should be temporary api error: %+v", res)
	}
}

func TestYandexTemporaryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if res.Permanent {
		t.Fatalf("429 must be temporary: %+v", res)
	}
}

func TestYandexNonJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("nope"))
	}))
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if res.Kind != KindJSON {
		t.Fatalf("expected json kind, got %+v", res)
	}
}

func TestGoogleDispatcher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"link":"https://a","title":"A"}]}`))
	}))
	defer server.Close()
	dispatcher := googleDispatcher{vertex: googleProvider{http: testClient()}, cse: googleCSEProvider{http: testClient()}}
	if dispatcher.Code() != "google" {
		t.Fatalf("unexpected code %q", dispatcher.Code())
	}
	res := dispatcher.Search(context.Background(), Query{Text: "x", Count: 3},
		Credentials{"api_key": "k", "cx": "c"}, Params{"base_url": server.URL, "driver": "cse"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("dispatcher cse path failed: %+v", res)
	}
}

func TestGoogleEnterprise(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"document":{"derivedStructData":{"link":"https://a","title":"A","snippets":[{"snippet":"S"}]}}}]}`))
	}))
	defer server.Close()
	p := googleProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3},
		Credentials{"api_key": "k", "project_id": "p", "engine_id": "e"},
		Params{"base_url": server.URL, "driver_mode": "enterprise"})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "S" {
		t.Fatalf("unexpected enterprise result: %+v", res)
	}
}

func TestGoogleEnterpriseRequiresConfig(t *testing.T) {
	p := googleProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, Credentials{"api_key": "k"}, Params{"driver_mode": "enterprise"})
	if res.Kind != KindAPI || !res.Permanent {
		t.Fatalf("missing project/engine should be permanent api error: %+v", res)
	}
}

func TestClassifyTransport(t *testing.T) {
	if got := ClassifyTransport(context.DeadlineExceeded); got != KindTimeout {
		t.Fatalf("deadline exceeded should map to timeout, got %s", got)
	}
	if got := ClassifyTransport(errors.New("connection refused")); got != KindNet {
		t.Fatalf("expected net, got %s", got)
	}
	if got := ClassifyTransport(errors.New("i/o timeout")); got != KindTimeout {
		t.Fatalf("expected timeout, got %s", got)
	}
}

func TestHTTPClassify(t *testing.T) {
	if kind, permanent := HTTPClassify(http.StatusTooManyRequests, []int{429}); kind != KindHTTP || permanent {
		t.Fatalf("429 should be retryable: %s %v", kind, permanent)
	}
	if _, permanent := HTTPClassify(http.StatusBadRequest, []int{429}); !permanent {
		t.Fatal("400 should be permanent")
	}
	if _, permanent := HTTPClassify(http.StatusBadGateway, nil); permanent {
		t.Fatal("5xx should be retryable even without an explicit list")
	}
}

func TestAPISerpentTopLevelOrganic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"organic":[{"url":"https://a","title":"A"}]}`))
	}))
	defer server.Close()
	p := apiserpentProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("top-level organic not parsed: %+v", res)
	}
}

func TestSearxngLessThanTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "hello world" {
			t.Errorf("template did not inject q: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	p := searxngProvider{http: testClient()}
	params := Params{"base_url": server.URL + "/search?q=<query>"}
	if res := p.Search(context.Background(), Query{Text: "hello world", Count: 1}, nil, params); !res.OK {
		t.Fatalf("angle-bracket template failed: %+v", res)
	}
}

func TestSearxngRequiresExternalURL(t *testing.T) {
	p := searxngProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, nil, Params{})
	if res.Kind != KindAPI || !res.Permanent {
		t.Fatalf("missing external url must be a permanent configuration error: %+v", res)
	}
}

func TestTruncatedBodyIsBounded(t *testing.T) {
	// A body larger than the cap must not be read in full.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		big := make([]byte, MaxResponseBytes+1024)
		for i := range big {
			big[i] = 'a'
		}
		_, _ = w.Write(big)
	}))
	defer server.Close()
	client := testClient()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	body, truncated, err := client.ReadBody(resp)
	if err != nil {
		t.Fatalf("ReadBody: %v", err)
	}
	if !truncated || len(body) != MaxResponseBytes {
		t.Fatalf("expected truncated body of %d bytes, got %d (truncated=%v)", MaxResponseBytes, len(body), truncated)
	}
}
