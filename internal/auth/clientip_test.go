package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresForwardedByDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := NewClientIPResolver(nil).ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want peer address", got)
	}
}

func TestClientIPTrustsForwardedFromProxy(t *testing.T) {
	r := NewClientIPResolver([]string{"10.0.0.0/8"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.1.2.3")
	if got := r.ClientIP(req); got != "198.51.100.9" {
		t.Fatalf("ClientIP = %q, want left-most client address", got)
	}

	// A non-trusted peer must not be able to spoof the header.
	req.RemoteAddr = "203.0.113.7:5555"
	if got := r.ClientIP(req); got != "203.0.113.7" {
		t.Fatalf("spoofed ClientIP = %q, want peer address", got)
	}
}
