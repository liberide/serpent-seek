package engine

import (
	"net/url"
	"strings"

	"github.com/liberide/serpent-seek/internal/providers"
)

// trackingParams are query keys dropped during link normalization in addition
// to every utm_* parameter.
var trackingParams = map[string]bool{
	"gclid": true, "dclid": true, "fbclid": true, "yclid": true,
	"twclid": true, "msclkid": true, "mc_cid": true, "mc_eid": true,
	"igshid": true, "spm": true, "_openstat": true,
}

// normalizeLink reduces a result URL to its dedupe key: lowercase scheme and
// host, no www. prefix, no trailing slash, no fragment and no utm_*/tracking
// query parameters.
func normalizeLink(raw string) string {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return strings.ToLower(trimmed)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	if strings.HasPrefix(host, "www.") {
		host = strings.TrimPrefix(host, "www.")
	}
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path != "" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			k := strings.ToLower(key)
			if strings.HasPrefix(k, "utm_") || trackingParams[k] {
				q.Del(key)
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// merger accumulates full-chain rows keyed by their normalized link. The
// first occurrence of a link wins; later duplicates only extend Sources.
type merger struct {
	rows      []providers.Row
	byKey     map[string]int
	collected int // rows seen, including duplicates
}

func newMerger() *merger {
	return &merger{byKey: map[string]int{}}
}

// add merges the rows of one block. provider is the display name recorded in
// Sources. It returns true when the merged set grew or gained a source.
func (m *merger) add(provider string, rows providers.Rows) bool {
	changed := false
	for _, row := range rows {
		m.collected++
		key := normalizeLink(row.Link)
		if idx, dup := m.byKey[key]; dup {
			sources := m.rows[idx].Sources
			found := false
			for _, s := range sources {
				if s == provider {
					found = true
					break
				}
			}
			if !found {
				m.rows[idx].Sources = append(sources, provider)
				changed = true
			}
			continue
		}
		row.Sources = append([]string{}, provider)
		m.byKey[key] = len(m.rows)
		m.rows = append(m.rows, row)
		changed = true
	}
	return changed
}

// collectedTotal returns how many rows were fed into the merger.
func (m *merger) collectedTotal() int { return m.collected }

// uniqueCount returns the number of distinct links seen so far.
func (m *merger) uniqueCount() int { return len(m.rows) }

// duplicatesRemoved returns how many rows were dropped as duplicates.
func (m *merger) duplicatesRemoved() int { return m.collected - len(m.rows) }

// providers returns the provider display names that contributed rows, in
// order of first appearance.
func (m *merger) providers() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, row := range m.rows {
		for _, s := range row.Sources {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// rows returns the merged list capped at count (0 = no cap), preserving the
// order of first appearance.
func (m *merger) rowsUpTo(count int) providers.Rows {
	rows := m.rows
	if count > 0 && len(rows) > count {
		rows = rows[:count]
	}
	out := make(providers.Rows, len(rows))
	for i, row := range rows {
		row.Sources = append([]string{}, row.Sources...)
		out[i] = row
	}
	return out
}
