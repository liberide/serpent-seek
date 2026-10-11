package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustedBareIP(t *testing.T) {
	r := NewClientIPResolver([]string{"10.1.2.3"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:1"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	if got := r.ClientIP(req); got != "8.8.8.8" {
		t.Fatalf("ClientIP = %q, want forwarded client", got)
	}

	// A different peer is not trusted, so the header is ignored.
	req.RemoteAddr = "10.1.2.4:1"
	if got := r.ClientIP(req); got != "10.1.2.4" {
		t.Fatalf("ClientIP = %q, want peer address", got)
	}
}

func TestClientIPIPv6AndMultipleHops(t *testing.T) {
	r := NewClientIPResolver([]string{"10.0.0.0/8", "2001:db8::/32"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[2001:db8::1]:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1, 2001:db8::2")
	if got := r.ClientIP(req); got != "203.0.113.5" {
		t.Fatalf("ClientIP = %q, want left-most non-trusted hop", got)
	}

	// No header from a trusted proxy falls back to the peer.
	req.Header.Del("X-Forwarded-For")
	if got := r.ClientIP(req); got != "2001:db8::1" {
		t.Fatalf("ClientIP = %q, want IPv6 peer address", got)
	}

	// Invalid entries are skipped.
	req.Header.Set("X-Forwarded-For", "garbage, 198.51.100.7")
	if got := r.ClientIP(req); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q, want valid client after garbage", got)
	}
}

func TestClientIPTrustedHelper(t *testing.T) {
	if NewClientIPResolver(nil).Trusted() {
		t.Fatal("no trusted proxies configured, Trusted() must be false")
	}
	if !NewClientIPResolver([]string{"10.0.0.0/8"}).Trusted() {
		t.Fatal("a configured proxy must report Trusted() = true")
	}
}
