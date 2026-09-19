package logging

import (
	"strings"
	"sync"
)

// Redactor masks registered secret values from log output. Upstream APIs may
// echo an API key back inside an error body; such values must never reach the
// logs. The registry is process-wide and safe for concurrent use.
type Redactor struct {
	mu      sync.RWMutex
	secrets []string
}

// NewRedactor returns an empty Redactor.
func NewRedactor() *Redactor {
	return &Redactor{}
}

// Add registers a secret. Empty values are ignored. Shorter secrets are kept
// too, but values shorter than 4 characters are skipped to avoid masking
// random substrings.
func (r *Redactor) Add(values ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range values {
		v = strings.TrimSpace(v)
		if len(v) < 4 {
			continue
		}
		dup := false
		for _, existing := range r.secrets {
			if existing == v {
				dup = true
				break
			}
		}
		if !dup {
			r.secrets = append(r.secrets, v)
		}
	}
}

// String replaces every registered secret substring with "***".
func (r *Redactor) String(s string) string {
	if s == "" {
		return s
	}
	r.mu.RLock()
	secrets := r.secrets
	r.mu.RUnlock()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(s, secret) {
			s = strings.ReplaceAll(s, secret, "***")
		}
	}
	return s
}

// Count returns the number of registered secrets (used in tests).
func (r *Redactor) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.secrets)
}
