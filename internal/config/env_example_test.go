package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestEnvExampleDocumentsEveryBinding verifies that every environment variable
// read by config.Load appears in .env.example.
func TestEnvExampleDocumentsEveryBinding(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Skip(".env.example not present in this checkout")
	}
	// Match VAR= and commented "# VAR=" lines.
	re := regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]*)=`)
	documented := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		documented[m[1]] = true
	}

	var missing []string
	for _, b := range envBindings {
		if !documented[b.Env] {
			missing = append(missing, b.Env+" (binding "+b.Key+")")
		}
	}
	// Variables read directly in Load.
	for _, env := range []string{
		"PORT", "HOST", "DATA_DIR", "SQLITE_PATH", "LOG_FORMAT",
		"HISTORY_CLEANUP_CRON", "VACUUM_CRON", "SETUP_TOKEN", "SERPENT_API_KEY",
		"APISERPENT_API_KEY", "SERPBASE_API_KEY", "SEARXNG_URL", "SEARXNG_API_KEY",
		"SEARXNG_LANGUAGE", "YANDEX_API_KEY", "YANDEX_FOLDER_ID", "NCBI_API_KEY",
		"GOOGLE_API_KEY", "GOOGLE_MODEL", "GOOGLE_CX", "GOOGLE_DRIVER",
		"BLOCK_PRIVATE_NETWORKS",
	} {
		if !documented[env] {
			missing = append(missing, env+" (direct)")
		}
	}
	if len(missing) > 0 {
		t.Errorf(".env.example is missing documented entries:\n  %s", strings.Join(missing, "\n  "))
	}
}
