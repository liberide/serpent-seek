package auth

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/liberide/serpent-seek/internal/store"
)

// waUser adapts a store.User to the webauthn.User interface.
type waUser struct {
	id      []byte
	name    string
	display string
	creds   []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.id }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.display }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// PasskeyService wraps go-webauthn with short-lived ceremony sessions.
type PasskeyService struct {
	web      *webauthn.WebAuthn
	mu       sync.Mutex
	sessions map[string]webauthn.SessionData
	expires  map[string]time.Time
	lookupFn func(rawID []byte) (*store.User, []*store.Passkey, error)
}

// NewPasskeyService builds the service. It returns a disabled service (Enabled
// false) when origin/rpID are not configured, which makes the UI hide passkeys.
func NewPasskeyService(origin, rpID, rpName string) (*PasskeyService, error) {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	rpID = strings.TrimSpace(rpID)
	if origin == "" {
		return &PasskeyService{expires: map[string]time.Time{}, sessions: map[string]webauthn.SessionData{}}, nil
	}
	if rpID == "" {
		if u, err := url.Parse(origin); err == nil {
			rpID = u.Hostname()
		}
	}
	if rpName == "" {
		rpName = "SerpentSeek"
	}
	web, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, err
	}
	return &PasskeyService{
		web:      web,
		sessions: map[string]webauthn.SessionData{},
		expires:  map[string]time.Time{},
	}, nil
}

// Enabled reports whether passkeys are available (https origin + rpID).
func (s *PasskeyService) Enabled() bool { return s != nil && s.web != nil }

// RPID returns the configured relying party id.
func (s *PasskeyService) RPID() string {
	if !s.Enabled() {
		return ""
	}
	return s.web.Config.RPID
}

// BeginRegistration starts a registration ceremony.
func (s *PasskeyService) BeginRegistration(user *store.User, credentials []*store.Passkey) (*protocol.CredentialCreation, string, error) {
	creation, session, err := s.web.BeginRegistration(&waUser{
		id: []byte(user.ID), name: user.Name, display: user.Name, creds: toWACredentials(credentials),
	})
	if err != nil {
		return nil, "", err
	}
	id := s.storeSession(*session)
	return creation, id, nil
}

// FinishRegistration validates the attestation response.
func (s *PasskeyService) FinishRegistration(user *store.User, credentials []*store.Passkey, sessionID string, r *http.Request) (*webauthn.Credential, error) {
	session, ok := s.takeSession(sessionID)
	if !ok {
		return nil, errSessionExpired
	}
	return s.web.FinishRegistration(&waUser{
		id: []byte(user.ID), name: user.Name, display: user.Name, creds: toWACredentials(credentials),
	}, session, r)
}

// BeginLogin starts a discoverable (usernameless) login ceremony.
func (s *PasskeyService) BeginLogin() (*protocol.CredentialAssertion, string, error) {
	assertion, session, err := s.web.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	return assertion, s.storeSession(*session), nil
}

// FinishLogin validates the assertion response and returns the resolved user.
func (s *PasskeyService) FinishLogin(sessionID string, r *http.Request) (*store.User, *webauthn.Credential, error) {
	session, ok := s.takeSession(sessionID)
	if !ok {
		return nil, nil, errSessionExpired
	}
	var found *store.User
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		user, pks, err := s.lookup(rawID)
		if err != nil {
			return nil, err
		}
		found = user
		return &waUser{id: []byte(user.ID), name: user.Name, display: user.Name, creds: toWACredentials(pks)}, nil
	}
	_, cred, err := s.web.FinishPasskeyLogin(handler, session, r)
	if err != nil {
		return nil, nil, err
	}
	return found, cred, nil
}

// lookupFn is injected to keep the auth package independent of handler wiring.

// SetLookup registers the credential lookup callback used during login.
func (s *PasskeyService) SetLookup(fn func(rawID []byte) (*store.User, []*store.Passkey, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookupFn = fn
}

func (s *PasskeyService) lookup(rawID []byte) (*store.User, []*store.Passkey, error) {
	s.mu.Lock()
	fn := s.lookupFn
	s.mu.Unlock()
	if fn == nil {
		return nil, nil, errSessionExpired
	}
	return fn(rawID)
}

func (s *PasskeyService) storeSession(session webauthn.SessionData) string {
	id, _ := RandomToken(16)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, exp := range s.expires {
		if now.After(exp) {
			delete(s.expires, key)
			delete(s.sessions, key)
		}
	}
	s.sessions[id] = session
	s.expires[id] = now.Add(5 * time.Minute)
	return id
}

func (s *PasskeyService) takeSession(id string) (webauthn.SessionData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	exp := s.expires[id]
	if !ok {
		return webauthn.SessionData{}, false
	}
	delete(s.sessions, id)
	delete(s.expires, id)
	if time.Now().After(exp) {
		return webauthn.SessionData{}, false
	}
	return session, true
}

var errSessionExpired = errPasskeySession("passkey ceremony session expired")

type errPasskeySession string

func (e errPasskeySession) Error() string { return string(e) }

// ToWebAuthnCredentials converts stored passkeys for the webauthn package.
func toWACredentials(pks []*store.Passkey) []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(pks))
	for _, p := range pks {
		out = append(out, webauthn.Credential{
			ID:        p.CredentialID,
			PublicKey: p.PublicKey,
			Transport: parseTransports(p.Transports),
			Authenticator: webauthn.Authenticator{
				AAGUID:    p.AAGUID,
				SignCount: p.SignCount,
			},
		})
	}
	return out
}

// FromWebAuthnCredential converts a registered credential for storage.
func FromWebAuthnCredential(userID, name string, c *webauthn.Credential) *store.Passkey {
	transports := make([]string, 0, len(c.Transport))
	for _, t := range c.Transport {
		transports = append(transports, string(t))
	}
	return &store.Passkey{
		UserID:       userID,
		Name:         name,
		CredentialID: c.ID,
		PublicKey:    c.PublicKey,
		SignCount:    c.Authenticator.SignCount,
		AAGUID:       c.Authenticator.AAGUID,
		Transports:   strings.Join(transports, ","),
		CreatedAt:    store.Now(),
	}
}

func parseTransports(v string) []protocol.AuthenticatorTransport {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var out []protocol.AuthenticatorTransport
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, protocol.AuthenticatorTransport(p))
		}
	}
	return out
}
