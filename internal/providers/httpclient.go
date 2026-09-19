package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// MaxResponseBytes caps an upstream response body, protecting memory from a
// hostile or broken server.
const MaxResponseBytes = 4 * 1024 * 1024

// HTTPClient is the shared outbound HTTP client. Redirects are blocked because
// Go's default client copies all headers (including X-API-Key) to the redirect
// target, leaking credentials to third-party hosts. This mirrors serpent-shim.
type HTTPClient struct {
	client    *http.Client
	userAgent string
	redact    func(string) string
}

// NewHTTPClient builds the shared client with the given User-Agent.
func NewHTTPClient(userAgent string) *HTTPClient {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &HTTPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second, // safety net; per-node ctx deadlines are tighter
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		userAgent: userAgent,
	}
}

// SetRedactor registers the secret masking function used when building errors.
func (h *HTTPClient) SetRedactor(fn func(string) string) { h.redact = fn }

// Redact masks registered secrets.
func (h *HTTPClient) Redact(s string) string {
	if h.redact == nil {
		return s
	}
	return h.redact(s)
}

// UserAgent returns the configured outbound User-Agent.
func (h *HTTPClient) UserAgent() string { return h.userAgent }

// Do executes a request, ensuring the browser User-Agent is present.
func (h *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", h.userAgent)
	}
	return h.client.Do(req)
}

// ReadBody reads a response body up to MaxResponseBytes and reports whether it
// was truncated.
func (h *HTTPClient) ReadBody(resp *http.Response) ([]byte, bool, error) {
	if resp.Body == nil {
		return nil, false, nil
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, MaxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return body, false, err
	}
	truncated := len(body) > MaxResponseBytes
	if truncated {
		body = body[:MaxResponseBytes]
	}
	return body, truncated, nil
}

// ClassifyTransport maps a transport error to a Result kind.
func ClassifyTransport(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return KindTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "deadline exceeded") {
		return KindTimeout
	}
	return KindNet
}

// HTTPClassify maps a non-2xx status to a kind and permanence given the
// configured retryable codes. Unknown codes are treated as permanent.
func HTTPClassify(status int, retryCodes []int) (string, bool) {
	for _, c := range retryCodes {
		if c == status {
			return KindHTTP, false
		}
	}
	// 5xx is always retryable even if not explicitly listed.
	if status >= 500 {
		return KindHTTP, false
	}
	return KindHTTP, true
}

// IsJSONStatus reports whether the status code is success (2xx).
func IsJSONStatus(status int) bool { return status >= 200 && status < 300 }

// trimBody produces a short, single-line, redacted excerpt of a response body.
func (h *HTTPClient) trimBody(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 160 {
		s = s[:160]
	}
	return h.Redact(s)
}

// HTTPError formats a concise error with a redacted body excerpt.
func (h *HTTPClient) HTTPError(status int, reason string, body []byte) string {
	return strings.TrimSpace(fmt.Sprintf("HTTP %d %s %s", status, reason, h.trimBody(body)))
}
