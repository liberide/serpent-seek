package providers

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuth2 token sources with caching, stdlib only (crypto/rsa RS256 signing).
// Vertex AI Search does not support API keys at all ("API keys are not
// supported by this API"), so a service-account JWT-bearer grant is required.
// Azure Entra uses the simpler client-credentials grant.

var tokenHTTP = &http.Client{Timeout: 15 * time.Second}

// tokenCache caches one OAuth2 access token until shortly before expiry.
type tokenCache struct {
	mu     sync.Mutex
	token  string
	expiry time.Time
	fetch  func(ctx context.Context) (token string, expiresIn int, err error)
}

// Token returns a cached token or fetches a new one.
func (c *tokenCache) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expiry) {
		return c.token, nil
	}
	token, expiresIn, err := c.fetch(ctx)
	if err != nil {
		return "", err
	}
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	// 60s skew: never hand out a token that dies mid-request.
	c.token = token
	c.expiry = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return token, nil
}

// Invalidate drops the cached token (called on a 401 to force re-authentication).
func (c *tokenCache) Invalidate() {
	c.mu.Lock()
	c.token = ""
	c.expiry = time.Time{}
	c.mu.Unlock()
}

var (
	tokenSourcesMu sync.Mutex
	tokenSources   = map[string]*tokenCache{}
)

// cachedTokenSource returns the shared cache for the key, creating it once.
func cachedTokenSource(key string, fetch func(ctx context.Context) (string, int, error)) *tokenCache {
	tokenSourcesMu.Lock()
	defer tokenSourcesMu.Unlock()
	if ts, ok := tokenSources[key]; ok {
		return ts
	}
	ts := &tokenCache{fetch: fetch}
	tokenSources[key] = ts
	return ts
}

func hashKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}

// --- Google service account (JWT bearer grant, RS256) ---

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// vertexTokenSource resolves (and caches) the token source for a Vertex
// service-account JSON credential.
func vertexTokenSource(saJSON string) (*tokenCache, error) {
	saJSON = strings.TrimSpace(saJSON)
	if saJSON == "" {
		return nil, errors.New("service_account_json credential is required")
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		return nil, fmt.Errorf("service_account_json is not valid JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("service_account_json: client_email and private_key are required")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	key, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("service_account_json: private_key: %w", err)
	}
	return cachedTokenSource("gsa:"+hashKey(saJSON), func(ctx context.Context) (string, int, error) {
		return googleJWTBearerToken(ctx, sa, key, "https://www.googleapis.com/auth/cloud-platform")
	}), nil
}

// parseRSAPrivateKey decodes a PEM PKCS#8 (Google) or PKCS#1 RSA key.
func parseRSAPrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("key is not RSA")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// googleJWTBearerToken mints a one-hour access token using a signed JWT grant.
func googleJWTBearerToken(ctx context.Context, sa serviceAccount, key *rsa.PrivateKey, scope string) (string, int, error) {
	now := time.Now()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss":   sa.ClientEmail,
		"scope": scope,
		"aud":   sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	enc := base64.RawURLEncoding
	unsigned := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", 0, fmt.Errorf("sign JWT: %w", err)
	}
	assertion := unsigned + "." + enc.EncodeToString(sig)

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	return postTokenForm(ctx, sa.TokenURI, form)
}

// --- Microsoft Entra (client credentials grant) ---

// entraToken returns an access token for Azure AI Search RBAC mode.
// Credentials: tenant_id, client_id, client_secret.
func entraToken(ctx context.Context, c Credentials) (string, error) {
	tenant := strings.TrimSpace(c["tenant_id"])
	clientID := strings.TrimSpace(c["client_id"])
	secret := strings.TrimSpace(c["client_secret"])
	if tenant == "" || clientID == "" || secret == "" {
		return "", errors.New("auth_mode=aad requires tenant_id, client_id, client_secret credentials")
	}
	tokenURL := "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
	ts := cachedTokenSource("entra:"+hashKey(tenant, clientID), func(ctx context.Context) (string, int, error) {
		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {clientID},
			"client_secret": {secret},
			"scope":         {"https://search.azure.com/.default"},
		}
		return postTokenForm(ctx, tokenURL, form)
	})
	return ts.Token(ctx)
}

// postTokenForm posts an OAuth2 form grant and parses {access_token, expires_in}.
// Error bodies are echoed without the form data so secrets never leak to logs.
func postTokenForm(ctx context.Context, tokenURL string, form url.Values) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := tokenHTTP.Do(req)
	if err != nil {
		return "", 0, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode/100 != 2 {
		return "", 0, fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", 0, fmt.Errorf("token endpoint returned no access_token")
	}
	return out.AccessToken, out.ExpiresIn, nil
}

// --- Azure AI Search field-map helpers ---

func azureTitleField(p Params) string   { return defaultStr(p["title_field"], "title") }
func azureURLField(p Params) string     { return defaultStr(p["url_field"], "url") }
func azureSnippetField(p Params) string { return defaultStr(p["snippet_field"], "description") }
