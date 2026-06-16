package config

import "testing"

// ── ContextWindowForModel ──────────────────────────────────────────────────────

func TestContextWindowForModel_NilEntry(t *testing.T) {
	var e *ProviderEntry
	if got := e.ContextWindowForModel("anything"); got != 0 {
		t.Errorf("nil entry: ContextWindowForModel = %d, want 0", got)
	}
}

func TestContextWindowForModel_FallbackToProviderWide(t *testing.T) {
	e := &ProviderEntry{ContextWindow: 128_000}
	if got := e.ContextWindowForModel("unlisted-model"); got != 128_000 {
		t.Errorf("fallback: got %d, want 128000", got)
	}
}

func TestContextWindowForModel_PerModelOverride(t *testing.T) {
	e := &ProviderEntry{
		ContextWindow:   1_000_000,
		ContextWindows: map[string]int{
			"mimo-v2.5":      65_536,
			"deepseek-v4-flash": 128_000,
		},
	}
	if got := e.ContextWindowForModel("mimo-v2.5"); got != 65_536 {
		t.Errorf("per-model: mimo-v2.5 = %d, want 65536", got)
	}
	if got := e.ContextWindowForModel("deepseek-v4-flash"); got != 128_000 {
		t.Errorf("per-model: deepseek-v4-flash = %d, want 128000", got)
	}
	// Unlisted falls back.
	if got := e.ContextWindowForModel("deepseek-v4-pro"); got != 1_000_000 {
		t.Errorf("fallback to provider-wide: got %d, want 1000000", got)
	}
}

func TestContextWindowForModel_TrimsWhitespace(t *testing.T) {
	e := &ProviderEntry{
		ContextWindows: map[string]int{"mimo-v2.5": 65_536},
	}
	if got := e.ContextWindowForModel("  mimo-v2.5\t"); got != 65_536 {
		t.Errorf("trim: got %d, want 65536", got)
	}
}

func TestContextWindowForModel_ZeroDisablesCompaction(t *testing.T) {
	e := &ProviderEntry{
		ContextWindow:   128_000,
		ContextWindows: map[string]int{"mimo-v2.5": 0},
	}
	if got := e.ContextWindowForModel("mimo-v2.5"); got != 0 {
		t.Errorf("explicit 0 must win: got %d, want 0", got)
	}
	// Other models still get the provider-wide value.
	if got := e.ContextWindowForModel("other"); got != 128_000 {
		t.Errorf("fallback: got %d, want 128000", got)
	}
}

func TestContextWindowForModel_EmptyMapFallsBack(t *testing.T) {
	e := &ProviderEntry{
		ContextWindow:   200_000,
		ContextWindows: map[string]int{},
	}
	if got := e.ContextWindowForModel("anything"); got != 200_000 {
		t.Errorf("empty map: got %d, want 200000", got)
	}
}