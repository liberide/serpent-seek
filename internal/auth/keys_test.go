package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateAndVerifyKey(t *testing.T) {
	for _, admin := range []bool{false, true} {
		plaintext, prefix, hash, err := GenerateKey(admin)
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		kind, gotPrefix, secret, ok := ParseKey(plaintext)
		if !ok {
			t.Fatalf("ParseKey failed for %q", plaintext)
		}
		if gotPrefix != prefix {
			t.Fatalf("prefix mismatch: %q != %q", gotPrefix, prefix)
		}
		wantKind := KindSearch
		if admin {
			wantKind = KindAdmin
		}
		if kind != wantKind {
			t.Fatalf("kind mismatch: %q != %q", kind, wantKind)
		}
		if !VerifySecret(hash, secret) {
			t.Fatal("VerifySecret should accept the generated secret")
		}
		if VerifySecret(hash, secret+"x") {
			t.Fatal("VerifySecret should reject a wrong secret")
		}
		if VerifySecret("not-a-hash", secret) {
			t.Fatal("VerifySecret should reject malformed hashes")
		}
	}
}

func TestGenerateDeveloperKey(t *testing.T) {
	plaintext, prefix, hash, err := GenerateDeveloperKey()
	if err != nil {
		t.Fatalf("GenerateDeveloperKey: %v", err)
	}
	kind, gotPrefix, secret, ok := ParseKey(plaintext)
	if !ok {
		t.Fatalf("ParseKey failed for %q", plaintext)
	}
	if kind != KindDeveloper {
		t.Fatalf("kind mismatch: %q != developer", kind)
	}
	if gotPrefix != prefix {
		t.Fatalf("prefix mismatch: %q != %q", gotPrefix, prefix)
	}
	if !strings.HasPrefix(plaintext, DeveloperKeyPrefix) {
		t.Fatalf("plaintext %q should start with %q", plaintext, DeveloperKeyPrefix)
	}
	if !VerifySecret(hash, secret) {
		t.Fatal("VerifySecret should accept the generated developer secret")
	}
}

func TestParseKeyRejectsGarbage(t *testing.T) {
	for _, token := range []string{"", "abc", "seek_ak_onlyprefix", "seek_zz_abc_def"} {
		if _, _, _, ok := ParseKey(token); ok {
			t.Fatalf("expected ParseKey to reject %q", token)
		}
	}
}

func TestValidateCSRF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if ValidateCSRF(req) {
		t.Fatal("missing csrf cookie should fail")
	}
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "token"})
	req.Header.Set(CSRFHeaderName, "token")
	if !ValidateCSRF(req) {
		t.Fatal("matching double-submit should pass")
	}
	req.Header.Set(CSRFHeaderName, "other")
	if ValidateCSRF(req) {
		t.Fatal("mismatched token should fail")
	}
}
