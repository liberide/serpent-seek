package httpd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	sqlitestore "github.com/liberide/serpent-seek/internal/store/sqlite"
)

// newMetricsServer builds a minimal Server for handleMetrics tests.
func newMetricsServer(t *testing.T) (*Server, *config.Manager) {
	t.Helper()
	ctx := context.Background()
	driver, err := sqlitestore.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := driver.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	settings := config.NewManager(cfg, driver)
	if err := settings.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := &Server{
		settings: settings,
		clientIP: auth.NewClientIPResolver(nil),
		hub:      nil,
		engine:   nil,
		log:      logging.New(logging.Options{Level: "error"}),
	}
	return s, settings
}

func getMetrics(t *testing.T, s *Server, authz, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	w := httptest.NewRecorder()
	s.handleMetrics(w, req)
	return w
}

func TestMetricsPublicByDefault(t *testing.T) {
	s, _ := newMetricsServer(t)
	if res := getMetrics(t, s, "", ""); res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (endpoint must stay public without settings)", res.Code)
	}
}

func TestMetricsBearerToken(t *testing.T) {
	ctx := context.Background()
	s, settings := newMetricsServer(t)
	if err := settings.Set(ctx, config.KeyMetricsAuthToken, "s3cret"); err != nil {
		t.Fatalf("set token: %v", err)
	}

	if res := getMetrics(t, s, "", ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("no header: status = %d, want 401", res.Code)
	}
	if h := getMetrics(t, s, "", "").Header().Get("WWW-Authenticate"); h == "" {
		t.Error("401 must carry a WWW-Authenticate challenge for scrapers")
	}
	if res := getMetrics(t, s, "Bearer wrong", ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", res.Code)
	}
	if res := getMetrics(t, s, "Bearer s3cret", ""); res.Code != http.StatusOK {
		t.Fatalf("correct token: status = %d, want 200", res.Code)
	}
}

func TestMetricsAllowedCIDRs(t *testing.T) {
	ctx := context.Background()
	s, settings := newMetricsServer(t)
	if err := settings.Set(ctx, config.KeyMetricsAllowedCIDRs, "10.0.0.0/8, 192.168.1.7"); err != nil {
		t.Fatalf("set cidrs: %v", err)
	}

	if res := getMetrics(t, s, "", "10.1.2.3:5555"); res.Code != http.StatusOK {
		t.Fatalf("allowed subnet: status = %d, want 200", res.Code)
	}
	if res := getMetrics(t, s, "", "192.168.1.7:5555"); res.Code != http.StatusOK {
		t.Fatalf("allowed bare IP: status = %d, want 200", res.Code)
	}
	if res := getMetrics(t, s, "", "8.8.8.8:5555"); res.Code != http.StatusForbidden {
		t.Fatalf("outside allowlist: status = %d, want 403", res.Code)
	}
}

func TestIPAllowed(t *testing.T) {
	cases := []struct {
		ip    string
		cidrs []string
		want  bool
	}{
		{"10.0.0.1", []string{"10.0.0.0/8"}, true},
		{"10.0.0.1", []string{"10.0.0.1"}, true},
		{"2001:db8::1", []string{"2001:db8::/32"}, true},
		// IPv4-mapped IPv6 must match IPv4 allowlists.
		{"::ffff:10.0.0.1", []string{"10.0.0.0/8"}, true},
		{"::ffff:10.0.0.1", []string{"10.0.0.1"}, true},
		{"10.0.0.2", []string{"10.0.0.1"}, false},
		{"not-an-ip", []string{"10.0.0.0/8"}, false},
		// Malformed entries never match.
		{"10.0.0.1", []string{"bogus", "10.0.0.0/24"}, true},
		{"10.0.1.1", []string{"bogus"}, false},
		{"10.0.0.1", nil, false},
	}
	for _, c := range cases {
		if got := ipAllowed(c.ip, c.cidrs); got != c.want {
			t.Errorf("ipAllowed(%q, %v) = %v, want %v", c.ip, c.cidrs, got, c.want)
		}
	}
}
