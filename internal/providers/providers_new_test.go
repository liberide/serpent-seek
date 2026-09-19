package providers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// --- Yandex /v2/web/search (XML in rawData) ---

func yandexServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

func TestYandexXMLError15IsEmpty(t *testing.T) {
	x := `<yandexsearch version="1.0"><response><error code="15">Nothing found</error></response></yandexsearch>`
	server := yandexServer(t, `{"rawData":"`+base64.StdEncoding.EncodeToString([]byte(x))+`"}`)
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if !res.OK || res.Kind != KindEmpty || res.Error != "" {
		t.Fatalf("XML error 15 must be a clean empty result: %+v", res)
	}
}

func TestYandexXMLErrorIsAPIPermanent(t *testing.T) {
	x := `<yandexsearch version="1.0"><response><error code="2">Bad query</error></response></yandexsearch>`
	server := yandexServer(t, `{"rawData":"`+base64.StdEncoding.EncodeToString([]byte(x))+`"}`)
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if res.Kind != KindAPI || !res.Permanent {
		t.Fatalf("XML error 2 must be permanent api: %+v", res)
	}
}

func TestYandexBadRawData(t *testing.T) {
	server := yandexServer(t, `{"rawData":"!!!not-base64!!!"}`)
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if res.Kind != KindJSON {
		t.Fatalf("broken base64 must be kind=json: %+v", res)
	}
}

func TestYandexMissingRawData(t *testing.T) {
	server := yandexServer(t, `{"somethingElse":true}`)
	defer server.Close()
	p := yandexProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if res.Kind != KindJSON {
		t.Fatalf("missing rawData must be kind=json: %+v", res)
	}
}

// --- yandex_gen ---

func TestYandexGenAnswerAndSources(t *testing.T) {
	server := yandexServer(t, `{"message":{"text":"Python 3.14 is out."},"sources":[{"url":"https://python.org","title":"Python"}]}`)
	defer server.Close()
	p := yandexGenProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "news", Count: 3}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if !res.OK || res.Answer != "Python 3.14 is out." || len(res.Rows) != 1 || res.Rows[0].Link != "https://python.org" {
		t.Fatalf("unexpected yandex_gen result: %+v", res)
	}
}

func TestYandexGenAnswerOnlyIsEmpty(t *testing.T) {
	server := yandexServer(t, `{"answer":"no links here"}`)
	defer server.Close()
	p := yandexGenProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k", "folder_id": "f"}, Params{"base_url": server.URL})
	if !res.OK || res.Kind != KindOK && res.Kind != KindEmpty {
		t.Fatalf("unexpected: %+v", res)
	}
	if res.Answer != "no links here" {
		t.Fatalf("answer text must be preserved: %+v", res)
	}
	if len(res.Rows) != 0 {
		t.Fatalf("no sources → no rows: %+v", res)
	}
}

// --- Stack Exchange backoff ---

func TestStackExchangeBackoff429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error_id":502,"error_message":"too many requests","error_name":"throttle_violation","backoff":22,"quota_remaining":100}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "stackexchange")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{}, Params{"base_url": server.URL})
	if res.Kind != KindAPI || res.Permanent {
		t.Fatalf("backoff must be a non-permanent api error: %+v", res)
	}
	if res.RetryAfterMS != 22000 {
		t.Fatalf("backoff=22 must become RetryAfterMS=22000: %+v", res)
	}
}

func TestStackExchangeBackoffOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"title":"T","link":"https://so/q/1","body":"<p>hi</p>"}],"backoff":5,"quota_remaining":90}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "stackexchange")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{}, Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("expected rows: %+v", res)
	}
	if res.RetryAfterMS != 5000 {
		t.Fatalf("backoff on success must still surface RetryAfterMS=5000: %+v", res)
	}
}

func findSpec(t *testing.T, code string) restSpec {
	t.Helper()
	for _, s := range restSpecs() {
		if s.code == code {
			return s
		}
	}
	t.Fatalf("spec %q not found", code)
	return restSpec{}
}

// --- Azure AI Search field map ---

func TestAzureFieldMapAndAutoSelect(t *testing.T) {
	var gotSelect string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotSelect, _ = body["select"].(string)
		_, _ = w.Write([]byte(`{"value":[{"Title":"Doc A","Url":"https://a","Description":"DA"}]}`))
	}))
	defer server.Close()
	p := restProvider{http: testClient(), spec: findSpec(t, "azure_search")}
	params := Params{
		"base_url": server.URL, "service_name": "svc", "index_name": "idx",
		"title_field": "Title", "url_field": "Url", "snippet_field": "Description",
	}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"}, params)
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("unexpected azure result: %+v", res)
	}
	row := res.Rows[0]
	if row.Title != "Doc A" || row.Link != "https://a" || row.Snippet != "DA" {
		t.Fatalf("field map not honoured: %+v", row)
	}
	if gotSelect != "Title,Url,Description" {
		t.Fatalf("auto-select must request the mapped fields, got %q", gotSelect)
	}
}

// --- Vertex AI Search OAuth2 ---

func testServiceAccount(t *testing.T, tokenURL string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemText := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sa, _ := json.Marshal(map[string]string{
		"client_email": "sa@test.iam.gserviceaccount.com",
		"private_key":  string(pemText),
		"token_uri":    tokenURL,
	})
	return string(sa)
}

func TestVertexSearchOAuth2(t *testing.T) {
	var tokenCalls atomic.Int32
	var searchAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("unexpected grant_type %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("assertion") == "" {
			t.Errorf("signed JWT assertion missing")
		}
		_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		searchAuth = r.Header.Get("Authorization")
		if !strings.Contains(r.URL.Path, "projects/p/locations/global/collections/default_collection/engines/e/servingConfigs/custom_cfg:search") {
			t.Errorf("unexpected vertex path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":[{"document":{"derivedStructData":{"link":"https://a","title":"A","snippets":[{"snippet":"S"}]}}}]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := vertexSearchProvider{http: testClient()}
	creds := Credentials{"service_account_json": testServiceAccount(t, server.URL+"/token")}
	params := Params{"base_url": server.URL, "project_id": "p", "engine_id": "e", "serving_config": "custom_cfg"}
	res := p.Search(context.Background(), Query{Text: "x", Count: 3}, creds, params)
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Snippet != "S" {
		t.Fatalf("unexpected vertex result: %+v", res)
	}
	if searchAuth != "Bearer tok-1" {
		t.Fatalf("search must carry the OAuth2 token, got %q", searchAuth)
	}

	// Second call must reuse the cached token.
	res = p.Search(context.Background(), Query{Text: "x", Count: 3}, creds, params)
	if !res.OK || tokenCalls.Load() != 1 {
		t.Fatalf("token must be cached across calls (token calls=%d): %+v", tokenCalls.Load(), res)
	}
}

func TestVertexSearchRefreshesOn401(t *testing.T) {
	var tokenCalls atomic.Int32
	var searchCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		n := tokenCalls.Add(1)
		tok := "tok-1"
		if n > 1 {
			tok = "tok-2"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "expires_in": 3600})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		searchCalls.Add(1)
		if r.Header.Get("Authorization") == "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"stale"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"document":{"derivedStructData":{"link":"https://b","title":"B"}}}]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := vertexSearchProvider{http: testClient()}
	creds := Credentials{"service_account_json": testServiceAccount(t, server.URL+"/token")}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, creds,
		Params{"base_url": server.URL, "project_id": "p", "engine_id": "e"})
	if !res.OK || len(res.Rows) != 1 {
		t.Fatalf("401 must trigger one token refresh and succeed: %+v", res)
	}
	if tokenCalls.Load() != 2 || searchCalls.Load() != 2 {
		t.Fatalf("expected 2 token + 2 search calls, got %d/%d", tokenCalls.Load(), searchCalls.Load())
	}
}

func TestVertexSearchRequiresSA(t *testing.T) {
	p := vertexSearchProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{},
		Params{"project_id": "p", "engine_id": "e"})
	if !res.Permanent || res.Kind != KindAPI {
		t.Fatalf("missing service_account_json must be a permanent config error: %+v", res)
	}
}

// --- PubMed two-step ---

type stageRecorder struct{ stages []string }

func (r *stageRecorder) Report(sc StageCall) { r.stages = append(r.stages, sc.Stage) }

func pubmedServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "esearch.fcgi"):
			if r.URL.Query().Get("term") == "nothing" {
				_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":["38123456","38123457"]}}`))
		case strings.HasSuffix(r.URL.Path, "esummary.fcgi"):
			_, _ = w.Write([]byte(`{"result":{"uids":["38123456","38123457"],
				"38123456":{"uid":"38123456","title":"CRISPR-Cas9 gene therapy","source":"Nat Med","pubdate":"2026 Jan"},
				"38123457":{"uid":"38123457","title":"Second study","source":"Lancet","pubdate":"2025 Dec"}}}`))
		case strings.HasSuffix(r.URL.Path, "efetch.fcgi"):
			_, _ = w.Write([]byte(`<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>38123456</PMID>
				<Article><Abstract><AbstractText Label="BACKGROUND">Retinal gene therapy works.</AbstractText>
				<AbstractText Label="METHODS">A <b>phase</b> I/II trial.</AbstractText></Abstract></Article>
				</MedlineCitation></PubmedArticle></PubmedArticleSet>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestPubMedTwoStep(t *testing.T) {
	hits := &atomic.Int32{}
	server := pubmedServer(t, hits)
	defer server.Close()
	p := pubmedProvider{http: testClient()}
	rec := &stageRecorder{}
	ctx := WithStepReporter(context.Background(), rec)
	res := p.Search(ctx, Query{Text: "crispr", Count: 2}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "max_rps": "1000"})
	if !res.OK || len(res.Rows) != 2 {
		t.Fatalf("unexpected pubmed result: %+v", res)
	}
	row := res.Rows[0]
	if row.Link != "https://pubmed.ncbi.nlm.nih.gov/38123456/" || row.Title != "CRISPR-Cas9 gene therapy" {
		t.Fatalf("row mapping failed: %+v", row)
	}
	if row.Snippet != "Nat Med · 2026 Jan" {
		t.Fatalf("snippet must be source+pubdate: %+v", row)
	}
	if got := strings.Join(rec.stages, ","); got != "esearch,esummary" {
		t.Fatalf("stage reporting broken: %v", got)
	}
}

func TestPubMedEmptyShortCircuits(t *testing.T) {
	hits := &atomic.Int32{}
	server := pubmedServer(t, hits)
	defer server.Close()
	p := pubmedProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "nothing", Count: 2}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "max_rps": "1000"})
	if !res.OK || res.Kind != KindEmpty {
		t.Fatalf("empty idlist must be empty: %+v", res)
	}
	if hits.Load() != 1 {
		t.Fatalf("esummary must not be called on empty esearch (hits=%d)", hits.Load())
	}
}

func TestPubMedFetchAbstracts(t *testing.T) {
	hits := &atomic.Int32{}
	server := pubmedServer(t, hits)
	defer server.Close()
	p := pubmedProvider{http: testClient()}
	rec := &stageRecorder{}
	ctx := WithStepReporter(context.Background(), rec)
	res := p.Search(ctx, Query{Text: "crispr", Count: 2}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "max_rps": "1000", "fetch_abstracts": "true", "snippet_max_chars": "200"})
	if !res.OK || len(res.Rows) != 2 {
		t.Fatalf("unexpected: %+v", res)
	}
	want := "BACKGROUND: Retinal gene therapy works. METHODS: A phase I/II trial."
	if res.Rows[0].Snippet != want {
		t.Fatalf("abstract snippet wrong: %q", res.Rows[0].Snippet)
	}
	if res.Rows[1].Snippet != "Lancet · 2025 Dec" {
		t.Fatalf("row without abstract keeps meta snippet: %+v", res.Rows[1])
	}
	if got := strings.Join(rec.stages, ","); got != "esearch,esummary,efetch" {
		t.Fatalf("stage reporting broken: %v", got)
	}
}

// --- limiter ---

func TestMaxRPSParam(t *testing.T) {
	if got := maxRPSParam(Params{"max_rps": "5"}, 3); got != 5 {
		t.Fatalf("override loses: %v", got)
	}
	if got := maxRPSParam(Params{"max_rps": "junk"}, 3); got != 3 {
		t.Fatalf("invalid override must fall back: %v", got)
	}
	if got := maxRPSParam(Params{}, 0); got != 0 {
		t.Fatalf("0 default must stay 0: %v", got)
	}
}
