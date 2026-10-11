// Package notify delivers outbound webhook notifications for operational
// events (provider circuit-breaker trips and daily quota exhaustion).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Logger is the minimal logging surface the notifier needs.
type Logger interface {
	Warn(rid, api, message string, args ...any)
}

// minInterval throttles repeated notifications for the same target.
const minInterval = time.Minute

var (
	mu     sync.Mutex
	last   = map[string]time.Time{}
	client = &http.Client{Timeout: 5 * time.Second}
)

// Emit posts a JSON webhook for event to url. It is a no-op when url is empty.
// Repeated events for the same target are throttled to one per minute. Emit is
// synchronous; callers that must not block the request path should invoke it in
// a goroutine. The delivery timeout is independent of ctx.
func Emit(ctx context.Context, url, event string, data map[string]any, log Logger) {
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	key := url + "|" + event + "|" + target(data)
	now := time.Now()
	mu.Lock()
	if t, ok := last[key]; ok && now.Sub(t) < minInterval {
		mu.Unlock()
		return
	}
	last[key] = now
	mu.Unlock()

	payload := map[string]any{
		"event": event,
		"ts":    now.UTC().Format(time.RFC3339),
		"data":  data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		warn(log, "webhook marshal failed: "+err.Error())
		return
	}
	// Detach from the request context so a cancelled search still delivers.
	reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		warn(log, "webhook request failed: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SerpentSeek-Webhook/1.0")
	resp, err := client.Do(req)
	if err != nil {
		warn(log, "webhook delivery failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		warn(log, "webhook returned "+resp.Status)
	}
}

// target returns the dedup key carried in the event data.
func target(data map[string]any) string {
	if v, ok := data["target"].(string); ok {
		return v
	}
	return ""
}

func warn(log Logger, msg string) {
	if log != nil {
		log.Warn("", "", msg)
	}
}
