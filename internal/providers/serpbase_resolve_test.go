package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSerpBaseResolvesGoogleLinks(t *testing.T) {
	const (
		gotoA = "https://www.google.com/goto?url=CAESiwEBxCJF19ebeHqdTTteFvpR1tOSVCE7W1kyoUCGxyrOfRTg2rlfoSJJgFjpzo-FbpbYqwZno44lDkP6Q56sZZLqR8sRHlG2v3I4fsN1LGmsdKw1tjZ3CKukibfoekLWdznqqAvYA4fvMShmxN76nplBkM5mnc9LlyfeeN40-wIihNuYQsJ0ndBa_7yV"
		urlC  = "https://www.google.com/url?q=https%3A%2F%2Freal.example%2Fc"
	)
	var (
		resolveCalls int
		gotLinks     []string
		gotKey       string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/google/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"organic":[` +
			`{"link":"` + gotoA + `","url":"` + gotoA + `","source_url":"` + gotoA + `","title":"A","snippet":"S"},` +
			`{"link":"https://direct.example/b","source_url":"` + gotoA + `","title":"B","snippet":"S"},` +
			`{"link":"` + urlC + `","title":"C","snippet":"S"},` +
			`{"link":"` + gotoA + `","title":"A again","snippet":"S"}]}`))
	})
	mux.HandleFunc("/google/resolve", func(w http.ResponseWriter, r *http.Request) {
		resolveCalls++
		gotKey = r.Header.Get("X-API-Key")
		var body struct {
			Links []string `json:"links"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode resolve body: %v", err)
		}
		gotLinks = body.Links
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resolved_links": []map[string]any{
				{"link": gotoA, "url": "https://real.example/a", "reason": "resolved"},
				{"link": urlC, "url": "https://real.example/c", "reason": "resolved"},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := serpbaseProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 5}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "resolve_google_links": "true"})
	if !res.OK || len(res.Rows) != 4 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if resolveCalls != 1 {
		t.Fatalf("expected one resolve call, got %d", resolveCalls)
	}
	if gotKey != "k" {
		t.Fatalf("resolve must carry the api key, got %q", gotKey)
	}
	wantLinks := []string{gotoA, urlC}
	if len(gotLinks) != len(wantLinks) || gotLinks[0] != wantLinks[0] || gotLinks[1] != wantLinks[1] {
		t.Fatalf("only unique Google wrappers must be sent, got %#v", gotLinks)
	}
	if res.Rows[0].Link != "https://real.example/a" || res.Rows[2].Link != "https://real.example/c" {
		t.Fatalf("wrapper links were not resolved: %+v", res.Rows)
	}
	if res.Rows[1].Link != "https://direct.example/b" {
		t.Fatalf("direct link must stay untouched: %+v", res.Rows[1])
	}
	if res.Rows[3].Link != "https://real.example/a" {
		t.Fatalf("duplicate wrapper must be resolved too: %+v", res.Rows[3])
	}
}

func TestSerpBaseResolveGoogleLinksDisabled(t *testing.T) {
	const wrapper = "https://www.google.com/goto?url=https%3A%2F%2Freal.example%2Fa"
	resolveCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/google/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"organic":[{"link":"` + wrapper + `","title":"A"}]}`))
	})
	mux.HandleFunc("/google/resolve", func(w http.ResponseWriter, r *http.Request) {
		resolveCalls++
		_, _ = w.Write([]byte(`{"resolved_links":[]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := serpbaseProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL, "resolve_google_links": "false"})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Link != wrapper {
		t.Fatalf("unexpected result: %+v", res)
	}
	if resolveCalls != 0 {
		t.Fatalf("resolver must be skipped when disabled, got %d calls", resolveCalls)
	}
}

func TestSerpBaseResolveGoogleLinksBestEffort(t *testing.T) {
	const wrapper = "https://www.google.com/goto?url=https%3A%2F%2Freal.example%2Fa"
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "http error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("<html>nope</html>"))
			},
		},
		{
			name: "unresolved reason",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"resolved_links": []map[string]any{
						{"link": wrapper, "url": "https://real.example/a", "reason": "challenge"},
					},
				})
			},
		},
		{
			name: "missing url",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"resolved_links": []map[string]any{
						{"link": wrapper, "reason": "timeout"},
					},
				})
			},
		},
		{
			name: "unsafe url scheme",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"resolved_links": []map[string]any{
						{"link": wrapper, "url": "javascript:alert(1)", "reason": "resolved"},
					},
				})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolveCalls := 0
			mux := http.NewServeMux()
			mux.HandleFunc("/google/search", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"status":0,"organic":[{"link":"` + wrapper + `","title":"A"}]}`))
			})
			mux.HandleFunc("/google/resolve", func(w http.ResponseWriter, r *http.Request) {
				resolveCalls++
				tc.handler(w, r)
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			p := serpbaseProvider{http: testClient()}
			res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
				Params{"base_url": server.URL})
			if !res.OK || len(res.Rows) != 1 || res.Rows[0].Link != wrapper {
				t.Fatalf("resolution failure must keep the original row: %+v", res)
			}
			if resolveCalls != 1 {
				t.Fatalf("expected one resolve attempt, got %d", resolveCalls)
			}
		})
	}
}

func TestSerpBaseResolvesURLFieldWhenLinkMissing(t *testing.T) {
	const wrapper = "https://www.google.com/url?q=https%3A%2F%2Freal.example%2Ffallback"
	var gotLinks []string
	mux := http.NewServeMux()
	mux.HandleFunc("/google/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"organic":[{"url":"` + wrapper + `","title":"A"}]}`))
	})
	mux.HandleFunc("/google/resolve", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Links []string `json:"links"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotLinks = body.Links
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resolved_links": []map[string]any{
				{"link": wrapper, "url": "https://real.example/fallback", "reason": "resolved"},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := serpbaseProvider{http: testClient()}
	res := p.Search(context.Background(), Query{Text: "x", Count: 1}, Credentials{"api_key": "k"},
		Params{"base_url": server.URL})
	if !res.OK || len(res.Rows) != 1 || res.Rows[0].Link != "https://real.example/fallback" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(gotLinks) != 1 || gotLinks[0] != wrapper {
		t.Fatalf("url field must be resolved when link is absent, got %#v", gotLinks)
	}
}

func TestGoogleWrapperLinks(t *testing.T) {
	tests := []struct {
		link string
		want bool
	}{
		{"https://www.google.com/goto?url=https%3A%2F%2Fwww.example.com%2F", true},
		{"https://www.google.com/goto?url=CAESiwEBxCJF19ebeHqdTTteFvpR1tOSVCE7W1kyoUCGxyrOfRTg2rlfoSJJgFjpzo-FbpbYqwZno44lDkP6Q56sZZLqR8sRHlG2v3I4fsN1LGmsdKw1tjZ3CKukibfoekLWdznqqAvYA4fvMShmxN76nplBkM5mnc9LlyfeeN40-wIihNuYQsJ0ndBa_7yV", true},
		{"https://google.com/url?q=https%3A%2F%2Fexample.com", true},
		{"https://www.google.co.uk/url?q=x", true},
		{"https://www.google.com.au/url?q=x", true},
		{"https://www.google.de/url?q=x", true},
		{"https://google.co/url?q=x", true},
		{"https://www.google.cat/url?q=x", true},
		{"https://www.google.com/url/", true},
		{"HTTPS://WWW.GOOGLE.COM/GOTO?url=x", true},
		{"https://www.google.com/search?q=x", false},
		{"https://www.google.evil.example/goto?url=x", false},
		{"https://google.evil.com/url?q=x", false},
		{"https://example.com/url?q=x", false},
		{"ftp://www.google.com/url?q=x", false},
		{"https://www.google.com/", false},
		{"not a url", false},
	}
	for _, tc := range tests {
		if got := isGoogleResultWrapper(tc.link); got != tc.want {
			t.Errorf("isGoogleResultWrapper(%q) = %v, want %v", tc.link, got, tc.want)
		}
	}
}
