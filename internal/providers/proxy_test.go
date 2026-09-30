package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestProxyConfigURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  ProxyConfig
		want string
	}{
		{"http default", ProxyConfig{Host: "127.0.0.1", Port: "8080"}, "http://127.0.0.1:8080"},
		{"https with auth", ProxyConfig{Type: "https", Host: "proxy.local", Port: "3128", Username: "u", Password: "p"}, "https://u:p@proxy.local:3128"},
		{"socks5", ProxyConfig{Type: "socks5", Host: "1.2.3.4", Port: "1080"}, "socks5://1.2.3.4:1080"},
		{"host already has port", ProxyConfig{Host: "127.0.0.1:9000"}, "http://127.0.0.1:9000"},
		{"full url host", ProxyConfig{Host: "socks5://1.2.3.4:1080"}, "socks5://1.2.3.4:1080"},
		{"full url host with explicit port", ProxyConfig{Host: "http://p", Port: "8080"}, "http://p:8080"},
		{"username without password", ProxyConfig{Host: "h", Port: "1", Username: "u"}, "http://u@h:1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := tc.cfg.URL()
			if err != nil {
				t.Fatalf("URL: %v", err)
			}
			if u == nil {
				t.Fatal("expected non-nil URL")
			}
			if got := u.String(); got != tc.want {
				t.Fatalf("URL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProxyConfigEmptyHost(t *testing.T) {
	u, err := (&ProxyConfig{}).URL()
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if u != nil {
		t.Fatalf("expected nil URL for empty host, got %v", u)
	}
}

func TestProxyContextRoundTrip(t *testing.T) {
	if proxyFromContext(context.Background()) != nil {
		t.Fatal("expected nil proxy on a bare context")
	}
	cfg := &ProxyConfig{Host: "h", Port: "1"}
	if got := proxyFromContext(WithProxy(context.Background(), cfg)); got != cfg {
		t.Fatalf("proxyFromContext = %v, want %v", got, cfg)
	}
}

// TestHTTPClientUsesContextProxy verifies that a proxy attached to the request
// context actually reroutes the shared client: the target must never be hit.
func TestHTTPClientUsesContextProxy(t *testing.T) {
	var proxyHits int32
	var targetHits int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&proxyHits, 1)
		_, _ = w.Write([]byte("proxied"))
	}))
	defer proxy.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte("direct"))
	}))
	defer target.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	ctx := WithProxy(context.Background(), &ProxyConfig{Host: proxyURL.Hostname(), Port: proxyURL.Port()})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "proxied" {
		t.Fatalf("body = %q, want proxied (target hits=%d proxy hits=%d)", string(body), targetHits, proxyHits)
	}
	if atomic.LoadInt32(&proxyHits) != 1 {
		t.Fatalf("proxy hits = %d, want 1", proxyHits)
	}
	if atomic.LoadInt32(&targetHits) != 0 {
		t.Fatalf("target hits = %d, want 0", targetHits)
	}
}
