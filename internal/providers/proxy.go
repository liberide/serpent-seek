package providers

import (
	"context"
	"net"
	"net/url"
	"strings"
)

// proxyContextKey carries a per-request ProxyConfig through the request
// context. The shared HTTP transport consults it so individual provider calls
// can be routed through different proxies.
type proxyContextKey struct{}

// ProxyConfig describes an outbound HTTP proxy selected for one provider call.
type ProxyConfig struct {
	// Type is the proxy scheme: http (default), https or socks5.
	Type     string
	Host     string
	Port     string
	Username string
	Password string
}

// normalizeProxyScheme maps a configured type to a URL scheme.
func normalizeProxyScheme(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "https":
		return "https"
	case "socks", "socks5", "socks5h":
		return "socks5"
	default:
		return "http"
	}
}

// URL builds the proxy URL, or returns (nil, nil) when no host is configured.
// Host may be a bare hostname, host:port, or a full URL; explicit Type/Port/
// Username/Password fields win over values parsed from the host.
func (c *ProxyConfig) URL() (*url.URL, error) {
	if c == nil {
		return nil, nil
	}
	rawHost := strings.TrimSpace(c.Host)
	if rawHost == "" {
		return nil, nil
	}
	u := &url.URL{Host: rawHost}
	if strings.Contains(rawHost, "://") {
		parsed, err := url.Parse(rawHost)
		if err != nil {
			return nil, err
		}
		u = parsed
	}
	if strings.TrimSpace(c.Type) != "" {
		u.Scheme = normalizeProxyScheme(c.Type)
	} else if u.Scheme == "" {
		u.Scheme = "http"
	}
	if port := strings.TrimSpace(c.Port); port != "" {
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			u.Host = net.JoinHostPort(u.Host, port)
		}
	}
	if c.Username != "" {
		if c.Password != "" {
			u.User = url.UserPassword(c.Username, c.Password)
		} else {
			u.User = url.User(c.Username)
		}
	}
	return u, nil
}

// WithProxy attaches a proxy configuration to ctx. A nil config is a no-op.
func WithProxy(ctx context.Context, cfg *ProxyConfig) context.Context {
	if cfg == nil {
		return ctx
	}
	return context.WithValue(ctx, proxyContextKey{}, cfg)
}

// proxyFromContext returns the proxy attached to ctx, if any.
func proxyFromContext(ctx context.Context) *ProxyConfig {
	if ctx == nil {
		return nil
	}
	if cfg, ok := ctx.Value(proxyContextKey{}).(*ProxyConfig); ok {
		return cfg
	}
	return nil
}
