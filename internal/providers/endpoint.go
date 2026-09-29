package providers

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ValidateBaseURL validates a provider base URL; empty values are allowed.
func ValidateBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (only http/https)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("URL must include a host")
	}
	if u.User != nil {
		return errors.New("URL must not embed credentials")
	}
	return nil
}

// ProxyTypeAllowed reports whether a proxy scheme is supported.
func ProxyTypeAllowed(scheme string) bool {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "", "http", "https", "socks", "socks5", "socks5h":
		return true
	default:
		return false
	}
}

// ValidateProxyHost validates a proxy host (a bare host[:port] or a URL).
func ValidateProxyHost(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
		if !ProxyTypeAllowed(u.Scheme) {
			return fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
		}
		if u.Host == "" {
			return errors.New("proxy URL must include a host")
		}
		return nil
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return errors.New("proxy host must not contain whitespace")
	}
	return nil
}

// ValidateProxyPort validates an optional proxy port.
func ValidateProxyPort(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("proxy port must be a number between 1 and 65535")
	}
	return nil
}
