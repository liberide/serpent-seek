package providers

import (
	"strconv"
	"strings"
)

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

// parseCSV splits a comma separated list, trimming and lowercasing entries.
func parseCSV(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseUpperCSV splits a comma separated list into uppercase entries.
func parseUpperCSV(v string) []string {
	raw := parseCSV(v)
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		out = append(out, strings.ToUpper(p))
	}
	return out
}

// parseInt parses an integer with a fallback.
func parseInt(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// parseIntCSV parses a comma separated integer list with a fallback.
func parseIntCSV(v string, def []int) []int {
	parts := parseCSV(v)
	if len(parts) == 0 {
		return def
	}
	var out []int
	for _, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func containsStr(list []string, needle string) bool {
	for _, s := range list {
		if s == needle {
			return true
		}
	}
	return false
}

func containsInt(list []int, needle int) bool {
	for _, n := range list {
		if n == needle {
			return true
		}
	}
	return false
}
