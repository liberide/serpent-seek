// Package updatecheck compares the running version with the latest published
// release of the project.
//
// It only ever performs a single GET request to a configurable URL (by default
// the public GitHub "latest release" API) and never modifies the installation.
// Every failure is non-fatal for the caller.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Result is the outcome of an update check. Enabled is false when the check is
// turned off in the settings.
type Result struct {
	Enabled         bool   `json:"enabled"`
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	URL             string `json:"url,omitempty"`
	Name            string `json:"name,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	Prerelease      bool   `json:"prerelease,omitempty"`
	// Unofficial is set by the HTTP layer when the configured release feed is not
	// the official project feed.
	Unofficial bool   `json:"unofficial,omitempty"`
	CheckedAt  string `json:"checked_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Checker fetches and caches the latest release.
type Checker struct {
	client    *http.Client
	userAgent string

	mu     sync.RWMutex
	result Result
	has    bool
}

// New builds a checker. client should have a short timeout; userAgent is sent
// to the release API (GitHub rejects requests without one).
func New(client *http.Client, userAgent string) *Checker {
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	return &Checker{client: client, userAgent: userAgent}
}

// Status returns the cached result, if a check has completed.
func (c *Checker) Status() (Result, bool) {
	if c == nil {
		return Result{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result, c.has
}

// Check fetches the latest release from url and compares it with current. The
// result is cached on success (including "no update").
func (c *Checker) Check(ctx context.Context, url, current string) (Result, error) {
	if c == nil {
		return Result{}, fmt.Errorf("updatecheck: nil checker")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return Result{}, fmt.Errorf("updatecheck: empty url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("updatecheck: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}
	var release struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Prerelease  bool   `json:"prerelease"`
		Draft       bool   `json:"draft"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return Result{}, err
	}

	res := Result{
		Enabled:     true,
		Current:     current,
		Latest:      strings.TrimPrefix(strings.TrimPrefix(release.TagName, "v"), "V"),
		URL:         release.HTMLURL,
		Name:        release.Name,
		PublishedAt: release.PublishedAt,
		Prerelease:  release.Prerelease,
		CheckedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if release.TagName != "" && !release.Draft {
		res.UpdateAvailable = CompareVersions(current, release.TagName) < 0
	}

	c.mu.Lock()
	c.result = res
	c.has = true
	c.mu.Unlock()
	return res, nil
}

// CompareVersions compares two semantic versions, returning -1, 0 or 1. A
// leading "v" and build metadata ("+...") are ignored; prereleases sort below
// the corresponding release. If either value is not a version, it returns 0, so
// a malformed tag never reports a false update.
func CompareVersions(a, b string) int {
	if !Parseable(a) || !Parseable(b) {
		return 0
	}
	na, pa := parse(a)
	nb, pb := parse(b)
	for i := 0; i < 3; i++ {
		switch {
		case na[i] < nb[i]:
			return -1
		case na[i] > nb[i]:
			return 1
		}
	}
	if pa == "" && pb != "" {
		return 1
	}
	if pa != "" && pb == "" {
		return -1
	}
	return strings.Compare(pa, pb)
}

// Parseable reports whether v looks like a semantic version (a leading numeric
// major component).
func Parseable(v string) bool {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" {
		return false
	}
	major := strings.SplitN(v, ".", 2)[0]
	if i := strings.IndexAny(major, "-+"); i >= 0 {
		major = major[:i]
	}
	_, err := strconv.Atoi(major)
	return err == nil
}

func parse(v string) ([3]int, string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	var nums [3]int
	for i, part := range strings.Split(v, ".") {
		if i >= 3 {
			break
		}
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			nums[i] = n
		}
	}
	return nums, pre
}
