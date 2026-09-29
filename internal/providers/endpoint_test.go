package providers

import "testing"

func TestValidateBaseURL(t *testing.T) {
	valid := []string{"", "https://example.com", "http://host:8080/path"}
	for _, v := range valid {
		if err := ValidateBaseURL(v); err != nil {
			t.Fatalf("ValidateBaseURL(%q) = %v", v, err)
		}
	}
	invalid := []string{"file:///etc/passwd", "gopher://x", "ftp://x", "https://user:pass@x", "https://"}
	for _, v := range invalid {
		if err := ValidateBaseURL(v); err == nil {
			t.Fatalf("ValidateBaseURL(%q) = nil, want error", v)
		}
	}
}

func TestValidateProxyHostAndPort(t *testing.T) {
	if err := ValidateProxyHost("127.0.0.1:1080"); err != nil {
		t.Fatalf("proxy host: %v", err)
	}
	if err := ValidateProxyHost("socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("proxy url: %v", err)
	}
	if err := ValidateProxyHost("file:///x"); err == nil {
		t.Fatal("expected error for file scheme")
	}
	if err := ValidateProxyPort("1080"); err != nil {
		t.Fatalf("port: %v", err)
	}
	if err := ValidateProxyPort("0"); err == nil {
		t.Fatal("expected error for port 0")
	}
	if err := ValidateProxyPort("70000"); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}
