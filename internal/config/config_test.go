package config

import (
	"context"
	"errors"
	"testing"
)

type memKV struct{ m map[string]string }

func (k *memKV) GetSetting(_ context.Context, key string) (string, error) {
	if v, ok := k.m[key]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func (k *memKV) AllSettings(_ context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range k.m {
		out[key] = value
	}
	return out, nil
}

func (k *memKV) SetSetting(_ context.Context, key, value string) error {
	k.m[key] = value
	return nil
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m := NewManager(cfg, &memKV{m: map[string]string{}})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	return m
}

// An environment AUTH_ENABLED=false may be overridden from the UI: the operator
// must always be able to turn authentication back on.
func TestAuthEnvFalseCanBeEnabledFromUI(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "false")
	m := newTestManager(t)
	ctx := context.Background()
	if m.GetBool(ctx, KeyAuthEnabled) {
		t.Fatal("env AUTH_ENABLED=false must start disabled")
	}
	if err := m.Set(ctx, KeyAuthEnabled, "true"); err != nil {
		t.Fatalf("enabling auth from the UI must be allowed: %v", err)
	}
	if !m.GetBool(ctx, KeyAuthEnabled) {
		t.Fatal("auth must now be enabled")
	}
	if _, env := m.Snapshot(ctx); env[KeyAuthEnabled] {
		t.Fatal("env AUTH_ENABLED=false must not be reported as a read-only pin")
	}
}

// An environment AUTH_ENABLED=true stays pinned: it may force auth on but the
// UI cannot silently switch it off.
func TestAuthEnvTrueCannotBeDisabledFromUI(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	m := newTestManager(t)
	ctx := context.Background()
	if !m.GetBool(ctx, KeyAuthEnabled) {
		t.Fatal("env AUTH_ENABLED=true must start enabled")
	}
	if err := m.Set(ctx, KeyAuthEnabled, "false"); err == nil {
		t.Fatal("disabling auth must be rejected when the environment forces it on")
	}
	if !m.GetBool(ctx, KeyAuthEnabled) {
		t.Fatal("auth must stay enabled")
	}
	if _, env := m.Snapshot(ctx); !env[KeyAuthEnabled] {
		t.Fatal("env AUTH_ENABLED=true must be reported as a read-only pin")
	}
}

// Any other env-pinned setting remains read-only.
func TestOtherEnvPinnedSettingIsReadOnly(t *testing.T) {
	t.Setenv("MAX_ATTEMPTS", "7")
	m := newTestManager(t)
	ctx := context.Background()
	if m.GetInt(ctx, KeyMaxAttempts) != 7 {
		t.Fatal("env MAX_ATTEMPTS must win")
	}
	if err := m.Set(ctx, KeyMaxAttempts, "3"); err == nil {
		t.Fatal("env-pinned max_attempts must be read-only")
	}
}
