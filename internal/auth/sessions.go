package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/store"
)

// SessionCookieName is the browser session cookie.
const SessionCookieName = "seek_sid"

// CSRFCookieName is the double-submit CSRF cookie (readable by JavaScript).
const CSRFCookieName = "seek_csrf"

// CSRFHeaderName is the required header on cookie-authenticated mutations.
const CSRFHeaderName = "X-CSRF-Token"

// SessionTTL is the rolling browser session lifetime.
const SessionTTL = 30 * 24 * time.Hour

// HashToken returns the hex sha256 of a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession inserts a new session and returns its raw token.
func CreateSession(ctx context.Context, st store.Storage, userID, ip, ua string) (string, error) {
	token, err := RandomToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	sess := &store.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		TokenHash: HashToken(token),
		CreatedAt: now.Format(time.RFC3339Nano),
		ExpiresAt: now.Add(SessionTTL).Format(time.RFC3339Nano),
		IP:        ip,
		UA:        ua,
	}
	if err := st.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	return token, nil
}

// LookupSession resolves a raw session token to its session and user, rolling
// the expiry forward and pruning expired records.
func LookupSession(ctx context.Context, st store.Storage, token string) (*store.Session, *store.User, error) {
	if token == "" {
		return nil, nil, ErrInvalidKey
	}
	hash := HashToken(token)
	sess, err := st.GetSessionByTokenHash(ctx, hash)
	if err != nil {
		return nil, nil, ErrInvalidKey
	}
	expires, err := time.Parse(time.RFC3339Nano, sess.ExpiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		_ = st.DeleteSession(ctx, sess.ID)
		return nil, nil, ErrInvalidKey
	}
	user, err := st.GetUser(ctx, sess.UserID)
	if err != nil || user.Disabled {
		return nil, nil, ErrInvalidKey
	}
	// Rolling expiry: extend when more than an hour has elapsed.
	now := time.Now().UTC()
	sess.ExpiresAt = now.Add(SessionTTL).Format(time.RFC3339Nano)
	_ = st.UpdateSessionExpiry(ctx, sess.ID, sess.ExpiresAt)
	return sess, user, nil
}

// SetSessionCookies writes the session and CSRF cookies.
func SetSessionCookies(w http.ResponseWriter, r *http.Request, token string, secure bool) error {
	csrf, err := RandomToken(32)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrf,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
	return nil
}

// ClearSessionCookies expires both cookies.
func ClearSessionCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", HttpOnly: name == SessionCookieName,
			Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: -1,
		})
	}
}

// SessionUserID extracts the user id from a session (helper for handlers).
func SessionUserID(sess *store.Session) string {
	if sess == nil {
		return ""
	}
	return sess.UserID
}
