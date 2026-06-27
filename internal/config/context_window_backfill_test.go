package config

import "testing"

// ── mergeLegacySingleModelContextWindow ───────────────────────────────────────

func TestMergeLegacySingleModelContextWindow_AppliesToAllOldModels(t *testing.T) {
	merged := &ProviderEntry{
		Models:        []string{"mimo-v2.5-pro", "mimo-v2.5"},
		ContextWindow: 1_000_000,
	}
	old := &ProviderEntry{
		Model:         "mimo-v2.5",
		ContextWindow: 65_536,
	}
	mergeLegacySingleModelContextWindow(merged, old)
	if got, want := merged.ContextWindows["mimo-v2.5"], 65_536; got != want {
		t.Errorf("mimo-v2.5 override = %d, want %d", got, want)
	}
}

func TestMergeLegacySingleModelContextWindow_PreservesExistingOverride(t *testing.T) {
	merged := &ProviderEntry{
		Models:         []string{"mimo-v2.5"},
		ContextWindows: map[string]int{"mimo-v2.5": 32_000},
	}
	old := &ProviderEntry{
		Model:         "mimo-v2.5",
		ContextWindow: 65_536,
	}
	mergeLegacySingleModelContextWindow(merged, old)
	if got, want := merged.ContextWindows["mimo-v2.5"], 32_000; got != want {
		t.Errorf("existing override should win: got %d, want %d", got, want)
	}
}

func TestMergeLegacySingleModelContextWindow_SkipsZero(t *testing.T) {
	merged := &ProviderEntry{Models: []string{"mimo-v2.5"}}
	old := &ProviderEntry{Model: "mimo-v2.5", ContextWindow: 0} // 0 = disabled
	mergeLegacySingleModelContextWindow(merged, old)
	if len(merged.ContextWindows) != 0 {
		t.Errorf("zero window must not be merged: got %+v", merged.ContextWindows)
	}
}

func TestMergeLegacySingleModelContextWindow_NilSafe(t *testing.T) {
	mergeLegacySingleModelContextWindow(nil, &ProviderEntry{ContextWindow: 1})
	mergeLegacySingleModelContextWindow(&ProviderEntry{}, nil)
}

// ── officialProviderFromLegacy preserves ContextWindows ──────────────────────

func TestOfficialProviderFromLegacy_PreservesContextWindows(t *testing.T) {
	entry := ProviderEntry{Name: "mimo-token-plan"}
	old := &ProviderEntry{
		ContextWindow:  1_000_000,
		ContextWindows: map[string]int{"mimo-v2.5": 65_536},
	}
	got := officialProviderFromLegacy(entry, old)
	if got.ContextWindows["mimo-v2.5"] != 65_536 {
		t.Errorf("ContextWindows lost in migration: %+v", got.ContextWindows)
	}
	if got.ContextWindow != 1_000_000 {
		t.Errorf("ContextWindow lost in migration: %d", got.ContextWindow)
	}
}

func TestOfficialProviderFromLegacy_NilContextWindowsStaysNil(t *testing.T) {
	entry := ProviderEntry{Name: "mimo-token-plan"}
	old := &ProviderEntry{ContextWindow: 1_000_000}
	got := officialProviderFromLegacy(entry, old)
	if got.ContextWindows != nil {
		t.Errorf("nil ContextWindows should stay nil, got %+v", got.ContextWindows)
	}
}
