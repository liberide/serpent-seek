package auth

import (
	"crypto/subtle"
	"net/http"
)

// EnsureCSRFCookie makes sure the double-submit cookie exists so the SPA can
// read it and send it back in the X-CSRF-Token header.
func EnsureCSRFCookie(w http.ResponseWriter, r *http.Request, secure bool) {
	if _, err := r.Cookie(CSRFCookieName); err == nil {
		return
	}
	token, err := RandomToken(32)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ValidateCSRF checks the double-submit token on unsafe methods.
func ValidateCSRF(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get(CSRFHeaderName)
	if header == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) == 1
}
