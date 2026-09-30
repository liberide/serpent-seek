package auth

import (
	"net"
	"net/http"
	"strings"
)

// ClientIPResolver extracts the client IP, honoring X-Forwarded-For only from
// trusted proxies.
type ClientIPResolver struct {
	trusted []*net.IPNet
}

// NewClientIPResolver parses trusted proxy entries (IPs or CIDRs).
func NewClientIPResolver(trusted []string) *ClientIPResolver {
	r := &ClientIPResolver{}
	for _, entry := range trusted {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			r.trusted = append(r.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, ipnet, err := net.ParseCIDR(entry); err == nil {
			r.trusted = append(r.trusted, ipnet)
		}
	}
	return r
}

// ClientIP returns the best-effort client IP.
func (r *ClientIPResolver) ClientIP(req *http.Request) string {
	remote := hostOnly(req.RemoteAddr)
	if r == nil || len(r.trusted) == 0 || !r.isTrusted(remote) {
		return remote
	}
	xff := req.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote
	}
	// Walk right-to-left, skipping trusted hops.
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" || net.ParseIP(candidate) == nil {
			continue
		}
		if r.isTrusted(candidate) {
			continue
		}
		return candidate
	}
	return remote
}

// Trusted reports whether any trusted proxy is configured.
func (r *ClientIPResolver) Trusted() bool { return r != nil && len(r.trusted) > 0 }

func (r *ClientIPResolver) isTrusted(raw string) bool {
	ip := net.ParseIP(raw)
	if ip == nil {
		return false
	}
	for _, n := range r.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// hostOnly strips an optional port.
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
