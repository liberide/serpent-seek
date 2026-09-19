package logging

import "testing"

func TestRedactorMasksRegisteredSecrets(t *testing.T) {
	r := NewRedactor()
	r.Add("supersecretvalue", "ab", "")
	if got := r.String("error: supersecretvalue leaked"); got != "error: *** leaked" {
		t.Fatalf("unexpected redaction: %q", got)
	}
	// Values shorter than 4 chars are not registered.
	if got := r.String("an ab value"); got != "an ab value" {
		t.Fatalf("short value should not be masked: %q", got)
	}
	if r.Count() != 1 {
		t.Fatalf("expected 1 secret, got %d", r.Count())
	}
}

func TestRedactorDeduplicates(t *testing.T) {
	r := NewRedactor()
	r.Add("abcdef")
	r.Add("abcdef")
	if r.Count() != 1 {
		t.Fatalf("expected dedup, got %d", r.Count())
	}
}

func TestLoggerRedactsMessageAndAttrs(t *testing.T) {
	red := NewRedactor()
	red.Add("topsecretvalue")
	var buf testWriter
	logger := New(Options{Writer: &buf, Level: "debug", Redact: red})
	logger.Info("0001-abcd", "apiserpent", "key is topsecretvalue", "detail", "topsecretvalue")
	if contains(buf.String(), "topsecretvalue") {
		t.Fatalf("secret leaked into log: %s", buf.String())
	}
	if !contains(buf.String(), "***") {
		t.Fatalf("expected masked value: %s", buf.String())
	}
}

type testWriter struct{ data []byte }

func (w *testWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *testWriter) String() string { return string(w.data) }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
